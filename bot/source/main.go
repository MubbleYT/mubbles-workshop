package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

//go:embed panel.html embed-editor.js community-panel.js
var assets embed.FS

var version = "1.4.0"

type Config struct {
	Community        CommunityConfig `json:"community"`
	Token            string          `json:"token,omitempty"`
	GuildID          string          `json:"guild_id"`
	BotName          string          `json:"bot_name"`
	MemberRoleID     string          `json:"member_role_id"`
	YouTubeID        string          `json:"youtube_id"`
	NotifyChannelID  string          `json:"notify_channel_id"`
	NotifyRoleID     string          `json:"notify_role_id"`
	NotifyText       string          `json:"notify_text"`
	LogChannelID     string          `json:"log_channel_id"`
	PollSeconds      int             `json:"poll_seconds"`
	AntiSpam         bool            `json:"anti_spam"`
	BlockInvites     bool            `json:"block_invites"`
	BlockLinks       bool            `json:"block_links"`
	SpamCount        int             `json:"spam_count"`
	SpamWindow       int             `json:"spam_window"`
	AutoStart        bool            `json:"auto_start"`
	AutoUpdate       bool            `json:"auto_update"`
	BanSlurs         bool            `json:"ban_slurs"`
	SlurTerms        string          `json:"slur_terms"`
	LimitSwearing    bool            `json:"limit_swearing"`
	SwearTerms       string          `json:"swear_terms"`
	SwearsPerMessage int             `json:"swears_per_message"`
	SwearsPerWindow  int             `json:"swears_per_window"`
	SwearWindow      int             `json:"swear_window"`
	TrapChannelID    string          `json:"trap_channel_id"`
}

type App struct {
	mu           sync.RWMutex
	cfg          Config
	session      *discordgo.Session
	ready        bool
	busy         bool
	status       string
	logs         []string
	lastCheck    string
	lastNotify   string
	lifecycle    sync.Mutex
	cancel       context.CancelFunc
	store        *Store
	dir          string
	csrf         string
	host         string
	spamMu       sync.Mutex
	spam         map[string][]time.Time
	cooldown     map[string]time.Time
	httpClient   *http.Client
	languageMu   sync.Mutex
	language     *LanguagePolicy
	swearMu      sync.Mutex
	swears       map[string][]SwearUse
	banMu        sync.Mutex
	banLast      map[string]time.Time
	embedMu      sync.Mutex
	communityMu  sync.Mutex
	joinMu       sync.Mutex
	joins        []JoinRecord
	raidMu       sync.Mutex
	memberMu     sync.Mutex
	voiceMu      sync.Mutex
	jobMu        sync.Mutex
	youtubeMu    sync.Mutex
	modalMu      sync.Mutex
	modals       map[string]ModalSession
	updater      *BotUpdater
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

func defaultConfig() Config {
	return Config{Community: defaultCommunityConfig(), BotName: "Mubble's Bot", NotifyText: "New video just dropped!", PollSeconds: 120, SpamCount: 6, SpamWindow: 8, AntiSpam: true, BlockInvites: true, AutoStart: true, AutoUpdate: true,
		BanSlurs: true, SlurTerms: defaultSlurs, LimitSwearing: true, SwearTerms: defaultSwears, SwearsPerMessage: 3, SwearsPerWindow: 8, SwearWindow: 60}
}
func main() {
	if len(os.Args) == 3 && os.Args[1] == "--apply-update" {
		if err := runUpdateHelper(os.Args[2]); err != nil {
			fmt.Println("Update failed:", err)
			os.Exit(1)
		}
		return
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		fmt.Println(err)
		return
	}
	dir = filepath.Join(dir, "MubbleDiscordBot")
	if err = os.MkdirAll(dir, 0700); err != nil {
		fmt.Println(err)
		return
	}
	store, err := openStore(filepath.Join(dir, "state.json"))
	unlock, lockErr := lockInstance(filepath.Join(dir, "instance.lock"))
	if lockErr != nil {
		fmt.Println("The bot is already open, or its data folder could not be locked. Close the other bot window first.")
		fmt.Scanln()
		return
	}
	defer unlock()
	if err != nil {
		fmt.Println("Cannot read saved data:", err)
		fmt.Scanln()
		return
	}
	a := &App{cfg: defaultConfig(), status: "Stopped", dir: dir, store: store, spam: map[string][]time.Time{}, cooldown: map[string]time.Time{}, httpClient: &http.Client{Timeout: 25 * time.Second}, shutdown: make(chan struct{})}
	if b, e := os.ReadFile(filepath.Join(dir, "config.json")); e == nil {
		if e = json.Unmarshal(b, &a.cfg); e != nil {
			fmt.Println("Invalid configuration:", e)
			fmt.Scanln()
			return
		}
	} else if !os.IsNotExist(e) {
		fmt.Println(e)
		return
	}
	normalizeCommunity(&a.cfg.Community)
	a.updater = newBotUpdater(a)
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		fmt.Println(err)
		return
	}
	a.csrf = hex.EncodeToString(key)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println(err)
		return
	}
	a.host = ln.Addr().String()
	server := &http.Server{Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second}
	url := "http://" + a.host
	fmt.Printf("Mubble Discord Bot %s\nControl panel: %s\nData: %s\nKeep this window open while the bot is running. Press Ctrl+C to exit.\n", version, url, dir)
	go func() {
		if e := server.Serve(ln); e != nil && !errors.Is(e, http.ErrServerClosed) {
			a.log("Panel error: " + a.errorText(e))
		}
	}()
	if err = writeUpdateHealth(dir, os.Args[1:]); err != nil {
		a.log("Update health check: " + err.Error())
	}
	if os.Getenv("MUBBLE_NO_BROWSER") != "1" {
		openBrowser(url)
	}
	if (a.cfg.AutoStart || hasArgument(os.Args[1:], "--resume-bot")) && a.cfg.Token != "" && a.cfg.GuildID != "" {
		go func() {
			if e := a.start(); e != nil {
				a.log("Start failed: " + a.errorText(e))
			}
		}()
	}
	updateCtx, cancelUpdates := context.WithCancel(context.Background())
	defer cancelUpdates()
	go a.updater.loop(updateCtx)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	select {
	case <-stop:
	case <-a.shutdown:
	}
	cancelUpdates()
	a.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	if c.Start() == nil {
		go c.Wait()
	}
}
func (a *App) config() Config { a.mu.RLock(); defer a.mu.RUnlock(); return a.cfg }
func (a *App) current() (*discordgo.Session, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.session == nil || !a.ready {
		return nil, errors.New("Start the bot and wait until it is connected")
	}
	return a.session, nil
}
func (a *App) log(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.Token != "" {
		s = strings.ReplaceAll(s, a.cfg.Token, "[secret]")
	}
	a.logs = append(a.logs, time.Now().Format("15:04:05")+"  "+s)
	if len(a.logs) > 150 {
		a.logs = a.logs[len(a.logs)-150:]
	}
	fmt.Println(s)
}
func (a *App) errorText(err error) string {
	if err == nil {
		return ""
	}
	var rest *discordgo.RESTError
	if errors.As(err, &rest) {
		if rest.Message != nil {
			return fmt.Sprintf("Discord: %s (code %d)", rest.Message.Message, rest.Message.Code)
		}
		return fmt.Sprintf("Discord HTTP %d", rest.Response.StatusCode)
	}
	s := err.Error()
	c := a.config()
	if c.Token != "" {
		s = strings.ReplaceAll(s, c.Token, "[secret]")
	}
	if strings.Contains(s, "/interactions/") || strings.Contains(s, "/webhooks/") {
		return "Discord interaction request failed"
	}
	return s
}

func validID(s string) bool {
	if len(s) < 16 || len(s) > 22 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func validateConfig(c *Config) error {
	c.Token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.Token), "Bot "))
	c.BotName = strings.TrimSpace(c.BotName)
	c.GuildID = strings.TrimSpace(c.GuildID)
	if c.Token == "" {
		return errors.New("Enter your Discord bot token")
	}
	if !validID(c.GuildID) {
		return errors.New("Enter a valid Discord server ID")
	}
	if c.BotName != "" && (utf8.RuneCountInString(c.BotName) < 2 || utf8.RuneCountInString(c.BotName) > 32) {
		return errors.New("Bot name must have 2–32 characters")
	}
	fields := []*string{&c.MemberRoleID, &c.NotifyChannelID, &c.NotifyRoleID, &c.LogChannelID, &c.TrapChannelID}
	for _, v := range fields {
		*v = strings.TrimSpace(*v)
		if *v != "" && !validID(*v) {
			return errors.New("Role and channel IDs must be valid Discord IDs")
		}
	}
	c.YouTubeID = strings.TrimSpace(c.YouTubeID)
	if strings.Contains(c.YouTubeID, "/channel/") {
		c.YouTubeID = strings.Split(strings.Split(c.YouTubeID, "/channel/")[1], "?")[0]
		c.YouTubeID = strings.TrimRight(c.YouTubeID, "/")
	}
	if c.YouTubeID != "" && !youtubeIDRE.MatchString(c.YouTubeID) {
		return errors.New("Use the YouTube channel ID beginning UC, not the @handle")
	}
	if (c.YouTubeID == "") != (c.NotifyChannelID == "") {
		return errors.New("Set both a YouTube channel ID and an announcement channel, or leave both empty")
	}
	if c.NotifyRoleID != "" && c.NotifyRoleID == c.GuildID {
		return errors.New("Select a dedicated notification role instead of @everyone")
	}
	if c.PollSeconds < 60 || c.PollSeconds > 3600 {
		return errors.New("Upload check interval must be 60–3600 seconds")
	}
	if c.SpamCount < 3 || c.SpamCount > 30 || c.SpamWindow < 3 || c.SpamWindow > 60 {
		return errors.New("Spam threshold must be 3–30 messages within 3–60 seconds")
	}
	if len(c.NotifyText) > 1000 {
		return errors.New("Notification text is too long (maximum 1000 bytes)")
	}
	if c.SwearsPerMessage < 1 || c.SwearsPerMessage > 30 || c.SwearsPerWindow < 1 || c.SwearsPerWindow > 100 || c.SwearWindow < 10 || c.SwearWindow > 600 {
		return errors.New("Swearing limits must be 1–30 per message and 1–100 within 10–600 seconds")
	}
	if err := validateCommunity(&c.Community, *c); err != nil {
		return err
	}
	if _, err := buildLanguagePolicy(*c); err != nil {
		return err
	}
	return nil
}
func (a *App) start() (err error) {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.Lock()
	if a.session != nil {
		a.mu.Unlock()
		return errors.New("Bot is already running")
	}
	a.busy = true
	a.status = "Connecting…"
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.busy = false
		if err != nil {
			a.status = "Connection failed"
		}
		a.mu.Unlock()
	}()
	c := a.config()
	if err = validateConfig(&c); err != nil {
		return
	}
	s, e := discordgo.New("Bot " + c.Token)
	if e != nil {
		return e
	}
	s.Client.Timeout = 25 * time.Second
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMembers | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsGuildMessageReactions | discordgo.IntentsGuildVoiceStates | discordgo.IntentAutoModerationExecution | discordgo.IntentGuildScheduledEvents
	s.State.TrackPresences = false
	s.State.TrackVoice = true
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		a.mu.Lock()
		if a.session == s {
			a.ready = true
			a.status = "Connected as " + r.User.Username
		}
		a.mu.Unlock()
		a.log("Connected to Discord")
	})
	s.AddHandler(func(s *discordgo.Session, e *discordgo.Connect) {
		a.mu.Lock()
		if a.session == s && s.State.User != nil {
			a.ready = true
			a.status = "Connected"
		}
		a.mu.Unlock()
	})
	s.AddHandler(func(s *discordgo.Session, e *discordgo.Disconnect) {
		a.mu.Lock()
		if a.session == s {
			a.ready = false
			a.status = "Reconnecting…"
		}
		a.mu.Unlock()
	})
	s.AddHandler(a.onInteraction)
	s.AddHandler(a.onMessage)
	s.AddHandler(a.onRulesReaction)
	s.AddHandler(a.onVoiceState)
	s.AddHandler(a.onNativeAutoMod)
	s.AddHandler(a.onCommunityEventUpdate)
	s.AddHandler(a.onCommunityEventDelete)
	s.AddHandler(func(s *discordgo.Session, e *discordgo.MessageUpdate) {
		c := a.config()
		if e.GuildID != c.GuildID || (!c.BlockInvites && !c.BlockLinks && !c.BanSlurs && !c.LimitSwearing && c.TrapChannelID == "") {
			return
		}
		m, err := s.ChannelMessage(e.ChannelID, e.ID)
		if err != nil {
			a.log("Could not inspect edited message: " + a.errorText(err))
			return
		}
		m.GuildID = e.GuildID
		a.moderateMessage(s, &discordgo.MessageCreate{Message: m}, false)
	})
	s.AddHandler(func(s *discordgo.Session, e *discordgo.GuildMemberAdd) { a.onJoin(s, e.Member) })
	s.AddHandler(func(s *discordgo.Session, e *discordgo.GuildMemberUpdate) {
		if e.BeforeUpdate != nil && e.BeforeUpdate.Pending && !e.Pending {
			a.onScreened(s, e.Member)
		}
	})
	a.mu.Lock()
	a.session = s
	a.mu.Unlock()
	cleanup := func() { a.mu.Lock(); a.session = nil; a.ready = false; a.mu.Unlock(); s.Close() }
	if e = s.Open(); e != nil {
		cleanup()
		return fmt.Errorf("%s. Check the token and enable Server Members Intent and Message Content Intent in the Developer Portal", a.errorText(e))
	}
	if _, e = s.Guild(c.GuildID); e != nil {
		cleanup()
		return fmt.Errorf("Cannot access that server: %s. Invite the bot to the server first", a.errorText(e))
	}
	if _, e = s.ApplicationCommandBulkOverwrite(s.State.User.ID, c.GuildID, commands()); e != nil {
		cleanup()
		return fmt.Errorf("Cannot register commands: %s", a.errorText(e))
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.mu.Lock()
	a.ready = true
	a.status = "Connected as " + s.State.User.Username
	a.mu.Unlock()
	if c.BotName != "" && s.State.User.Username != c.BotName {
		if _, e = s.UserUpdate(c.BotName, "", ""); e != nil {
			a.log("Could not set bot name: " + a.errorText(e))
		}
	}
	go a.youtubeLoop(ctx, s)
	go a.reconcileRulesReactions(ctx, s)
	go a.communityLoop(ctx, s)
	a.log("Moderation commands registered. Auto-role and video watcher active.")
	if _, e = a.inspect(s); e != nil {
		a.log("Setup check: " + a.errorText(e))
	}
	return nil
}
func (a *App) stop() {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.Lock()
	s := a.session
	a.session = nil
	a.ready = false
	a.status = "Stopped"
	a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	if s != nil {
		s.Close()
		a.log("Bot stopped")
	}
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	a.updateRoutes(mux)
	a.embedRoutes(mux)
	a.communityRoutes(mux)
	mux.HandleFunc("/community-panel.js", func(w http.ResponseWriter, r *http.Request) {
		b, _ := assets.ReadFile("community-panel.js")
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(b)
	})
	mux.HandleFunc("/embed-editor.js", func(w http.ResponseWriter, r *http.Request) {
		b, _ := assets.ReadFile("embed-editor.js")
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(b)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, _ := assets.ReadFile("panel.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, strings.ReplaceAll(string(b), "__CSRF__", a.csrf))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		c := a.cfg
		tokenSet := c.Token != ""
		c.Token = ""
		logs := append([]string{}, a.logs...)
		payload := map[string]any{"config": c, "token_set": tokenSet, "running": a.session != nil, "ready": a.ready, "busy": a.busy, "status": a.status, "logs": logs, "last_check": a.lastCheck, "last_notify": a.lastNotify, "version": version}
		a.mu.RUnlock()
		if a.updater != nil {
			payload["update"] = a.updater.snapshot()
		}
		sendJSON(w, payload)
	})
	mux.HandleFunc("/api/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		a.lifecycle.Lock()
		defer a.lifecycle.Unlock()
		a.mu.RLock()
		running := a.session != nil
		a.mu.RUnlock()
		if running {
			apiError(w, errors.New("Stop the bot before saving settings"))
			return
		}
		var c Config
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&c); e != nil {
			apiError(w, e)
			return
		}
		c.Community = a.config().Community
		if c.Token == "" {
			c.Token = a.config().Token
		}
		if e := validateConfig(&c); e != nil {
			apiError(w, e)
			return
		}
		if e := atomicJSON(filepath.Join(a.dir, "config.json"), c); e != nil {
			apiError(w, e)
			return
		}
		a.mu.Lock()
		a.cfg = c
		a.mu.Unlock()
		a.log("Settings saved")
		sendJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		if e := a.start(); e != nil {
			a.log(a.errorText(e))
			apiError(w, errors.New(a.errorText(e)))
			return
		}
		sendJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) { a.stop(); sendJSON(w, map[string]any{"ok": true}) })
	mux.HandleFunc("/api/inspect", func(w http.ResponseWriter, r *http.Request) {
		s, e := a.current()
		if e != nil {
			apiError(w, e)
			return
		}
		v, e := a.inspect(s)
		if e != nil {
			apiError(w, errors.New(a.errorText(e)))
			return
		}
		sendJSON(w, v)
	})
	mux.HandleFunc("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		s, e := a.current()
		if e != nil {
			apiError(w, e)
			return
		}
		var p struct {
			Name   string `json:"name"`
			Avatar string `json:"avatar"`
			Reset  bool   `json:"reset"`
		}
		if e = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&p); e != nil {
			apiError(w, e)
			return
		}
		name := strings.TrimSpace(p.Name)
		if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 32 {
			apiError(w, errors.New("Name must have 2–32 characters"))
			return
		}
		if p.Avatar != "" {
			if e = validateAvatar(p.Avatar); e != nil {
				apiError(w, e)
				return
			}
		}
		a.lifecycle.Lock()
		defer a.lifecycle.Unlock()
		var payload = map[string]any{"username": name}
		if p.Reset {
			payload["avatar"] = nil
		} else if p.Avatar != "" {
			payload["avatar"] = p.Avatar
		}
		_, e = s.RequestWithBucketID("PATCH", discordgo.EndpointUsers+"@me", payload, discordgo.EndpointUsers+"@me")
		if e != nil {
			apiError(w, errors.New(a.errorText(e)))
			return
		}
		c := a.config()
		c.BotName = name
		if e = atomicJSON(filepath.Join(a.dir, "config.json"), c); e != nil {
			apiError(w, fmt.Errorf("Discord profile updated, but settings could not be saved: %v", e))
			return
		}
		a.mu.Lock()
		a.cfg = c
		a.mu.Unlock()
		a.log("Discord name/avatar updated")
		a.mu.Lock()
		a.status = "Connected as " + name
		a.mu.Unlock()
		sendJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/test-video", func(w http.ResponseWriter, r *http.Request) {
		s, e := a.current()
		if e == nil {
			e = a.testVideo(s)
		}
		if e != nil {
			apiError(w, errors.New(a.errorText(e)))
			return
		}
		sendJSON(w, map[string]any{"ok": true})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'")
		if r.Host != a.host {
			http.Error(w, "Invalid host", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Panel-Key")), []byte(a.csrf)) != 1 {
				http.Error(w, "Invalid panel key", 403)
				return
			}
			if a.updater != nil && a.updater.snapshot().Phase == "restarting" && r.URL.Path != "/api/status" {
				http.Error(w, "The bot is restarting to install an update", http.StatusServiceUnavailable)
				return
			}
			if r.URL.Path != "/api/status" && r.URL.Path != "/api/embeds" && r.URL.Path != "/api/community" && r.Method != "POST" {
				http.Error(w, "POST required", 405)
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/community-") && r.URL.Path != "/api/community-save" && r.URL.Path != "/api/community-restore" {
			a.lifecycle.Lock()
			defer a.lifecycle.Unlock()
		}
		mux.ServeHTTP(w, r)
	})
}
func sendJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, e error) {
	w.WriteHeader(400)
	sendJSON(w, map[string]any{"error": e.Error()})
}
func validateAvatar(data string) error {
	p := strings.SplitN(data, ",", 2)
	if len(p) != 2 {
		return errors.New("Invalid avatar")
	}
	b, e := base64.StdEncoding.DecodeString(p[1])
	if e != nil || len(b) > 2<<20 {
		return errors.New("Avatar must be under 2 MB")
	}
	typ := http.DetectContentType(b)
	if typ != "image/png" && typ != "image/jpeg" && typ != "image/gif" {
		return errors.New("Use a PNG, JPG or GIF avatar")
	}
	if p[0] != "data:"+typ+";base64" {
		return errors.New("Invalid avatar image type")
	}
	return nil
}
