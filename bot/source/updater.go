package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const updateManifestURL = "https://raw.githubusercontent.com/MubbleYT/mubbles-workshop/main/bot/update.json"
const updateReleasePrefix = "https://github.com/MubbleYT/mubbles-workshop/releases/download/bot-v"
const maxUpdateBytes int64 = 64 << 20

type UpdateManifest struct {
	Schema  int    `json:"schema"`
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	Notes   string `json:"notes"`
}
type UpdateStatus struct {
	Current     string `json:"current"`
	Latest      string `json:"latest,omitempty"`
	Available   bool   `json:"available"`
	Supported   bool   `json:"supported"`
	Busy        bool   `json:"busy"`
	Phase       string `json:"phase"`
	Checked     string `json:"checked,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Error       string `json:"error,omitempty"`
	Downloaded  int64  `json:"downloaded"`
	Size        int64  `json:"size"`
	LastInstall string `json:"last_install,omitempty"`
}
type BotUpdater struct {
	mu     sync.Mutex
	app    *App
	client *http.Client
	state  UpdateStatus
}

func newBotUpdater(a *App) *BotUpdater {
	return &BotUpdater{app: a, state: UpdateStatus{Current: version, Supported: runtime.GOOS == "windows", Phase: "idle"},
		client: &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > 5 || !trustedUpdateRedirect(r.URL) {
				return errors.New("Update redirect was not a trusted GitHub download")
			}
			return nil
		}}}
}
func trustedUpdateRedirect(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	case "github.com":
		return strings.HasPrefix(u.Path, "/MubbleYT/mubbles-workshop/releases/download/bot-v")
	}
	return false
}
func versionParts(v string) ([3]uint64, error) {
	var out [3]uint64
	parts := strings.Split(v, ".")
	if len(parts) != 3 || len(v) > 32 {
		return out, errors.New("Invalid stable update version")
	}
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return out, errors.New("Invalid stable update version")
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return out, errors.New("Invalid stable update version")
			}
		}
		n, e := strconv.ParseUint(p, 10, 32)
		if e != nil {
			return out, e
		}
		out[i] = n
	}
	return out, nil
}
func newerVersion(candidate, current string) bool {
	a, e := versionParts(candidate)
	if e != nil {
		return false
	}
	b, e := versionParts(current)
	if e != nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
func validUpdateManifest(m UpdateManifest) error {
	if _, e := versionParts(m.Version); e != nil {
		return e
	}
	hash, e := hex.DecodeString(m.SHA256)
	if m.Schema != 1 || e != nil || len(hash) != 32 || m.SHA256 != strings.ToLower(m.SHA256) || m.Size < 512 || m.Size > maxUpdateBytes || len(m.Notes) > 8000 {
		return errors.New("Invalid update metadata")
	}
	if m.URL != updateReleasePrefix+m.Version+"/MubbleDiscordBot.exe" {
		return errors.New("Update executable must come from the Mubbles Workshop bot release")
	}
	return nil
}
func (u *BotUpdater) snapshot() UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	s := u.state
	result := readUpdateResult(u.app.dir)
	if result.Success && result.Version == version {
		s.LastInstall = result.Time
	}
	if !result.Success && result.Error != "" && s.Error == "" {
		s.Error = "Previous update failed: " + result.Error
	}
	return s
}
func (u *BotUpdater) phase(phase string) { u.mu.Lock(); u.state.Phase = phase; u.mu.Unlock() }
func (u *BotUpdater) begin() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.state.Busy {
		return errors.New("An update check or installation is already running")
	}
	u.state.Busy = true
	u.state.Error = ""
	u.state.Phase = "checking"
	u.state.Downloaded = 0
	return nil
}
func (u *BotUpdater) finish(err error) {
	u.mu.Lock()
	if u.state.Phase != "restarting" {
		u.state.Busy = false
		u.state.Phase = "idle"
	}
	if err != nil {
		u.state.Error = err.Error()
	}
	u.mu.Unlock()
	if err != nil {
		u.app.log("Online update: " + err.Error())
	}
}
func (u *BotUpdater) fetchManifest(ctx context.Context) (UpdateManifest, error) {
	var m UpdateManifest
	req, e := http.NewRequestWithContext(ctx, "GET", updateManifestURL+"?check="+strconv.FormatInt(time.Now().Unix()/60, 10), nil)
	if e != nil {
		return m, e
	}
	req.Header.Set("User-Agent", "MubbleDiscordBot/"+version)
	req.Header.Set("Cache-Control", "no-cache")
	resp, e := u.client.Do(req)
	if e != nil {
		return m, errors.New("Cannot reach the online update channel; your current bot will keep running")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return m, fmt.Errorf("Update channel returned HTTP %d; your current bot will keep running", resp.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, (32<<10)+1))
	if e != nil || len(data) > 32<<10 || json.Unmarshal(data, &m) != nil {
		return m, errors.New("Invalid online update response")
	}
	return m, validUpdateManifest(m)
}
func (u *BotUpdater) check(ctx context.Context) error {
	if e := u.begin(); e != nil {
		return e
	}
	e := u.perform(ctx, false, false)
	u.finish(e)
	return e
}
func (u *BotUpdater) install() error {
	if runtime.GOOS != "windows" {
		return errors.New("Installing updates is supported by the Windows executable")
	}
	if e := u.begin(); e != nil {
		return e
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		u.finish(u.perform(ctx, true, true))
	}()
	return nil
}
func (u *BotUpdater) perform(ctx context.Context, install, manual bool) error {
	m, e := u.fetchManifest(ctx)
	if e != nil {
		return e
	}
	u.mu.Lock()
	u.state.Latest = m.Version
	u.state.Available = newerVersion(m.Version, version)
	u.state.Notes = m.Notes
	u.state.Size = m.Size
	u.state.Checked = time.Now().Format(time.RFC3339)
	u.mu.Unlock()
	if !newerVersion(m.Version, version) || !install {
		return nil
	}
	result := readUpdateResult(u.app.dir)
	if !manual && !result.Success && result.Version == m.Version {
		return errors.New("This version failed its previous installation; automatic retry is paused until a new release. You can retry from the panel")
	}
	if runtime.GOOS != "windows" {
		return errors.New("Online installation requires Windows")
	}
	u.phase("downloading")
	root := filepath.Join(u.app.dir, "updates")
	if e = os.MkdirAll(root, 0700); e != nil {
		return e
	}
	dir, e := os.MkdirTemp(root, "pending-")
	if e != nil {
		return e
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()
	staged := filepath.Join(dir, "update.exe")
	if e = u.download(ctx, m, staged); e != nil {
		return e
	}
	if !manual && !u.app.config().AutoUpdate {
		return nil
	}
	target, e := os.Executable()
	if e != nil {
		return e
	}
	target, e = filepath.EvalSymlinks(target)
	if e != nil {
		return e
	}
	if e = preflightTarget(target); e != nil {
		return e
	}
	nonce := make([]byte, 32)
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	u.app.lifecycle.Lock()
	u.app.mu.RLock()
	resume := u.app.session != nil
	u.app.mu.RUnlock()
	plan := UpdatePlan{Target: target, Staged: staged, ParentPID: os.Getpid(), SHA256: m.SHA256, Size: m.Size, Version: m.Version, Nonce: hex.EncodeToString(nonce), ResumeBot: resume}
	planPath := filepath.Join(dir, "plan.json")
	if e = atomicJSON(planPath, plan); e == nil {
		e = launchUpdateHelper(planPath, plan)
	}
	if e == nil {
		keep = true
		u.phase("restarting")
		u.app.log("Installing bot " + m.Version + "; the panel will reopen after restart")
		u.app.shutdownOnce.Do(func() {
			if u.app.shutdown != nil {
				close(u.app.shutdown)
			}
		})
	}
	u.app.lifecycle.Unlock()
	return e
}
func (u *BotUpdater) download(ctx context.Context, m UpdateManifest, path string) error {
	if e := validUpdateManifest(m); e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "GET", m.URL, nil)
	if e != nil {
		return e
	}
	req.Header.Set("User-Agent", "MubbleDiscordBot/"+version)
	resp, e := u.client.Do(req)
	if e != nil {
		return errors.New("Update download failed; the current executable is unchanged")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Update download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 0 && resp.ContentLength != m.Size {
		return errors.New("Update download size did not match")
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	good := false
	defer func() {
		f.Close()
		if !good {
			os.Remove(path)
		}
	}()
	buffer := make([]byte, 64<<10)
	limited := io.LimitReader(resp.Body, m.Size+1)
	var total int64
	for {
		n, readErr := limited.Read(buffer)
		if n > 0 {
			if _, e = f.Write(buffer[:n]); e != nil {
				return e
			}
			total += int64(n)
			u.mu.Lock()
			u.state.Downloaded = total
			u.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return errors.New("Update download was interrupted")
		}
		if e = ctx.Err(); e != nil {
			return e
		}
	}
	if total != m.Size {
		return errors.New("Update download was incomplete or exceeded its expected size")
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	u.phase("verifying")
	if e = verifyUpdateFile(path, m.SHA256, m.Size); e != nil {
		return e
	}
	good = true
	return nil
}
func verifyUpdateFile(path, expected string, size int64) error {
	info, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Size() != size || size > maxUpdateBytes {
		return errors.New("Update executable has the wrong file type or size")
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	h := sha256.New()
	_, e = io.Copy(h, f)
	f.Close()
	if e != nil || hex.EncodeToString(h.Sum(nil)) != expected {
		return errors.New("Update checksum did not match; installation was cancelled")
	}
	p, e := pe.Open(path)
	if e != nil {
		return errors.New("Update was not a Windows executable")
	}
	defer p.Close()
	optional, ok := p.OptionalHeader.(*pe.OptionalHeader64)
	if !ok || p.Machine != pe.IMAGE_FILE_MACHINE_AMD64 || optional.Subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_CUI || p.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || p.Characteristics&pe.IMAGE_FILE_DLL != 0 {
		return errors.New("Update was not a Windows 64-bit console application")
	}
	return nil
}
func preflightTarget(target string) error {
	info, e := os.Lstat(target)
	if e != nil {
		return e
	}
	if !filepath.IsAbs(target) || !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(target), ".exe") {
		return errors.New("Cannot replace this executable")
	}
	probe, e := os.CreateTemp(filepath.Dir(target), ".mubble-write-check-")
	if e != nil {
		return errors.New("The bot folder is read-only. Move the executable to a writable folder before updating")
	}
	name := probe.Name()
	probe.Close()
	return os.Remove(name)
}
func (u *BotUpdater) loop(ctx context.Context) {
	if runtime.GOOS != "windows" {
		return
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if u.app.config().AutoUpdate {
			if e := u.begin(); e == nil {
				u.finish(u.perform(ctx, true, false))
			}
		}
		timer.Reset(6 * time.Hour)
	}
}
func (a *App) updateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/update-check", func(w http.ResponseWriter, r *http.Request) {
		if a.updater == nil {
			apiError(w, errors.New("Updater is unavailable"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		if e := a.updater.check(ctx); e != nil {
			apiError(w, e)
			return
		}
		sendJSON(w, a.updater.snapshot())
	})
	mux.HandleFunc("/api/update-install", func(w http.ResponseWriter, r *http.Request) {
		if a.updater == nil {
			apiError(w, errors.New("Updater is unavailable"))
			return
		}
		if e := a.updater.install(); e != nil {
			apiError(w, e)
			return
		}
		sendJSON(w, a.updater.snapshot())
	})
}

type UpdatePlan struct {
	Target    string `json:"target"`
	Staged    string `json:"staged"`
	ParentPID int    `json:"parent_pid"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Version   string `json:"version"`
	Nonce     string `json:"nonce"`
	ResumeBot bool   `json:"resume_bot"`
}
type UpdateHealth struct {
	Version string `json:"version"`
	Nonce   string `json:"nonce"`
	PID     int    `json:"pid,omitempty"`
}

func hasArgument(args []string, value string) bool {
	for _, a := range args {
		if a == value {
			return true
		}
	}
	return false
}
func loadUpdatePlan(planPath string) (UpdatePlan, string, error) {
	var p UpdatePlan
	configDir, e := os.UserConfigDir()
	if e != nil {
		return p, "", e
	}
	appDir := filepath.Join(configDir, "MubbleDiscordBot")
	dir := filepath.Dir(planPath)
	rel, e := filepath.Rel(filepath.Join(appDir, "updates"), dir)
	if e != nil || !filepath.IsAbs(planPath) || filepath.Base(planPath) != "plan.json" || !strings.HasPrefix(rel, "pending-") || strings.ContainsAny(rel, "/\\") {
		return p, appDir, errors.New("Update plan is outside the bot's update folder")
	}
	b, e := os.ReadFile(planPath)
	if e != nil {
		return p, appDir, e
	}
	if len(b) > 16<<10 || json.Unmarshal(b, &p) != nil {
		return p, appDir, errors.New("Invalid update plan")
	}
	nonce, e := hex.DecodeString(p.Nonce)
	if e != nil || len(nonce) != 32 || p.ParentPID <= 0 || p.Staged != filepath.Join(dir, "update.exe") || p.Target == p.Staged {
		return p, appDir, errors.New("Invalid update plan binding")
	}
	if e = validUpdateManifest(UpdateManifest{Schema: 1, Version: p.Version, URL: updateReleasePrefix + p.Version + "/MubbleDiscordBot.exe", SHA256: p.SHA256, Size: p.Size}); e != nil {
		return p, appDir, e
	}
	if e = preflightTarget(p.Target); e != nil {
		return p, appDir, e
	}
	return p, appDir, nil
}
func copyUpdateFile(source, target string) error {
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, io.LimitReader(in, maxUpdateBytes+1))
	if e == nil {
		e = out.Sync()
	}
	closeErr := out.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		os.Remove(target)
	}
	return e
}
func launchUpdateHelper(path string, p UpdatePlan) error {
	current, e := os.Executable()
	if e != nil {
		return e
	}
	helper := filepath.Join(filepath.Dir(path), "helper.exe")
	if e = copyUpdateFile(current, helper); e != nil {
		return e
	}
	cmd := exec.Command(helper, "--apply-update", path)
	hideUpdateHelper(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ready := filepath.Join(filepath.Dir(path), "ready.json")
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b, _ := os.ReadFile(ready)
			var health UpdateHealth
			if json.Unmarshal(b, &health) == nil && health.Nonce == p.Nonce {
				return nil
			}
		case e := <-done:
			return fmt.Errorf("Update helper could not start: %v", e)
		case <-timer.C:
			cmd.Process.Kill()
			return errors.New("Update helper did not become ready; the current bot will keep running")
		}
	}
}
func writeUpdateHealth(appDir string, args []string) error {
	for i, a := range args {
		if a != "--update-health" || i+1 >= len(args) {
			continue
		}
		path := args[i+1]
		plan, eAppDir, e := loadUpdatePlan(filepath.Join(filepath.Dir(path), "plan.json"))
		if e != nil {
			return e
		}
		if eAppDir != appDir || filepath.Base(path) != "health.json" || plan.Version != version {
			return errors.New("Invalid update startup marker")
		}
		return atomicJSON(path, UpdateHealth{Version: version, Nonce: plan.Nonce, PID: os.Getpid()})
	}
	return nil
}

type UpdateResult struct {
	Version string `json:"version"`
	Success bool   `json:"success"`
	Time    string `json:"time"`
	Error   string `json:"error,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

func readUpdateResult(appDir string) UpdateResult {
	var r UpdateResult
	b, e := os.ReadFile(filepath.Join(appDir, "update-result.json"))
	if e == nil && len(b) < 16<<10 {
		json.Unmarshal(b, &r)
	}
	return r
}
