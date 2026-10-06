package main

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

const embedGuild = "123456789012345678"
const embedChannel = "223456789012345678"
const embedRole = "323456789012345678"
const embedBot = "423456789012345678"
const embedUser = "523456789012345678"
const embedMessage = "623456789012345678"

func validDraft() EmbedDraft {
	d := emptyEmbedDraft()
	d.ID = "draft123"
	d.Name = "Rules"
	d.ChannelID = embedChannel
	d.Title = "Workshop rules"
	d.Description = "Be kind. Read the rules."
	d.Mode = "button"
	d.RoleID = embedRole
	return d
}
func TestEmbedValidationAndPayload(t *testing.T) {
	d := validDraft()
	if e := validateEmbed(&d); e != nil {
		t.Fatal(e)
	}
	p := embedPayload(d)
	if p.Embeds[0].Color != 0xed4245 || len(p.Components) != 1 {
		t.Fatal("Color/button missing")
	}
	row := p.Components[0].(discordgo.ActionsRow)
	button := row.Components[0].(discordgo.Button)
	if button.CustomID != "mubble_rules:"+d.ID || button.Style != discordgo.DangerButton {
		t.Fatal("Wrong button identity/style")
	}
	if len(p.AllowedMentions.Parse) != 0 {
		t.Fatal("Embed messages must not mass-ping")
	}
	d.Description = strings.Repeat("x", 4097)
	if e := validateEmbed(&d); e == nil {
		t.Fatal("Description limit must be enforced")
	}
	d = validDraft()
	d.Fields = []EmbedField{{"Section", "Details", true}}
	if e := validateEmbed(&d); e != nil {
		t.Fatal(e)
	}
	d.URL = "javascript:alert(1)"
	if e := validateEmbed(&d); e == nil {
		t.Fatal("Unsafe links must be rejected")
	}
	d = validDraft()
	d.Mode = "reaction"
	d.Emoji = "not an emoji"
	if e := validateEmbed(&d); e == nil {
		t.Fatal("Text must not be accepted as a reaction")
	}
}
func TestEmojiIdentity(t *testing.T) {
	for _, s := range []string{"<:check:123456789012345678>", "<a:check:123456789012345678>", "check:123456789012345678"} {
		api, key, e := emojiParts(s)
		if e != nil || api != "check:123456789012345678" || key != "id:123456789012345678" {
			t.Fatal(api, key, e)
		}
	}
	_, key, e := emojiParts("☑️")
	if e != nil || key != reactionKey(discordgo.Emoji{Name: "☑"}) {
		t.Fatal("Unicode variation-selector matching failed")
	}
}
func TestEmbedPersistenceAndDraftIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := openStore(path)
	d := validDraft()
	pub := &PublishedEmbed{embedChannel, embedMessage, d}
	p := EmbedPost{ID: d.ID, GuildID: embedGuild, Draft: d, Published: pub}
	if e := s.saveEmbed(p); e != nil {
		t.Fatal(e)
	}
	p.Draft.RoleID = "723456789012345678"
	p.Draft.Description = "Edited, not yet published"
	if e := s.saveEmbed(p); e != nil {
		t.Fatal(e)
	}
	s, e := openStore(path)
	if e != nil {
		t.Fatal(e)
	}
	read, ok := s.embedPost(d.ID)
	if !ok || read.Published.Draft.RoleID != embedRole || read.Draft.RoleID == embedRole {
		t.Fatal("Draft edits must not change active role bindings")
	}
	if len(s.embedPosts("other")) != 0 {
		t.Fatal("Saved embeds must be scoped to the server")
	}
}

type embedTransport struct {
	paths   []string
	methods []string
	bodies  []string
}

func (f *embedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.paths = append(f.paths, r.URL.Path)
	f.methods = append(f.methods, r.Method)
	b, _ := io.ReadAll(r.Body)
	f.bodies = append(f.bodies, string(b))
	body := "{}"
	switch {
	case strings.HasSuffix(r.URL.Path, "/guilds/"+embedGuild+"/channels"):
		body = "[]"
	case strings.HasSuffix(r.URL.Path, "/roles"):
		body = `[{"id":"` + embedGuild + `","position":0,"permissions":"1024"},{"id":"botrole","position":10,"permissions":"268503040"},{"id":"` + embedRole + `","position":1,"permissions":"1024"}]`
	case strings.HasSuffix(r.URL.Path, "/members/"+embedBot):
		body = `{"user":{"id":"` + embedBot + `","bot":true},"roles":["botrole"]}`
	case strings.HasSuffix(r.URL.Path, "/members/"+embedUser):
		body = `{"user":{"id":"` + embedUser + `"},"roles":[],"pending":false}`
	case strings.HasSuffix(r.URL.Path, "/channels/"+embedChannel):
		body = `{"id":"` + embedChannel + `","guild_id":"` + embedGuild + `","type":0}`
	case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/messages"):
		body = `{"id":"` + embedMessage + `","channel_id":"` + embedChannel + `"}`
	case r.Method == "PATCH" && strings.HasSuffix(r.URL.Path, "/messages/"+embedMessage):
		body = `{"id":"` + embedMessage + `","channel_id":"` + embedChannel + `"}`
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}
func embedFixture(t *testing.T, mode string) (*App, *discordgo.Session, *embedTransport) {
	t.Helper()
	tr := &embedTransport{}
	s, _ := discordgo.New("Bot fake")
	s.Client = &http.Client{Transport: tr}
	s.State.User = &discordgo.User{ID: embedBot}
	s.State.GuildAdd(&discordgo.Guild{ID: embedGuild, OwnerID: "owner", Roles: []*discordgo.Role{{ID: embedGuild, Permissions: discordgo.PermissionViewChannel}, {ID: "botrole", Position: 10, Permissions: discordgo.PermissionManageRoles | discordgo.PermissionSendMessages | discordgo.PermissionEmbedLinks | discordgo.PermissionAddReactions | discordgo.PermissionReadMessageHistory}}, Members: []*discordgo.Member{{GuildID: embedGuild, User: s.State.User, Roles: []string{"botrole"}}}, Channels: []*discordgo.Channel{{ID: embedChannel, GuildID: embedGuild, Type: discordgo.ChannelTypeGuildText}}})
	store, e := openStore(filepath.Join(t.TempDir(), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	c := defaultConfig()
	c.GuildID = embedGuild
	d := validDraft()
	d.Mode = mode
	post := EmbedPost{ID: d.ID, GuildID: embedGuild, Draft: d, Published: &PublishedEmbed{embedChannel, embedMessage, d}}
	if e = store.saveEmbed(post); e != nil {
		t.Fatal(e)
	}
	return &App{cfg: c, store: store, session: s, ready: true}, s, tr
}
func roleWrites(tr *embedTransport) int {
	n := 0
	for i, p := range tr.paths {
		if tr.methods[i] == "PUT" && strings.HasSuffix(p, "/roles/"+embedRole) {
			n++
		}
	}
	return n
}
func TestRulesReactionScopesAndGrant(t *testing.T) {
	a, s, tr := embedFixture(t, "reaction")
	member := &discordgo.Member{GuildID: embedGuild, User: &discordgo.User{ID: embedUser}}
	e := &discordgo.MessageReactionAdd{MessageReaction: &discordgo.MessageReaction{GuildID: embedGuild, ChannelID: embedChannel, MessageID: embedMessage, UserID: embedUser, Emoji: discordgo.Emoji{Name: "❌"}}, Member: member}
	a.onRulesReaction(s, e)
	if len(tr.paths) != 0 {
		t.Fatal("Wrong emoji must not grant a role")
	}
	e.Emoji.Name = "✅"
	e.MessageID = "wrong"
	a.onRulesReaction(s, e)
	if len(tr.paths) != 0 {
		t.Fatal("Other messages must not grant a role")
	}
	e.MessageID = embedMessage
	member.Pending = true
	a.onRulesReaction(s, e)
	if roleWrites(tr) != 0 {
		t.Fatal("Screening must finish before granting a role")
	}
	member.Pending = false
	a.onRulesReaction(s, e)
	if roleWrites(tr) != 1 {
		t.Fatal("Valid reaction should grant the role", tr.paths)
	}
	member.Roles = []string{embedRole}
	a.onRulesReaction(s, e)
	if roleWrites(tr) != 1 {
		t.Fatal("Already-held role must not be assigned again")
	}
}
func TestRulesButtonScopesAndGrant(t *testing.T) {
	a, s, tr := embedFixture(t, "button")
	i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{ID: "interaction", AppID: embedBot, Token: "test-interaction-token", Type: discordgo.InteractionMessageComponent, GuildID: embedGuild, ChannelID: embedChannel, Message: &discordgo.Message{ID: "wrong"}, Member: &discordgo.Member{User: &discordgo.User{ID: embedUser}}, Data: discordgo.MessageComponentInteractionData{CustomID: "mubble_rules:draft123", ComponentType: discordgo.ButtonComponent}}}
	a.onRulesButton(s, i)
	if roleWrites(tr) != 0 {
		t.Fatal("A copied/forged button must not grant a role")
	}
	i.Message.ID = embedMessage
	a.onRulesButton(s, i)
	if roleWrites(tr) != 1 {
		t.Fatal("Valid saved button should grant a role", tr.paths)
	}
}
func TestPublishUpdatesExistingMessage(t *testing.T) {
	a, _, tr := embedFixture(t, "button")
	p, _ := a.store.embedPost("draft123")
	p.Draft.Description = "Updated rules"
	if e := a.store.saveEmbed(p); e != nil {
		t.Fatal(e)
	}
	post, _, e := a.publishEmbed(p.ID)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for i, path := range tr.paths {
		if tr.methods[i] == "PATCH" && strings.HasSuffix(path, "/messages/"+embedMessage) {
			found = true
			var b map[string]any
			if e = json.Unmarshal([]byte(tr.bodies[i]), &b); e != nil {
				t.Fatal(e)
			}
			if !strings.Contains(tr.bodies[i], "Updated rules") {
				t.Fatal("Updated embed text not sent")
			}
		}
	}
	if !found || post.Published.MessageID != embedMessage {
		t.Fatal("Updates must retain the original posted message")
	}
}
