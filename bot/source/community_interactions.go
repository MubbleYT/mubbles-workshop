package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

func stableNonce(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:12])
}

type ModalSession struct {
	User    string
	Guild   string
	Kind    string
	Expires time.Time
}

func (a *App) openCommunityModal(s *discordgo.Session, i *discordgo.InteractionCreate, kind string) {
	id, err := newEmbedID()
	if err != nil {
		a.replyCommunity(s, i, "Could not open form.")
		return
	}
	a.modalMu.Lock()
	if a.modals == nil {
		a.modals = map[string]ModalSession{}
	}
	for k, v := range a.modals {
		if v.Expires.Before(time.Now()) {
			delete(a.modals, k)
		}
	}
	if len(a.modals) >= 500 {
		a.modalMu.Unlock()
		a.replyCommunity(s, i, "Too many forms open. Please try again shortly.")
		return
	}
	a.modals[id] = ModalSession{i.Member.User.ID, i.GuildID, kind, time.Now().Add(10 * time.Minute)}
	a.modalMu.Unlock()
	input := func(key, label string, style discordgo.TextInputStyle, max int, required bool) discordgo.MessageComponent {
		return discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: key, Label: label, Style: style, MaxLength: max, Required: required}}}
	}
	title := "Share a suggestion"
	rows := []discordgo.MessageComponent{input("title", "Title", discordgo.TextInputShort, 100, true), input("body", "Tell us more", discordgo.TextInputParagraph, 1800, true)}
	if kind == "ticket" || kind == "report" {
		title = "Contact moderators"
		if kind == "report" {
			title = "Report an issue privately"
		}
		rows[1] = input("body", "Details / message link (staff can see this)", discordgo.TextInputParagraph, 1800, true)
	}
	if kind == "bug" {
		title = "Report a project bug"
		rows = []discordgo.MessageComponent{input("title", "Which issue are you reporting?", discordgo.TextInputShort, 100, true), input("version", "Project version + Minecraft version", discordgo.TextInputShort, 180, true), input("body", "Steps to reproduce / what went wrong", discordgo.TextInputParagraph, 1800, true), input("expected", "What should happen?", discordgo.TextInputParagraph, 800, true), input("image", "Screenshot / clip link (HTTPS, optional)", discordgo.TextInputShort, 300, false)}
	}
	if err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: "mw:form:" + id, Title: title, Components: rows}}); err != nil {
		a.log("Open form: " + a.errorText(err))
	}
}
func modalValues(d discordgo.ModalSubmitInteractionData) map[string]string {
	out := map[string]string{}
	for _, row := range d.Components {
		if r, ok := row.(*discordgo.ActionsRow); ok {
			for _, c := range r.Components {
				if t, ok := c.(*discordgo.TextInput); ok {
					out[t.CustomID] = strings.TrimSpace(t.Value)
				}
			}
		}
	}
	return out
}
func (a *App) replyCommunity(s *discordgo.Session, i *discordgo.InteractionCreate, text string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: clip(text, 1900), Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions()}}); err != nil {
		a.log("Community response: " + a.errorText(err))
	}
}
func (a *App) deferredCommunity(s *discordgo.Session, i *discordgo.InteractionCreate, fn func() (string, error)) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral}}); err != nil {
		return
	}
	text, err := fn()
	if err != nil {
		text = "Could not complete that: " + a.errorText(err)
		a.log("Community action: " + a.errorText(err))
	}
	text = clip(text, 1900)
	if _, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &text, AllowedMentions: noMentions()}); err != nil {
		a.log("Community response: " + a.errorText(err))
	}
}
func (a *App) handleCommunityInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	var custom string
	var form bool
	if i.Type == discordgo.InteractionMessageComponent {
		custom = i.MessageComponentData().CustomID
	} else if i.Type == discordgo.InteractionModalSubmit {
		custom = i.ModalSubmitData().CustomID
		form = true
	} else if i.Type == discordgo.InteractionApplicationCommand {
		n := i.ApplicationCommandData().Name
		if n == "ticket" || n == "report" || n == "suggest" || n == "bugreport" {
			if i.GuildID != a.config().GuildID || i.Member == nil || i.Member.User == nil || i.Member.User.Bot || i.Member.Pending {
				a.replyCommunity(s, i, "Complete the server's membership screening first.")
				return true
			}
			kind := map[string]string{"ticket": "ticket", "report": "report", "suggest": "suggest", "bugreport": "bug"}[n]
			a.openCommunityModal(s, i, kind)
			return true
		}
		return false
	} else {
		return false
	}
	if !strings.HasPrefix(custom, "mw:") {
		return false
	}
	if i.GuildID != a.config().GuildID || i.Member == nil || i.Member.User == nil || i.Member.User.Bot || i.Member.Pending {
		a.replyCommunity(s, i, "This control is only for members who completed screening in the configured server.")
		return true
	}
	parts := strings.Split(custom, ":")
	if len(parts) != 3 {
		a.replyCommunity(s, i, "Invalid control.")
		return true
	}
	kind, id := parts[1], parts[2]
	if form {
		if kind != "form" {
			a.replyCommunity(s, i, "Invalid form.")
			return true
		}
		a.modalMu.Lock()
		m, ok := a.modals[id]
		if ok && m.User == i.Member.User.ID && m.Guild == i.GuildID {
			delete(a.modals, id)
		}
		a.modalMu.Unlock()
		if !ok || m.User != i.Member.User.ID || m.Guild != i.GuildID || time.Now().After(m.Expires) {
			a.replyCommunity(s, i, "That form expired. Open a fresh one and try again.")
			return true
		}
		values := modalValues(i.ModalSubmitData())
		a.deferredCommunity(s, i, func() (string, error) {
			if m.Kind == "ticket" || m.Kind == "report" {
				return a.createTicket(s, i.Member, values["title"], values["body"])
			}
			return a.createSubmission(s, i.Member, m.Kind, values)
		})
		return true
	}
	v := a.store.community(i.GuildID)
	if kind == "roles" || kind == "ticket" || kind == "suggest" || kind == "bug" {
		panel, ok := v.Panels[id]
		validKind := panel.Kind == kind || (panel.Kind == "submissions" && (kind == "suggest" || kind == "bug"))
		if !ok || !validKind || i.Message == nil || i.Message.ID != panel.MessageID || i.ChannelID != panel.ChannelID {
			a.replyCommunity(s, i, "That panel is no longer active. Use the latest panel.")
			return true
		}
		if kind == "roles" {
			a.deferredCommunity(s, i, func() (string, error) {
				return a.applySelectedRoles(s, i.Member, panel, i.MessageComponentData().Values)
			})
			return true
		}
		a.openCommunityModal(s, i, kind)
		return true
	}
	a.deferredCommunity(s, i, func() (string, error) {
		switch kind {
		case "close", "reopen":
			t, ok := v.Tickets[id]
			if !ok || i.Message == nil || i.Message.ID != t.MessageID || i.ChannelID != t.ChannelID {
				return "", errors.New("Invalid ticket control")
			}
			return a.setTicketClosed(s, i.Member, t, kind == "close")
		case "up", "down", "clearvote", "status":
			return a.updateSubmission(s, i, kind, id)
		default:
			return "", errors.New("Unknown community control")
		}
	})
	return true
}
func (a *App) applySelectedRoles(s *discordgo.Session, m *discordgo.Member, p CommunityPanel, selected []string) (string, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	c := a.config()
	allowed := map[string]bool{}
	currentOptions := map[string]bool{}
	for _, r := range notificationRoles(c) {
		currentOptions[r.RoleID] = true
	}
	for _, r := range p.Roles {
		if !currentOptions[r.RoleID] {
			return "", errors.New("Role menu settings changed. Ask a moderator to publish an updated menu")
		}
		allowed[r.RoleID] = true
		if err := a.validatePublicRole(s, c.GuildID, r.RoleID); err != nil {
			return "", err
		}
	}
	chosen := map[string]bool{}
	for _, id := range selected {
		if !allowed[id] {
			return "", errors.New("Selected role is not in this menu")
		}
		chosen[id] = true
	}
	fresh, e := s.GuildMember(c.GuildID, m.User.ID)
	if e != nil {
		return "", e
	}
	if fresh.Pending || fresh.User.Bot {
		return "", errors.New("Complete membership screening first")
	}
	held := map[string]bool{}
	for _, id := range fresh.Roles {
		held[id] = true
	}
	type change struct {
		id    string
		added bool
	}
	changes := []change{}
	rollback := func() {
		for j := len(changes) - 1; j >= 0; j-- {
			x := changes[j]
			var e error
			if x.added {
				e = s.GuildMemberRoleRemove(c.GuildID, m.User.ID, x.id)
			} else {
				e = s.GuildMemberRoleAdd(c.GuildID, m.User.ID, x.id)
			}
			if e != nil {
				a.log("Role-menu rollback: " + a.errorText(e))
			}
		}
	}
	for _, r := range p.Roles {
		if chosen[r.RoleID] == held[r.RoleID] {
			continue
		}
		if chosen[r.RoleID] {
			e = s.GuildMemberRoleAdd(c.GuildID, m.User.ID, r.RoleID)
		} else {
			e = s.GuildMemberRoleRemove(c.GuildID, m.User.ID, r.RoleID)
		}
		if e != nil {
			rollback()
			return "", e
		}
		changes = append(changes, change{r.RoleID, chosen[r.RoleID]})
	}
	return "Your interests and notification preferences have been updated. Selecting nothing removes all roles offered by this menu.", nil
}
func (a *App) publishCommunityPanel(s *discordgo.Session, kind string) (CommunityPanel, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	c := a.config()
	cc := c.Community
	ch := ""
	title := ""
	desc := ""
	components := []discordgo.MessageComponent{}
	roles := []SelfRole{}
	switch kind {
	case "roles":
		ch = cc.RolesChannel
		title = "Make yourself at home"
		desc = "Choose your interests and the updates you want to hear about. Select all the roles you want to keep, or clear the selection to opt out."
		roles = notificationRoles(c)
		if len(roles) == 0 {
			return CommunityPanel{}, errors.New("Add an interest or notification role first")
		}
		for _, r := range roles {
			if e := a.validatePublicRole(s, c.GuildID, r.RoleID); e != nil {
				return CommunityPanel{}, e
			}
		}
	case "ticket":
		ch = cc.TicketPanelChannel
		title = "Need a hand?"
		desc = "Contact the moderators privately. Reports, server questions and collaboration requests are welcome. Only you, the selected staff and server administrators can see your ticket."
		if len(cc.TicketStaffRoles) == 0 {
			return CommunityPanel{}, errors.New("Choose ticket staff roles first")
		}
		if e := a.checkStaffRoles(s); e != nil {
			return CommunityPanel{}, e
		}
	case "submissions":
		ch = cc.SubmissionPanelChannel
		title = "Workshop ideas & bug reports"
		desc = "Have an idea for a video or project? Found a bug in a mod or shader? Send it here. Submissions will be public in the configured channels."
		if cc.SuggestChannel == "" && cc.BugChannel == "" {
			return CommunityPanel{}, errors.New("Choose a suggestion or bug-report channel first")
		}
	default:
		return CommunityPanel{}, errors.New("Unknown panel type")
	}
	if e := a.postingChannel(s, ch); e != nil {
		return CommunityPanel{}, e
	}
	state := a.store.community(c.GuildID)
	var panel CommunityPanel
	for _, p := range state.Panels {
		if p.Kind == kind && p.ChannelID == ch {
			panel = p
			break
		}
	}
	if panel.ID == "" {
		id, e := newEmbedID()
		if e != nil {
			return panel, e
		}
		panel = CommunityPanel{ID: id, Kind: kind, ChannelID: ch}
	}
	panel.Roles = roles
	if kind == "roles" {
		opts := []discordgo.SelectMenuOption{}
		for _, r := range roles {
			opts = append(opts, discordgo.SelectMenuOption{Label: r.Label, Value: r.RoleID, Description: r.Description})
		}
		components = append(components, discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.SelectMenu{CustomID: "mw:roles:" + panel.ID, Placeholder: "Choose interests & notifications", MinValues: ptr(0), MaxValues: len(opts), Options: opts}}})
	} else if kind == "ticket" {
		components = append(components, discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.Button{CustomID: "mw:ticket:" + panel.ID, Label: "Contact moderators", Style: discordgo.PrimaryButton}}})
	} else {
		buttons := []discordgo.MessageComponent{}
		if cc.SuggestChannel != "" {
			buttons = append(buttons, discordgo.Button{CustomID: "mw:suggest:" + panel.ID, Label: "Share an idea", Style: discordgo.PrimaryButton})
		}
		if cc.BugChannel != "" {
			buttons = append(buttons, discordgo.Button{CustomID: "mw:bug:" + panel.ID, Label: "Report a bug", Style: discordgo.SecondaryButton})
		}
		components = append(components, discordgo.ActionsRow{Components: buttons})
	}
	payload := &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{{Title: title, Description: desc, Color: 0xb18aff}}, Components: components, AllowedMentions: noMentions()}
	fresh := panel.MessageID == ""
	var message *discordgo.Message
	var err error
	if fresh {
		message, err = a.sendStable(s, ch, "panel:"+panel.ID, payload)
	} else {
		message, err = s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: panel.MessageID, Channel: ch, Embeds: &payload.Embeds, Components: &components, AllowedMentions: noMentions()})
	}
	if err != nil {
		return panel, err
	}
	panel.MessageID = message.ID
	if err = a.store.updateCommunity(c.GuildID, func(v *CommunityState) error {
		if len(v.Panels) >= 100 && v.Panels[panel.ID].ID == "" {
			return errors.New("Saved panel limit reached (100)")
		}
		v.Panels[panel.ID] = panel
		return nil
	}); err != nil {
		if fresh {
			s.ChannelMessageDelete(ch, message.ID)
		}
		return panel, err
	}
	return panel, nil
}
func (a *App) checkStaffRoles(s *discordgo.Session) error {
	c := a.config()
	roles, e := s.GuildRoles(c.GuildID)
	if e != nil {
		return e
	}
	for _, id := range c.Community.TicketStaffRoles {
		found := false
		for _, r := range roles {
			if r.ID == id && id != c.GuildID {
				found = true
			}
		}
		if !found {
			return errors.New("A selected ticket staff role no longer exists")
		}
	}
	return nil
}
func ticketOverwrites(g, bot, owner string, staff []string, closed bool) []*discordgo.PermissionOverwrite {
	allow := int64(discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionReadMessageHistory | discordgo.PermissionAttachFiles | discordgo.PermissionEmbedLinks)
	ownerAllow := allow
	var deny int64
	if closed {
		ownerAllow &^= discordgo.PermissionSendMessages
		deny = discordgo.PermissionSendMessages
	}
	out := []*discordgo.PermissionOverwrite{{ID: g, Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionViewChannel}, {ID: bot, Type: discordgo.PermissionOverwriteTypeMember, Allow: allow | discordgo.PermissionManageChannels}, {ID: owner, Type: discordgo.PermissionOverwriteTypeMember, Allow: ownerAllow, Deny: deny}}
	for _, id := range staff {
		out = append(out, &discordgo.PermissionOverwrite{ID: id, Type: discordgo.PermissionOverwriteTypeRole, Allow: allow})
	}
	return out
}
func ticketPayload(t Ticket) *discordgo.MessageSend {
	title := "Private ticket: " + t.Subject
	desc := t.Body + "\n\nOpened by <@" + t.Owner + ">. Staff will reply here."
	label := "Close ticket"
	action := "close"
	style := discordgo.DangerButton
	if t.Closed {
		title = "Closed ticket: " + t.Subject
		desc += "\n\nThis ticket is closed. Its history is retained in this private channel."
		label = "Reopen ticket (staff)"
		action = "reopen"
		style = discordgo.SecondaryButton
	}
	return &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{{Title: clip(title, 256), Description: clip(desc, 4096), Color: 0xb18aff, Footer: &discordgo.MessageEmbedFooter{Text: "Ticket " + t.ID}}}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.Button{CustomID: "mw:" + action + ":" + t.ID, Label: label, Style: style}}}}, AllowedMentions: noMentions()}
}
func (a *App) createTicket(s *discordgo.Session, m *discordgo.Member, subject, body string) (string, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	c := a.config()
	if utf8.RuneCountInString(subject) < 1 || utf8.RuneCountInString(subject) > 100 || utf8.RuneCountInString(body) < 1 || utf8.RuneCountInString(body) > 1800 {
		return "", errors.New("Enter a subject (up to 100 characters) and details (up to 1800)")
	}
	if len(c.Community.TicketStaffRoles) == 0 {
		return "", errors.New("Tickets have not been configured yet")
	}
	if e := a.checkStaffRoles(s); e != nil {
		return "", e
	}
	v := a.store.community(c.GuildID)
	count, total := 0, 0
	for _, t := range v.Tickets {
		if !t.Closed {
			total++
			if t.Owner == m.User.ID {
				count++
			}
		}
		if t.Owner == m.User.ID && time.Since(t.Created) < time.Minute {
			return "", errors.New("Wait a minute before opening another ticket")
		}
	}
	if count >= 2 || total >= 100 || len(v.Tickets) >= 2000 {
		return "", errors.New("Ticket limit reached. Close an existing ticket or ask staff for help")
	}
	if c.Community.TicketCategory != "" {
		if _, e := a.checkCommunityChannel(s, c.Community.TicketCategory, discordgo.ChannelTypeGuildCategory); e != nil {
			return "", e
		}
	}
	id, e := newEmbedID()
	if e != nil {
		return "", e
	}
	staff := append([]string{}, c.Community.TicketStaffRoles...)
	ch, e := s.GuildChannelCreateComplex(c.GuildID, discordgo.GuildChannelCreateData{Name: "ticket-" + id[:8], Type: discordgo.ChannelTypeGuildText, ParentID: c.Community.TicketCategory, PermissionOverwrites: ticketOverwrites(c.GuildID, s.State.User.ID, m.User.ID, staff, false)})
	if e != nil {
		return "", e
	}
	t := Ticket{ID: id, Owner: m.User.ID, ChannelID: ch.ID, StaffRoles: staff, Subject: subject, Body: body, Created: time.Now().UTC()}
	msg, e := a.sendStable(s, ch.ID, "ticket:"+id, ticketPayload(t))
	if e != nil {
		s.ChannelDelete(ch.ID)
		return "", e
	}
	t.MessageID = msg.ID
	if e = a.store.updateCommunity(c.GuildID, func(v *CommunityState) error { v.Tickets[id] = t; return nil }); e != nil {
		s.ChannelDelete(ch.ID)
		return "", e
	}
	return "Your private ticket is ready: <#" + ch.ID + ">.", nil
}
func ticketStaff(m *discordgo.Member, t Ticket) bool {
	if hasPermission(m.Permissions, discordgo.PermissionManageChannels) {
		return true
	}
	for _, r := range m.Roles {
		for _, id := range t.StaffRoles {
			if r == id {
				return true
			}
		}
	}
	return false
}
func (a *App) setTicketClosed(s *discordgo.Session, m *discordgo.Member, t Ticket, closed bool) (string, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	g := a.config().GuildID
	current, ok := a.store.community(g).Tickets[t.ID]
	if !ok {
		return "", errors.New("Ticket not found")
	}
	t = current
	if !ticketStaff(m, t) && (!closed || m.User.ID != t.Owner) {
		return "", errors.New("Only the ticket owner or staff can close it; only staff can reopen it")
	}
	if t.Closed == closed {
		return "Ticket already has that status.", nil
	}
	if _, e := a.checkCommunityChannel(s, t.ChannelID, discordgo.ChannelTypeGuildText); e != nil {
		return "", e
	}
	old := t
	t.Closed = closed
	t.ClosedBy = m.User.ID
	now := time.Now().UTC()
	t.ClosedAt = &now
	if !closed {
		t.ClosedAt = nil
	}
	if _, e := s.ChannelEdit(t.ChannelID, &discordgo.ChannelEdit{PermissionOverwrites: ticketOverwrites(g, s.State.User.ID, t.Owner, t.StaffRoles, closed)}); e != nil {
		return "", e
	}
	if e := a.store.updateCommunity(g, func(v *CommunityState) error { v.Tickets[t.ID] = t; return nil }); e != nil {
		s.ChannelEdit(t.ChannelID, &discordgo.ChannelEdit{PermissionOverwrites: ticketOverwrites(g, s.State.User.ID, old.Owner, old.StaffRoles, old.Closed)})
		return "", e
	}
	p := ticketPayload(t)
	if _, e := s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: t.MessageID, Channel: t.ChannelID, Embeds: &p.Embeds, Components: &p.Components, AllowedMentions: noMentions()}); e != nil {
		a.log("Ticket status saved but card could not refresh: " + a.errorText(e))
	}
	if closed {
		return "Ticket closed. The private channel and its history are retained.", nil
	}
	return "Ticket reopened.", nil
}
func submissionPayload(x Submission) *discordgo.MessageSend {
	up, down := 0, 0
	for _, v := range x.Votes {
		if v == 1 {
			up++
		} else if v == -1 {
			down++
		}
	}
	fields := []*discordgo.MessageEmbedField{}
	if x.Kind == "bug" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Versions", Value: x.Version}, &discordgo.MessageEmbedField{Name: "Expected behavior", Value: x.Expected})
	}
	if x.ImageURL != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Screenshot / clip", Value: x.ImageURL})
	}
	desc := x.Body + "\n\nBy <@" + x.Author + "> · **" + x.Status + "**"
	choices := []discordgo.SelectMenuOption{}
	for _, status := range []string{"New", "Under review", "Planned", "In progress", "Fixed", "Declined"} {
		choices = append(choices, discordgo.SelectMenuOption{Label: status, Value: status})
	}
	return &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{{Title: clip(x.Title, 256), Description: desc, Color: 0xb18aff, Fields: fields, Footer: &discordgo.MessageEmbedFooter{Text: x.Kind + " · " + x.ID}}}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.Button{CustomID: "mw:up:" + x.ID, Label: "Support · " + strconv.Itoa(up), Style: discordgo.SuccessButton}, discordgo.Button{CustomID: "mw:down:" + x.ID, Label: "Against · " + strconv.Itoa(down), Style: discordgo.SecondaryButton}, discordgo.Button{CustomID: "mw:clearvote:" + x.ID, Label: "Remove vote", Style: discordgo.SecondaryButton}}}, discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.SelectMenu{CustomID: "mw:status:" + x.ID, Placeholder: "Status (moderators only)", MinValues: ptr(1), MaxValues: 1, Options: choices}}}}, AllowedMentions: noMentions()}
}
func validateSubmissionValues(kind string, v map[string]string) error {
	for _, x := range []struct {
		key      string
		max      int
		required bool
	}{{"title", 100, true}, {"body", 1800, true}, {"version", 180, kind == "bug"}, {"expected", 800, kind == "bug"}, {"image", 300, false}} {
		n := utf8.RuneCountInString(v[x.key])
		if n > x.max || (x.required && n == 0) {
			return fmt.Errorf("%s needs 1–%d characters", x.key, x.max)
		}
	}
	if v["image"] != "" && !validEmbedURL(v["image"]) {
		return errors.New("Screenshot or clip links must use HTTPS")
	}
	return nil
}
func (a *App) createSubmission(s *discordgo.Session, m *discordgo.Member, kind string, values map[string]string) (string, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	c := a.config()
	if e := validateSubmissionValues(kind, values); e != nil {
		return "", e
	}
	ch := c.Community.SuggestChannel
	if kind == "bug" {
		ch = c.Community.BugChannel
	}
	if e := a.postingChannel(s, ch); e != nil {
		return "", e
	}
	v := a.store.community(c.GuildID)
	if len(v.Submissions) >= 2500 {
		return "", errors.New("Submission limit reached (2500)")
	}
	for _, x := range v.Submissions {
		if x.Author == m.User.ID && time.Since(x.Created) < time.Minute {
			return "", errors.New("Wait a minute before sending another submission")
		}
	}
	id, e := newEmbedID()
	if e != nil {
		return "", e
	}
	x := Submission{ID: id, Kind: kind, Author: m.User.ID, Title: values["title"], Body: values["body"], Version: values["version"], Expected: values["expected"], ImageURL: values["image"], Status: "New", ChannelID: ch, Votes: map[string]int{}, Created: time.Now().UTC()}
	msg, e := a.sendStable(s, ch, "submission:"+id, submissionPayload(x))
	if e != nil {
		return "", e
	}
	x.MessageID = msg.ID
	if e = a.store.updateCommunity(c.GuildID, func(v *CommunityState) error { v.Submissions[id] = x; return nil }); e != nil {
		s.ChannelMessageDelete(ch, msg.ID)
		return "", e
	}
	return "Thanks! Your submission is posted: https://discord.com/channels/" + c.GuildID + "/" + ch + "/" + msg.ID, nil
}
func (a *App) updateSubmission(s *discordgo.Session, i *discordgo.InteractionCreate, kind, id string) (string, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	g := a.config().GuildID
	v := a.store.community(g)
	x, ok := v.Submissions[id]
	if !ok || i.Message == nil || i.Message.ID != x.MessageID || i.ChannelID != x.ChannelID {
		return "", errors.New("Invalid submission control")
	}
	if kind == "status" {
		if !hasPermission(i.Member.Permissions, discordgo.PermissionManageMessages) {
			return "", errors.New("Only moderators can change submission status")
		}
		vals := i.MessageComponentData().Values
		if len(vals) != 1 || !validSubmissionStatus(vals[0]) {
			return "", errors.New("Invalid status")
		}
		x.Status = vals[0]
		x.UpdatedBy = i.Member.User.ID
	} else {
		if x.Votes == nil {
			x.Votes = map[string]int{}
		}
		if len(x.Votes) >= 5000 && x.Votes[i.Member.User.ID] == 0 && kind != "clearvote" {
			return "", errors.New("Vote limit reached")
		}
		switch kind {
		case "up":
			x.Votes[i.Member.User.ID] = 1
		case "down":
			x.Votes[i.Member.User.ID] = -1
		case "clearvote":
			delete(x.Votes, i.Member.User.ID)
		}
	}
	if e := a.store.updateCommunity(g, func(v *CommunityState) error { v.Submissions[id] = x; return nil }); e != nil {
		return "", e
	}
	p := submissionPayload(x)
	if _, e := s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: x.MessageID, Channel: x.ChannelID, Embeds: &p.Embeds, Components: &p.Components, AllowedMentions: noMentions()}); e != nil {
		return "Saved your change, but the card could not refresh. Try again shortly.", nil
	}
	if kind == "status" {
		return "Status updated to " + x.Status + ".", nil
	}
	return "Your vote has been saved.", nil
}

func validSubmissionStatus(s string) bool {
	switch s {
	case "New", "Under review", "Planned", "In progress", "Fixed", "Declined":
		return true
	}
	return false
}
