package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestWarningPersistenceAndRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, e := openStore(path)
	if e != nil {
		t.Fatal(e)
	}
	w, e := s.warn("guild", "member", "mod", "Spam")
	if e != nil {
		t.Fatal(e)
	}
	s, e = openStore(path)
	if e != nil {
		t.Fatal(e)
	}
	if got := s.warnings("guild", "member"); len(got) != 1 || got[0].Reason != "Spam" {
		t.Fatal(got)
	}
	if len(s.warnings("other", "member")) != 0 {
		t.Fatal("Warnings crossed server boundaries")
	}
	if e = s.removeWarning("guild", "member", w.ID); e != nil {
		t.Fatal(e)
	}
	s, e = openStore(path)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.warnings("guild", "member")) != 0 {
		t.Fatal("Removal not persisted")
	}
	w2, e := s.warn("guild", "member", "mod", "Second")
	if e != nil || w2.ID == w.ID {
		t.Fatal("Warning IDs must not be reused")
	}
}
func TestFeedBaselineAndDuplicatePrevention(t *testing.T) {
	v := []Video{{ID: "abcdefghijk"}, {ID: "12345678901"}}
	f, p := planVideos(FeedState{}, v)
	if !f.Initialized || len(p) != 0 || len(f.Seen) != 2 {
		t.Fatal("First run must establish a quiet baseline")
	}
	v = append(v, Video{ID: "ABCDEFGHIJK"})
	f, p = planVideos(f, v)
	if len(p) != 1 || p[0].ID != "ABCDEFGHIJK" {
		t.Fatal(p)
	}
	f.Seen = append(f.Seen, p[0].ID)
	_, p = planVideos(f, v)
	if len(p) != 0 {
		t.Fatal("Already delivered videos must not be repeated")
	}
	s, _ := openStore(filepath.Join(t.TempDir(), "state.json"))
	if e := s.saveFeed("channel", f); e != nil {
		t.Fatal(e)
	}
	s2, e := openStore(s.path)
	if e != nil {
		t.Fatal(e)
	}
	_, p = planVideos(s2.feed("channel"), v)
	if len(p) != 0 {
		t.Fatal("Restart must not repeat videos")
	}
}
func TestParseNamespacedAtomFeed(t *testing.T) {
	rss := `<feed xmlns="http://www.w3.org/2005/Atom" xmlns:yt="http://www.youtube.com/xml/schemas/2015"><entry><yt:videoId>ABCDEFGHIJK</yt:videoId><title>New &amp; shiny</title><published>2026-10-02T10:00:00Z</published><author><name>Mubble</name></author></entry><entry><yt:videoId>abcdefghijk</yt:videoId><title>Older</title><published>2026-10-01T10:00:00Z</published></entry></feed>`
	videos, e := parseFeed(strings.NewReader(rss))
	if e != nil {
		t.Fatal(e)
	}
	if len(videos) != 2 || videos[0].ID != "abcdefghijk" || videos[1].Title != "New & shiny" {
		t.Fatal(videos)
	}
	if _, e = parseFeed(strings.NewReader(`<html>fail</html>`)); e == nil {
		t.Fatal("HTML must not be accepted as an empty feed")
	}
}
func TestMentionsAndRetryNonce(t *testing.T) {
	c := defaultConfig()
	c.NotifyRoleID = "123456789012345678"
	c.NotifyChannelID = "223456789012345678"
	c.NotifyText = "@everyone new upload"
	v := Video{ID: "ABCDEFGHIJK", Published: time.Now()}
	p := videoPayload(c, v, false)
	m := p["allowed_mentions"].(*discordgo.MessageAllowedMentions)
	if len(m.Parse) != 0 || len(m.Roles) != 1 || m.Roles[0] != c.NotifyRoleID {
		t.Fatal(m)
	}
	p2 := videoPayload(c, v, false)
	if p["nonce"] != p2["nonce"] || p["enforce_nonce"] != true {
		t.Fatal("Retries should use the same Discord deduplication nonce")
	}
	test := videoPayload(c, v, true)
	tm := test["allowed_mentions"].(*discordgo.MessageAllowedMentions)
	if len(tm.Roles) != 0 || strings.Contains(test["content"].(string), "<@&") {
		t.Fatal("Test notifications must never ping a role")
	}
}
func TestConfigValidation(t *testing.T) {
	c := defaultConfig()
	c.Token = "fake.token"
	c.GuildID = "123456789012345678"
	if e := validateConfig(&c); e != nil {
		t.Fatal(e)
	}
	c.YouTubeID = "@mubbleyt"
	c.NotifyChannelID = "223456789012345678"
	if e := validateConfig(&c); e == nil {
		t.Fatal("Handles require a channel ID")
	}
	c.YouTubeID = "https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv/"
	if e := validateConfig(&c); e != nil {
		t.Fatal(e)
	}
	c.NotifyRoleID = c.GuildID
	if e := validateConfig(&c); e == nil {
		t.Fatal("@everyone role is not an upload role")
	}
}
func TestPanelProtectsSecretsAndMutations(t *testing.T) {
	a := &App{cfg: Config{Token: "super-secret"}, csrf: "panel-secret", host: "127.0.0.1:9999"}
	h := a.routes()
	req := httptest.NewRequest("GET", "http://127.0.0.1:9999/api/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("Missing panel key must be rejected")
	}
	req.Header.Set("X-Panel-Key", a.csrf)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), a.cfg.Token) {
		t.Fatal("Token must not be returned in status")
	}
	req = httptest.NewRequest("GET", "http://127.0.0.1:9999/api/stop", nil)
	req.Header.Set("X-Panel-Key", a.csrf)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 405 {
		t.Fatal("Mutations must require POST")
	}
	req = httptest.NewRequest("GET", "http://evil.example/", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("Foreign Host must be blocked")
	}
}
func TestHierarchyAndPermissions(t *testing.T) {
	roles := []*discordgo.Role{{ID: "guild", Permissions: discordgo.PermissionSendMessages}, {ID: "mod", Position: 10, Permissions: discordgo.PermissionKickMembers}, {ID: "member", Position: 2}}
	m := &discordgo.Member{Roles: []string{"member"}}
	if rolePosition(m, roles) != 2 {
		t.Fatal("Wrong role position")
	}
	if hasPermission(guildPermissions(m, "guild", roles), discordgo.PermissionKickMembers) {
		t.Fatal("Member must not inherit moderator permission")
	}
	m.Roles = append(m.Roles, "mod")
	if rolePosition(m, roles) != 10 || !hasPermission(guildPermissions(m, "guild", roles), discordgo.PermissionKickMembers) {
		t.Fatal("Moderator role not applied")
	}
	if !hasPermission(discordgo.PermissionAdministrator, discordgo.PermissionBanMembers) {
		t.Fatal("Admin permission check")
	}
}

type fakeTransport struct {
	requests  []*http.Request
	responses []string
}

func (f *fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, r)
	body := "{}"
	if len(f.responses) > 0 {
		body = f.responses[0]
		f.responses = f.responses[1:]
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}
func TestAutoroleJoinGuards(t *testing.T) {
	a, s, tr := communityFixture(t)
	a.cfg.MemberRoleID = embedRole
	m := &discordgo.Member{GuildID: embedGuild, User: &discordgo.User{ID: embedUser}, Pending: true}
	a.assignRole(s, m)
	if len(tr.calls) != 0 {
		t.Fatal("Screening must finish before assigning a role")
	}
	m.Pending = false
	m.User.Bot = true
	a.assignRole(s, m)
	if len(tr.calls) != 0 {
		t.Fatal("Bots are excluded")
	}
	m.User.Bot = false
	m.Roles = []string{embedRole}
	a.assignRole(s, m)
	if len(tr.calls) != 0 {
		t.Fatal("Role already assigned")
	}
	m.Roles = nil
	a.assignRole(s, m)
	writes := 0
	for _, call := range tr.calls {
		if call.Method == "PUT" && strings.HasSuffix(call.Path, "/guilds/"+embedGuild+"/members/"+embedUser+"/roles/"+embedRole) {
			writes++
		}
	}
	if writes != 1 {
		t.Fatal("Member join must issue one role assignment")
	}
	before := len(tr.calls)
	m.GuildID = "other"
	a.assignRole(s, m)
	if len(tr.calls) != before {
		t.Fatal("Must not assign roles in other servers")
	}
}
func TestCommandDefinitions(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commands() {
		if seen[c.Name] {
			t.Fatal("Duplicate command")
		}
		seen[c.Name] = true
		if p := commandPermissions[c.Name]; p != 0 && (c.DefaultMemberPermissions == nil || *c.DefaultMemberPermissions != p) {
			t.Fatal("Missing default moderator permissions")
		}
		if _, e := json.Marshal(c); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"warn", "timeout", "ban", "youtube-test", "purge"} {
		if !seen[name] {
			t.Fatal("Missing command", name)
		}
	}
}
func TestAvatarValidation(t *testing.T) {
	data := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not really an image"))
	if e := validateAvatar(data); e == nil {
		t.Fatal("File content must match its declared image type")
	}
}
