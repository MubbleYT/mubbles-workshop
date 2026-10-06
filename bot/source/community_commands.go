package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	for n, p := range map[string]int64{"cases": discordgo.PermissionModerateMembers, "case": discordgo.PermissionModerateMembers, "case-note": discordgo.PermissionModerateMembers, "case-reverse": discordgo.PermissionModerateMembers, "raid-mode": discordgo.PermissionModerateMembers, "release": discordgo.PermissionManageServer, "live": discordgo.PermissionManageServer, "poll": discordgo.PermissionManageMessages, "event": discordgo.PermissionManageEvents} {
		commandPermissions[n] = p
	}
}
func communityCommands() []*discordgo.ApplicationCommand {
	str := func(n, d string, required bool, max int) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: discordgo.ApplicationCommandOptionString, Name: n, Description: d, Required: required, MaxLength: max}
	}
	integer := func(n, d string, min, max float64, required bool) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: discordgo.ApplicationCommandOptionInteger, Name: n, Description: d, Required: required, MinValue: ptr(min), MaxValue: max}
	}
	channel := func(required bool) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Where to post", Required: required, ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews}}
	}
	all := []*discordgo.ApplicationCommand{
		{Name: "roles", Description: "Find the interests and notification role menu"},
		{Name: "ticket", Description: "Contact moderators through a private ticket"},
		{Name: "report", Description: "Report a server issue privately to moderators"},
		{Name: "suggest", Description: "Send a public idea for a video or project"},
		{Name: "bugreport", Description: "Send a structured public mod or shader bug report"},
		{Name: "ticket-close", Description: "Close your current ticket and retain its history"},
		{Name: "faq", Description: "Read a configured FAQ answer", Options: []*discordgo.ApplicationCommandOption{str("topic", "FAQ topic, e.g. mods or shaders", true, 32)}},
		{Name: "cases", Description: "View recent moderation cases", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionUser, Name: "member", Description: "Filter by member", Required: false}}},
		{Name: "case", Description: "Read a moderation case", Options: []*discordgo.ApplicationCommandOption{str("id", "Case number", true, 20)}},
		{Name: "case-note", Description: "Add a note to a moderation case", Options: []*discordgo.ApplicationCommandOption{str("id", "Case number", true, 20), str("note", "Staff note", true, 400)}},
		{Name: "case-reverse", Description: "Reverse a ban, timeout or saved warning", Options: []*discordgo.ApplicationCommandOption{str("id", "Case number", true, 20), str("reason", "Why the action is being reversed", true, 400), {Type: discordgo.ApplicationCommandOptionBoolean, Name: "confirm", Description: "Set true to confirm", Required: true}}},
		{Name: "raid-mode", Description: "Temporarily restrict newcomers, or release raid restrictions", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "mode", Description: "Turn raid mode on or off", Required: true, Choices: []*discordgo.ApplicationCommandOptionChoice{{Name: "On", Value: "on"}, {Name: "Off", Value: "off"}}}, integer("minutes", "How long raid mode stays active", 1, 60, false)}},
		{Name: "voice", Description: "Rename or set the size of your temporary voice room", Options: []*discordgo.ApplicationCommandOption{str("name", "New room name", false, 90), integer("limit", "Member limit", 1, 99, false)}},
		{Name: "release", Description: "Announce a mod, shader or project release", Options: []*discordgo.ApplicationCommandOption{str("project", "Project name", true, 100), str("version", "Release version", true, 40), str("changes", "What changed", true, 1800), str("url", "HTTPS download link", true, 300)}},
		{Name: "live", Description: "Announce that you are live (manual notification)", Options: []*discordgo.ApplicationCommandOption{str("title", "Stream title", true, 100), str("url", "HTTPS stream link", true, 300)}},
		{Name: "poll", Description: "Post a native Discord poll", Options: []*discordgo.ApplicationCommandOption{str("question", "Poll question", true, 300), str("answers", "2–10 answers separated by |", true, 1000), channel(true), integer("hours", "Poll duration in hours", 1, 768, false), {Type: discordgo.ApplicationCommandOptionBoolean, Name: "multiple", Description: "Allow multiple answers", Required: false}}},
		{Name: "event", Description: "Create a community event and optional reminder", Options: []*discordgo.ApplicationCommandOption{str("name", "Event name", true, 100), str("start", "Start with timezone, e.g. 2026-10-04T18:00:00+02:00", true, 40), str("end", "End with timezone (same format)", true, 40), str("location", "Location or HTTPS link", true, 100), str("description", "What is happening", false, 1000), integer("reminder", "Minutes before start; 0 disables reminder", 0, 1440, false)}},
	}
	for _, name := range []string{"mods", "shaders", "youtube", "support"} {
		all = append(all, &discordgo.ApplicationCommand{Name: name, Description: "Show the " + name + " links and information"})
	}
	return all
}
func commandOptions(i *discordgo.InteractionCreate) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range i.ApplicationCommandData().Options {
		m[o.Name] = o
	}
	return m
}
func optionString(m map[string]*discordgo.ApplicationCommandInteractionDataOption, n string) string {
	if o := m[n]; o != nil {
		v, _ := o.Value.(string)
		return v
	}
	return ""
}
func optionInt(m map[string]*discordgo.ApplicationCommandInteractionDataOption, n string, def int) int {
	if o := m[n]; o != nil {
		return int(o.IntValue())
	}
	return def
}
func (a *App) executeCommunityCommand(s *discordgo.Session, i *discordgo.InteractionCreate) (string, error, bool) {
	name := i.ApplicationCommandData().Name
	o := commandOptions(i)
	c := a.config()
	actor := i.Member.User.ID
	switch name {
	case "mods", "shaders", "youtube", "support", "faq":
		topic := name
		if name == "faq" {
			topic = strings.ToLower(optionString(o, "topic"))
		}
		for _, f := range c.Community.FAQ {
			if f.Name == topic {
				return f.Text, nil, true
			}
		}
		return "No answer configured for that topic. Ask a moderator to add it in the community panel.", nil, true
	case "roles":
		for _, p := range a.store.community(c.GuildID).Panels {
			if p.Kind == "roles" && p.ChannelID == c.Community.RolesChannel {
				return "Choose your interests and notifications here: https://discord.com/channels/" + c.GuildID + "/" + p.ChannelID + "/" + p.MessageID, nil, true
			}
		}
		return "Ask a moderator to publish the role menu first.", nil, true
	case "ticket-close":
		for _, t := range a.store.community(c.GuildID).Tickets {
			if t.ChannelID == i.ChannelID {
				text, e := a.setTicketClosed(s, i.Member, t, true)
				return text, e, true
			}
		}
		return "Use this command inside the ticket you want to close.", nil, true
	case "cases":
		target := optionString(o, "member")
		all := a.store.community(c.GuildID).Cases
		text := "**Recent moderation cases**\n"
		n := 0
		for j := len(all) - 1; j >= 0 && n < 10; j-- {
			x := all[j]
			if target != "" && x.Target != target {
				continue
			}
			text += fmt.Sprintf("#%s · %s · %s · %s\n%s\n", x.ID, x.Kind, x.Target, x.Created.Format("2006-01-02"), clip(x.Reason, 100))
			n++
		}
		if n == 0 {
			text = "No matching cases saved yet."
		}
		return text, nil, true
	case "case", "case-note", "case-reverse":
		id := strings.TrimPrefix(optionString(o, "id"), "#")
		x, ok := findCase(a.store.community(c.GuildID), id)
		if !ok {
			return "", errors.New("Case not found"), true
		}
		if name == "case" {
			text := fmt.Sprintf("**Case #%s · %s**\nMember: %s\nModerator: %s\nTime: %s\nReason: %s", x.ID, x.Kind, x.Target, x.Actor, x.Created.Format(time.RFC3339), x.Reason)
			if x.Until != nil {
				text += "\nUntil: " + x.Until.Format(time.RFC3339)
			}
			if x.ReversedBy != "" {
				text += "\nReversed by " + x.ReversedBy + " in case #" + x.ReversalCase
			}
			if x.Note != "" {
				text += "\nNotes: " + x.Note
			}
			return text, nil, true
		}
		if name == "case-note" {
			note := optionString(o, "note")
			err := a.store.updateCommunity(c.GuildID, func(v *CommunityState) error {
				for j := range v.Cases {
					if v.Cases[j].ID == id {
						v.Cases[j].Note = clip(v.Cases[j].Note+"\n"+time.Now().UTC().Format(time.RFC3339)+" · "+actor+": "+note, 1800)
						return nil
					}
				}
				return errors.New("Case not found")
			})
			return "Case note saved.", err, true
		}
		if o["confirm"] == nil || !o["confirm"].BoolValue() {
			return "No action taken. Set confirm to true.", nil, true
		}
		text, e := a.reverseCase(s, i, x, optionString(o, "reason"))
		return text, e, true
	case "raid-mode":
		if optionString(o, "mode") == "off" {
			e := a.endRaid(s, actor)
			return "Raid mode ended. Matching newcomer timeouts have been removed.", e, true
		}
		until := time.Now().Add(time.Duration(optionInt(o, "minutes", c.Community.RaidMinutes)) * time.Minute)
		e := a.store.updateCommunity(c.GuildID, func(v *CommunityState) error { v.RaidUntil = until; return nil })
		if e == nil {
			a.audit(s, "Raid mode enabled by "+actor+" until "+until.Format(time.RFC3339))
		}
		return "Raid mode is active. New human members will receive temporary timeouts; nobody is banned for joining.", e, true
	case "voice":
		vs, e := s.State.VoiceState(c.GuildID, actor)
		if e != nil {
			return "", errors.New("Join your temporary voice room first"), true
		}
		r, ok := a.store.community(c.GuildID).VoiceRooms[vs.ChannelID]
		if !ok || r.Owner != actor {
			return "", errors.New("You can only edit your own temporary voice room"), true
		}
		edit := &discordgo.ChannelEdit{Name: optionString(o, "name"), UserLimit: optionInt(o, "limit", 0)}
		if edit.Name == "" && edit.UserLimit == 0 {
			return "Use /voice name:… or /voice limit:… inside your room.", nil, true
		}
		_, e = s.ChannelEdit(vs.ChannelID, edit)
		return "Voice room updated.", e, true
	case "release", "live":
		post := AnnouncementRequest{Kind: name, Title: optionString(o, "project"), Version: optionString(o, "version"), Text: optionString(o, "changes"), URL: optionString(o, "url")}
		if name == "live" {
			post.Title = optionString(o, "title")
		}
		link, e := a.publishAnnouncement(s, post)
		return "Posted: " + link, e, true
	case "poll":
		p := PollRequest{ChannelID: optionString(o, "channel"), Question: optionString(o, "question"), Answers: strings.Split(optionString(o, "answers"), "|"), Hours: optionInt(o, "hours", 24)}
		if o["multiple"] != nil {
			p.Multiple = o["multiple"].BoolValue()
		}
		link, e := a.publishPoll(s, p)
		return "Poll posted: " + link, e, true
	case "event":
		request := EventRequest{Name: optionString(o, "name"), Start: optionString(o, "start"), End: optionString(o, "end"), Location: optionString(o, "location"), Description: optionString(o, "description"), ReminderMinutes: optionInt(o, "reminder", 30), ChannelID: c.Community.EventChannel}
		e, err := a.createCommunityEvent(s, request)
		return "Event created: https://discord.com/events/" + c.GuildID + "/" + e.ID, err, true
	}
	return "", nil, false
}
func (a *App) reverseCase(s *discordgo.Session, i *discordgo.InteractionCreate, x ModCase, reason string) (string, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	g := i.GuildID
	actor := i.Member.User.ID
	state := a.store.community(g)
	x, ok := findCase(state, x.ID)
	if !ok || x.ReversedBy != "" {
		return "", errors.New("Case is missing or already reversed")
	}
	kind := x.Kind
	p := int64(0)
	switch kind {
	case "ban":
		p = discordgo.PermissionBanMembers
	case "timeout", "raid-timeout":
		p = discordgo.PermissionModerateMembers
	case "warn":
		p = discordgo.PermissionModerateMembers
	default:
		return "", errors.New("That action cannot be reversed automatically. You can add a case note")
	}
	if !hasPermission(i.Member.Permissions, p) {
		return "", errors.New("You lack the permission required to reverse this action")
	}
	if err := a.botGuildPermission(s, p); err != nil {
		return "", err
	}
	for _, later := range state.Cases {
		if kind != "warn" && later.Created.After(x.Created) && later.Target == x.Target && (later.Kind == kind || (kind == "raid-timeout" && later.Kind == "timeout")) && later.ReversedBy == "" {
			return "", errors.New("A newer action exists for this member. Review the latest case instead")
		}
	}
	switch kind {
	case "ban":
		if err := s.GuildBanDelete(g, x.Target, discordgo.WithAuditLogReason(actor+": "+reason)); err != nil {
			return "", err
		}
	case "timeout", "raid-timeout":
		if err := a.checkTarget(s, g, actor, x.Target, "untimeout"); err != nil {
			return "", err
		}
		m, e := s.GuildMember(g, x.Target)
		if e != nil {
			return "", e
		}
		if x.Until == nil || m.CommunicationDisabledUntil == nil || m.CommunicationDisabledUntil.Sub(*x.Until).Abs() > 2*time.Second {
			return "", errors.New("The current timeout differs from this case or has already ended")
		}
		if e = s.GuildMemberTimeout(g, x.Target, nil, discordgo.WithAuditLogReason(actor+": "+reason)); e != nil {
			return "", e
		}
	case "warn":
		if err := a.checkTarget(s, g, actor, x.Target, "remove-warning"); err != nil {
			return "", err
		}
		if err := a.store.removeWarning(g, x.Target, x.WarningID); err != nil {
			return "", err
		}
	}
	id := a.recordCase(s, "reverse-"+kind, actor, x.Target, x.ChannelID, reason, "", nil)
	if id == "" {
		return "Action reversed, but the reversal record could not be saved. Check activity.", nil
	}
	if e := a.markCaseReversed(g, x.ID, actor, id); e != nil {
		return "Action reversed, but the original case could not be updated. Check activity.", nil
	}
	return "Case #" + x.ID + " reversed. Recorded as case #" + id + ".", nil
}
func (a *App) markCaseReversed(g, id, actor, newID string) error {
	return a.store.updateCommunity(g, func(v *CommunityState) error {
		for j := range v.Cases {
			if v.Cases[j].ID == id {
				v.Cases[j].ReversedBy = actor
				v.Cases[j].ReversalCase = newID
				return nil
			}
		}
		return errors.New("Original case not found")
	})
}
func (a *App) linkManualReversal(g, target, kind, warning, actor, newID string) {
	all := a.store.community(g).Cases
	for j := len(all) - 1; j >= 0; j-- {
		x := all[j]
		if x.Target == target && x.ReversedBy == "" && (x.Kind == kind || (kind == "timeout" && x.Kind == "raid-timeout")) && (warning == "" || x.WarningID == warning) {
			if e := a.markCaseReversed(g, x.ID, actor, newID); e != nil {
				a.log("Case link: " + e.Error())
			}
			return
		}
	}
}
