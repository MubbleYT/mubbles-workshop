package main

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestSlurBoundariesNormalizationAndProfanitySeparation(t *testing.T) {
	c := defaultConfig()
	p, e := buildLanguagePolicy(c)
	if e != nil {
		t.Fatal(e)
	}
	for _, text := range []string{"faggot", "FAGGOTS!", "n1gger", "ｎｉｇｇｅｒ", "ni\u200bggers", "SCHWUCHTEL!"} {
		if p.slurs.count(text) != 1 {
			t.Errorf("Expected one slur match for %q", text)
		}
	}
	for _, text := range []string{"fuck!", "shit", "cunt", "gay", "queer", "trans", "Scunthorpe", "class assignment", "cocktail", "snigger", "faggotry"} {
		if p.slurs.count(text) != 0 {
			t.Errorf("False slur positive: %q", text)
		}
	}
	if p.swears.count("Fuck! This is fucking bullshit.") != 3 {
		t.Fatal("Profanity inflections and punctuation should count correctly")
	}
	if p.swears.count("class assignment, a cocktail and Scunthorpe") != 0 {
		t.Fatal("Swear substrings must not match innocent words")
	}
}
func TestSwearRollingCountAndEditedMessages(t *testing.T) {
	a := &App{}
	c := defaultConfig()
	c.GuildID = "guild"
	now := time.Now()
	if n := a.recordSwears(c, "user", "msg1", 3, now); n != 3 {
		t.Fatal(n)
	}
	if n := a.recordSwears(c, "user", "msg1", 2, now.Add(time.Second)); n != 2 {
		t.Fatal("Edits must replace rather than add", n)
	}
	if n := a.recordSwears(c, "user", "msg2", 3, now.Add(time.Second)); n != 5 {
		t.Fatal(n)
	}
	if n := a.recordSwears(c, "other", "msg3", 1, now.Add(time.Second)); n != 1 {
		t.Fatal("Users must have independent counts", n)
	}
	if n := a.recordSwears(c, "user", "msg1", 0, now.Add(2*time.Second)); n != 3 {
		t.Fatal("Removing profanity should remove its count", n)
	}
	if n := a.recordSwears(c, "user", "new", 1, now.Add(62*time.Second)); n != 1 {
		t.Fatal("Old counts must expire", n)
	}
}

type languageTransport struct {
	paths      []string
	methods    []string
	failDelete bool
}

func (f *languageTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.paths = append(f.paths, r.URL.Path)
	f.methods = append(f.methods, r.Method)
	status := 200
	body := "{}"
	if r.Method == "DELETE" && f.failDelete {
		status = 403
		body = `{"message":"Missing Permissions","code":50013}`
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}
func languageSession(t *testing.T, tr *languageTransport) (*discordgo.Session, *App) {
	t.Helper()
	s, e := discordgo.New("Bot fake")
	if e != nil {
		t.Fatal(e)
	}
	s.Client = &http.Client{Transport: tr}
	s.State.User = &discordgo.User{ID: "bot"}
	role := &discordgo.Role{ID: "guild", Permissions: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages}
	if e = s.State.GuildAdd(&discordgo.Guild{ID: "guild", OwnerID: "owner", Roles: []*discordgo.Role{role}, Members: []*discordgo.Member{{GuildID: "guild", User: &discordgo.User{ID: "user"}}, {GuildID: "guild", User: &discordgo.User{ID: "mod"}, Roles: []string{"modrole"}}}, Channels: []*discordgo.Channel{{ID: "channel", GuildID: "guild", Type: discordgo.ChannelTypeGuildText}}}); e != nil {
		t.Fatal(e)
	}
	g, e := s.State.Guild("guild")
	if e != nil {
		t.Fatal(e)
	}
	g.Roles = append(g.Roles, &discordgo.Role{ID: "modrole", Position: 3, Permissions: discordgo.PermissionManageMessages})
	c := defaultConfig()
	c.GuildID = "guild"
	c.BlockInvites = false
	c.AntiSpam = false
	store, _ := openStore(filepath.Join(t.TempDir(), "state.json"))
	return s, &App{store: store, cfg: c, spam: map[string][]time.Time{}, cooldown: map[string]time.Time{}}
}
func msg(id, user, content string) *discordgo.MessageCreate {
	return &discordgo.MessageCreate{Message: &discordgo.Message{ID: id, GuildID: "guild", ChannelID: "channel", Author: &discordgo.User{ID: user}, Content: content}}
}
func TestSlurDeletesAndBansIncludingModerator(t *testing.T) {
	for _, user := range []string{"user", "mod"} {
		tr := &languageTransport{}
		s, a := languageSession(t, tr)
		a.onMessage(s, msg("m1", user, "faggot"))
		if len(tr.paths) != 2 || tr.methods[0] != "DELETE" || tr.methods[1] != "PUT" || !strings.HasSuffix(tr.paths[1], "/guilds/guild/bans/"+user) {
			t.Fatalf("Expected delete + ban for %s: %+v", user, tr)
		}
		a.moderateMessage(s, msg("m2", user, "faggot"), false)
		if len(tr.paths) != 3 || tr.methods[2] != "DELETE" {
			t.Fatal("Buffered edits/messages should be deleted without duplicate bans")
		}
	}
}
func TestSlurBanStillRunsWhenMessageDeletionFails(t *testing.T) {
	tr := &languageTransport{failDelete: true}
	s, a := languageSession(t, tr)
	a.onMessage(s, msg("m1", "user", "faggot"))
	if len(tr.methods) != 2 || tr.methods[1] != "PUT" {
		t.Fatal("Deletion failure must not suppress ban")
	}
}
func TestOrdinarySwearsAllowedUntilLimitAndNeverBanned(t *testing.T) {
	tr := &languageTransport{}
	s, a := languageSession(t, tr)
	a.onMessage(s, msg("m1", "user", "fuck shit cunt"))
	if len(tr.paths) != 0 {
		t.Fatal("Three swears must be allowed")
	}
	a.moderateMessage(s, msg("m1", "user", "fuck shit cunt"), false)
	if len(tr.paths) != 0 {
		t.Fatal("Editing the same content must not double-count")
	}
	a.onMessage(s, msg("m2", "user", "fuck shit cunt"))
	if len(tr.paths) != 0 {
		t.Fatal("Six rolling swears must be allowed")
	}
	a.onMessage(s, msg("m3", "user", "fuck shit cunt"))
	if len(tr.paths) != 1 || tr.methods[0] != "DELETE" {
		t.Fatal("The rolling limit must only delete")
	}
	tr2 := &languageTransport{}
	s2, a2 := languageSession(t, tr2)
	a2.onMessage(s2, msg("m4", "user", "fuck shit cunt bitch"))
	if len(tr2.paths) != 1 || tr2.methods[0] != "DELETE" {
		t.Fatal("Per-message limit must only delete")
	}
}
func TestLegacyConfigKeepsSettingsAndAddsNewDefaults(t *testing.T) {
	c := defaultConfig()
	old := `{"token":"keep-token","guild_id":"123456789012345678","block_links":true}`
	if e := json.Unmarshal([]byte(old), &c); e != nil {
		t.Fatal(e)
	}
	if c.Token != "keep-token" || !c.BlockLinks || !c.BanSlurs || c.SwearsPerMessage != 3 || c.SwearsPerWindow != 8 {
		t.Fatal("Legacy settings/default migration failed")
	}
}
func TestLanguageListValidation(t *testing.T) {
	c := defaultConfig()
	c.SlurTerms = ""
	if _, e := buildLanguagePolicy(c); e == nil {
		t.Fatal("Enabled slur filter needs a list")
	}
	c.BanSlurs = false
	if _, e := buildLanguagePolicy(c); e != nil {
		t.Fatal(e)
	}
	if _, e := newWordMatcher(".*"); e == nil {
		t.Fatal("Regex must not be treated as a language rule")
	}
}
