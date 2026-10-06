package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}
type EmbedDraft struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	ChannelID   string       `json:"channel_id"`
	Content     string       `json:"content"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Color       string       `json:"color"`
	URL         string       `json:"url"`
	Author      string       `json:"author"`
	AuthorIcon  string       `json:"author_icon"`
	Image       string       `json:"image"`
	Thumbnail   string       `json:"thumbnail"`
	Footer      string       `json:"footer"`
	Fields      []EmbedField `json:"fields"`
	Mode        string       `json:"mode"`
	RoleID      string       `json:"role_id"`
	ButtonLabel string       `json:"button_label"`
	ButtonStyle string       `json:"button_style"`
	Emoji       string       `json:"emoji"`
}
type PublishedEmbed struct {
	ChannelID string     `json:"channel_id"`
	MessageID string     `json:"message_id"`
	Draft     EmbedDraft `json:"draft"`
}
type EmbedPost struct {
	ID        string          `json:"id"`
	GuildID   string          `json:"guild_id"`
	Draft     EmbedDraft      `json:"draft"`
	Published *PublishedEmbed `json:"published,omitempty"`
	Updated   time.Time       `json:"updated"`
}

func emptyEmbedDraft() EmbedDraft {
	return EmbedDraft{Name: "New message", Color: "#ed4245", Author: "Mubble’s Workshop", Mode: "none", ButtonLabel: "I've read the rules", ButtonStyle: "red", Emoji: "✅", Fields: []EmbedField{}}
}
func clonePost(p EmbedPost) EmbedPost {
	p.Draft.Fields = append([]EmbedField{}, p.Draft.Fields...)
	if p.Published != nil {
		pub := *p.Published
		pub.Draft.Fields = append([]EmbedField{}, pub.Draft.Fields...)
		p.Published = &pub
	}
	return p
}
func (s *Store) embedPosts(g string) []EmbedPost {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []EmbedPost{}
	for _, p := range s.data.Embeds {
		if p.GuildID == g {
			out = append(out, clonePost(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}
func (s *Store) embedPost(id string) (EmbedPost, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Embeds[id]
	return clonePost(p), ok
}
func (s *Store) saveEmbed(p EmbedPost) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Embeds == nil {
		s.data.Embeds = map[string]EmbedPost{}
	}
	old, exists := s.data.Embeds[p.ID]
	if !exists && len(s.data.Embeds) >= 100 {
		return errors.New("You can save up to 100 embed messages")
	}
	s.data.Embeds[p.ID] = clonePost(p)
	if e := atomicJSON(s.path, s.data); e != nil {
		if exists {
			s.data.Embeds[p.ID] = old
		} else {
			delete(s.data.Embeds, p.ID)
		}
		return e
	}
	return nil
}
func newEmbedID() (string, error) {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func validEmbedURL(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > 2048 {
		return false
	}
	u, e := url.ParseRequestURI(s)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

var customEmojiRE = regexp.MustCompile(`^<(?:a)?:([A-Za-z0-9_]{2,32}):([0-9]{16,22})>$`)
var rawEmojiRE = regexp.MustCompile(`^([A-Za-z0-9_]{2,32}):([0-9]{16,22})$`)

func emojiParts(s string) (api, key string, err error) {
	s = strings.TrimSpace(s)
	for _, re := range []*regexp.Regexp{customEmojiRE, rawEmojiRE} {
		if p := re.FindStringSubmatch(s); p != nil {
			return p[1] + ":" + p[2], "id:" + p[2], nil
		}
	}
	if s == "" || utf8.RuneCountInString(s) > 32 || strings.ContainsAny(s, " \t\r\n") {
		return "", "", errors.New("Choose one emoji, for example ✅, or paste a custom Discord emoji")
	}
	symbol := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			return "", "", errors.New("Use an emoji rather than text")
		}
		if r > 127 {
			symbol = true
		}
	}
	if !symbol {
		return "", "", errors.New("Use an emoji rather than text")
	}
	return s, "unicode:" + strings.ReplaceAll(s, "\ufe0f", ""), nil
}
func validateEmbed(d *EmbedDraft) error {
	d.Name = strings.TrimSpace(d.Name)
	d.ChannelID = strings.TrimSpace(d.ChannelID)
	d.Color = strings.TrimSpace(d.Color)
	d.RoleID = strings.TrimSpace(d.RoleID)
	d.Title = strings.TrimSpace(d.Title)
	d.Author = strings.TrimSpace(d.Author)
	d.Footer = strings.TrimSpace(d.Footer)
	if d.Name == "" || utf8.RuneCountInString(d.Name) > 80 {
		return errors.New("Give the message a name of 1–80 characters")
	}
	if !validID(d.ChannelID) {
		return errors.New("Choose a Discord channel for this message")
	}
	checks := []struct {
		value string
		max   int
		label string
	}{{d.Content, 2000, "Message text"}, {d.Title, 256, "Embed title"}, {d.Description, 4096, "Embed description"}, {d.Author, 256, "Author name"}, {d.Footer, 2048, "Footer"}}
	total := 0
	for _, v := range checks {
		n := utf8.RuneCountInString(v.value)
		if n > v.max {
			return fmt.Errorf("%s exceeds %d characters", v.label, v.max)
		}
		if v.label != "Message text" {
			total += n
		}
	}
	if len(d.Fields) > 25 {
		return errors.New("An embed can contain up to 25 fields")
	}
	for i := range d.Fields {
		f := &d.Fields[i]
		f.Name = strings.TrimSpace(f.Name)
		f.Value = strings.TrimSpace(f.Value)
		if f.Name == "" || f.Value == "" || utf8.RuneCountInString(f.Name) > 256 || utf8.RuneCountInString(f.Value) > 1024 {
			return errors.New("Each field needs a name (up to 256 characters) and text (up to 1024)")
		}
		total += utf8.RuneCountInString(f.Name) + utf8.RuneCountInString(f.Value)
	}
	if total > 6000 {
		return errors.New("The embed exceeds Discord's combined 6000-character limit")
	}
	if d.Title == "" && strings.TrimSpace(d.Description) == "" && len(d.Fields) == 0 && d.Image == "" && d.Thumbnail == "" {
		return errors.New("Add an embed title, description, field or image")
	}
	color := strings.TrimPrefix(d.Color, "#")
	if len(color) != 6 {
		return errors.New("Choose a six-digit embed color")
	}
	if _, e := strconv.ParseUint(color, 16, 24); e != nil {
		return errors.New("Invalid embed color")
	}
	d.Color = "#" + strings.ToLower(color)
	for _, s := range []string{d.URL, d.AuthorIcon, d.Image, d.Thumbnail} {
		if !validEmbedURL(s) {
			return errors.New("Embed links and images must use a valid https:// URL")
		}
	}
	if d.Mode != "none" && d.Mode != "button" && d.Mode != "reaction" {
		return errors.New("Choose no role action, a button, or an emoji reaction")
	}
	if d.Mode != "none" && !validID(d.RoleID) {
		return errors.New("Choose the read-the-rules role")
	}
	if d.Mode == "button" {
		d.ButtonLabel = strings.TrimSpace(d.ButtonLabel)
		if d.ButtonLabel == "" || utf8.RuneCountInString(d.ButtonLabel) > 80 {
			return errors.New("Button text must have 1–80 characters")
		}
		if d.ButtonStyle != "red" && d.ButtonStyle != "green" && d.ButtonStyle != "blue" && d.ButtonStyle != "grey" {
			return errors.New("Choose a valid button color")
		}
	}
	if d.Mode == "reaction" {
		if _, _, e := emojiParts(d.Emoji); e != nil {
			return e
		}
	}
	return nil
}
func embedPayload(d EmbedDraft) *discordgo.MessageSend {
	color, _ := strconv.ParseInt(strings.TrimPrefix(d.Color, "#"), 16, 32)
	e := &discordgo.MessageEmbed{Title: d.Title, Description: d.Description, Color: int(color), URL: d.URL}
	if d.Author != "" {
		e.Author = &discordgo.MessageEmbedAuthor{Name: d.Author, IconURL: d.AuthorIcon}
	}
	if d.Footer != "" {
		e.Footer = &discordgo.MessageEmbedFooter{Text: d.Footer}
	}
	if d.Image != "" {
		e.Image = &discordgo.MessageEmbedImage{URL: d.Image}
	}
	if d.Thumbnail != "" {
		e.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: d.Thumbnail}
	}
	for _, f := range d.Fields {
		e.Fields = append(e.Fields, &discordgo.MessageEmbedField{Name: f.Name, Value: f.Value, Inline: f.Inline})
	}
	p := &discordgo.MessageSend{Content: d.Content, Embeds: []*discordgo.MessageEmbed{e}, AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}, Components: []discordgo.MessageComponent{}}
	if d.Mode == "button" {
		style := discordgo.DangerButton
		switch d.ButtonStyle {
		case "green":
			style = discordgo.SuccessButton
		case "blue":
			style = discordgo.PrimaryButton
		case "grey":
			style = discordgo.SecondaryButton
		}
		p.Components = []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.Button{Label: d.ButtonLabel, Style: style, CustomID: "mubble_rules:" + d.ID}}}}
	}
	return p
}
func (a *App) saveEmbedDraft(d EmbedDraft) (EmbedPost, error) {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.embedMu.Lock()
	defer a.embedMu.Unlock()
	if e := validateEmbed(&d); e != nil {
		return EmbedPost{}, e
	}
	c := a.config()
	if !validID(c.GuildID) {
		return EmbedPost{}, errors.New("Save your Discord connection settings first")
	}
	p := EmbedPost{GuildID: c.GuildID}
	if d.ID != "" {
		var ok bool
		p, ok = a.store.embedPost(d.ID)
		if !ok || p.GuildID != c.GuildID {
			return EmbedPost{}, errors.New("Saved message was not found in this server")
		}
		if p.Published != nil && p.Published.ChannelID != d.ChannelID {
			return EmbedPost{}, errors.New("A published message must stay in its original channel; use New / Copy to post elsewhere")
		}
	} else {
		var e error
		d.ID, e = newEmbedID()
		if e != nil {
			return p, e
		}
	}
	p.ID = d.ID
	p.Draft = d
	p.Updated = time.Now().UTC()
	return p, a.store.saveEmbed(p)
}
func (a *App) validateRulesRole(s *discordgo.Session, g, roleID string) error {
	roles, e := s.GuildRoles(g)
	if e != nil {
		return e
	}
	bot, e := s.GuildMember(g, s.State.User.ID)
	if e != nil {
		return e
	}
	if !hasPermission(guildPermissions(bot, g, roles), discordgo.PermissionManageRoles) {
		return errors.New("The bot needs Manage Roles to give the rules role")
	}
	for _, r := range roles {
		if r.ID == roleID {
			if r.ID == g || r.Managed || r.Position >= rolePosition(bot, roles) {
				return errors.New("Choose a normal role below the bot's highest role")
			}
			if r.Permissions&unsafeSelfRolePermissions != 0 {
				return errors.New("Roles granted to members publicly must not contain staff or management permissions")
			}
			channels, err := s.GuildChannels(g)
			if err != nil {
				return err
			}
			for _, ch := range channels {
				for _, o := range ch.PermissionOverwrites {
					if o.Type == discordgo.PermissionOverwriteTypeRole && o.ID == roleID && o.Allow&unsafeSelfRolePermissions != 0 {
						return errors.New("Public roles must not grant channel-specific staff permissions")
					}
				}
			}
			return nil
		}
	}
	return errors.New("The rules role does not exist in this server")
}
func editPayload(s *discordgo.Session, pub PublishedEmbed, d EmbedDraft) (*discordgo.Message, error) {
	p := embedPayload(d)
	return s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: pub.MessageID, Channel: pub.ChannelID, Content: &p.Content, Embeds: &p.Embeds, Components: &p.Components, AllowedMentions: p.AllowedMentions})
}
func (a *App) publishEmbed(id string) (EmbedPost, string, error) {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.embedMu.Lock()
	defer a.embedMu.Unlock()
	s, e := a.current()
	if e != nil {
		return EmbedPost{}, "", e
	}
	c := a.config()
	p, ok := a.store.embedPost(id)
	if !ok || p.GuildID != c.GuildID {
		return p, "", errors.New("Saved message not found")
	}
	d := p.Draft
	if e = validateEmbed(&d); e != nil {
		return p, "", e
	}
	ch, e := s.Channel(d.ChannelID)
	if e != nil {
		return p, "", e
	}
	if ch.GuildID != c.GuildID {
		return p, "", errors.New("Embed channel must belong to the configured server")
	}
	perms, e := s.UserChannelPermissions(s.State.User.ID, d.ChannelID)
	if e != nil {
		return p, "", e
	}
	required := int64(discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionEmbedLinks)
	if d.Mode == "reaction" {
		required |= discordgo.PermissionAddReactions | discordgo.PermissionReadMessageHistory
	}
	if !hasPermission(perms, required) {
		return p, "", errors.New("The bot needs View Channel, Send Messages and Embed Links; emoji reactions also need Add Reactions and Read Message History")
	}
	if d.Mode != "none" {
		if e = a.validateRulesRole(s, c.GuildID, d.RoleID); e != nil {
			return p, "", e
		}
	}
	var message *discordgo.Message
	old := clonePost(p)
	fresh := p.Published == nil
	if fresh {
		message, e = s.ChannelMessageSendComplex(d.ChannelID, embedPayload(d))
	} else {
		message, e = editPayload(s, *p.Published, d)
	}
	if e != nil {
		return p, "", e
	}
	p.Published = &PublishedEmbed{d.ChannelID, message.ID, d}
	p.Updated = time.Now().UTC()
	if e = a.store.saveEmbed(p); e != nil {
		if fresh {
			s.ChannelMessageDelete(d.ChannelID, message.ID)
		} else {
			editPayload(s, *old.Published, old.Published.Draft)
		}
		return old, "", fmt.Errorf("Could not save the role binding; attempted to restore the previous message: %w", e)
	}
	warning := ""
	if old.Published != nil && old.Published.Draft.Mode == "reaction" {
		before, _, _ := emojiParts(old.Published.Draft.Emoji)
		after, _, _ := emojiParts(d.Emoji)
		if d.Mode != "reaction" || before != after {
			s.MessageReactionRemove(old.Published.ChannelID, old.Published.MessageID, before, "@me")
		}
	}
	if d.Mode == "reaction" {
		api, _, _ := emojiParts(d.Emoji)
		if e = s.MessageReactionAdd(d.ChannelID, message.ID, api); e != nil {
			warning = "Message posted, but the emoji could not be added: " + a.errorText(e)
		}
	}
	a.log("Published embed: " + d.Name)
	return p, warning, nil
}
func (a *App) embedRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/embeds", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		sendJSON(w, map[string]any{"posts": a.store.embedPosts(a.config().GuildID), "blank": emptyEmbedDraft()})
	})
	mux.HandleFunc("/api/embed-save", func(w http.ResponseWriter, r *http.Request) {
		var d EmbedDraft
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&d); e != nil {
			apiError(w, e)
			return
		}
		p, e := a.saveEmbedDraft(d)
		if e != nil {
			apiError(w, e)
			return
		}
		sendJSON(w, map[string]any{"post": p})
	})
	mux.HandleFunc("/api/embed-publish", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); e != nil {
			apiError(w, e)
			return
		}
		p, warning, e := a.publishEmbed(req.ID)
		if e != nil {
			apiError(w, errors.New(a.errorText(e)))
			return
		}
		sendJSON(w, map[string]any{"post": p, "warning": warning, "url": fmt.Sprintf("https://discord.com/channels/%s/%s/%s", p.GuildID, p.Published.ChannelID, p.Published.MessageID)})
	})
}
func (a *App) grantRulesRole(s *discordgo.Session, p EmbedPost, user string, member *discordgo.Member) error {
	if p.Published == nil {
		return errors.New("This rules message is not active")
	}
	if member == nil {
		var e error
		member, e = s.GuildMember(p.GuildID, user)
		if e != nil {
			return e
		}
	}
	if member.User == nil || member.User.Bot {
		return errors.New("Only server members can accept the rules")
	}
	if member.Pending {
		return errors.New("Finish Discord's membership screening first, then accept the rules here")
	}
	for _, id := range member.Roles {
		if id == p.Published.Draft.RoleID {
			return nil
		}
	}
	if e := a.validateRulesRole(s, p.GuildID, p.Published.Draft.RoleID); e != nil {
		return e
	}
	if e := s.GuildMemberRoleAdd(p.GuildID, user, p.Published.Draft.RoleID); e != nil {
		return e
	}
	a.audit(s, "Assigned rules role to "+user)
	return nil
}
func (a *App) onRulesButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if !strings.HasPrefix(data.CustomID, "mubble_rules:") || i.GuildID != a.config().GuildID || i.Member == nil || i.Member.User == nil {
		return
	}
	if e := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral}}); e != nil {
		a.log("Could not acknowledge rules button: " + a.errorText(e))
		return
	}
	a.embedMu.Lock()
	defer a.embedMu.Unlock()
	p, ok := a.store.embedPost(strings.TrimPrefix(data.CustomID, "mubble_rules:"))
	var err error
	if !ok || p.GuildID != i.GuildID || p.Published == nil || p.Published.Draft.Mode != "button" || i.Message == nil || p.Published.MessageID != i.Message.ID || p.Published.ChannelID != i.ChannelID {
		err = errors.New("This rules button is no longer active")
	} else {
		err = a.grantRulesRole(s, p, i.Member.User.ID, i.Member)
	}
	text := "You're all set! Your rules role has been added."
	if err != nil {
		text = "Could not add your rules role: " + a.errorText(err)
		a.log(text)
	}
	if _, e := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &text, AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}}); e != nil {
		a.log("Could not send rules confirmation: " + a.errorText(e))
	}
}
func reactionKey(e discordgo.Emoji) string {
	if e.ID != "" {
		return "id:" + e.ID
	}
	return "unicode:" + strings.ReplaceAll(e.Name, "\ufe0f", "")
}
func (a *App) onRulesReaction(s *discordgo.Session, e *discordgo.MessageReactionAdd) {
	if e.MessageReaction == nil || e.GuildID != a.config().GuildID || e.UserID == s.State.User.ID {
		return
	}
	a.embedMu.Lock()
	defer a.embedMu.Unlock()
	for _, p := range a.store.embedPosts(e.GuildID) {
		if p.Published == nil || p.Published.Draft.Mode != "reaction" || p.Published.ChannelID != e.ChannelID || p.Published.MessageID != e.MessageID {
			continue
		}
		_, key, err := emojiParts(p.Published.Draft.Emoji)
		if err != nil || key != reactionKey(e.Emoji) {
			continue
		}
		if err = a.grantRulesRole(s, p, e.UserID, e.Member); err != nil {
			a.log("Reaction role failed for " + e.UserID + ": " + a.errorText(err))
		}
		return
	}
}
func (a *App) reconcileRulesReactions(ctx context.Context, s *discordgo.Session) {
	for _, p := range a.store.embedPosts(a.config().GuildID) {
		if ctx.Err() != nil {
			return
		}
		if p.Published == nil || p.Published.Draft.Mode != "reaction" {
			continue
		}
		api, _, e := emojiParts(p.Published.Draft.Emoji)
		if e != nil {
			continue
		}
		after := ""
		for page := 0; page < 100; page++ {
			users, e := s.MessageReactions(p.Published.ChannelID, p.Published.MessageID, api, 100, "", after, discordgo.WithContext(ctx))
			if e != nil {
				if ctx.Err() == nil {
					a.log("Rules reaction catch-up: " + a.errorText(e))
				}
				break
			}
			for _, u := range users {
				if ctx.Err() != nil {
					return
				}
				if u.Bot {
					continue
				}
				a.embedMu.Lock()
				current, ok := a.store.embedPost(p.ID)
				if ok && current.Published != nil && current.Published.MessageID == p.Published.MessageID && current.Published.Draft.Mode == "reaction" && current.Published.Draft.Emoji == p.Published.Draft.Emoji {
					if e = a.grantRulesRole(s, current, u.ID, nil); e != nil && ctx.Err() == nil {
						a.log("Rules reaction catch-up for " + u.ID + ": " + a.errorText(e))
					}
				}
				a.embedMu.Unlock()
			}
			if len(users) < 100 {
				break
			}
			after = users[len(users)-1].ID
		}
	}
}
