package main

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
)

func ptr[T any](v T) *T { return &v }

var commandPermissions = map[string]int64{
	"warn": discordgo.PermissionModerateMembers, "warnings": discordgo.PermissionModerateMembers, "remove-warning": discordgo.PermissionModerateMembers,
	"timeout": discordgo.PermissionModerateMembers, "untimeout": discordgo.PermissionModerateMembers,
	"kick": discordgo.PermissionKickMembers, "ban": discordgo.PermissionBanMembers, "unban": discordgo.PermissionBanMembers,
	"purge": discordgo.PermissionManageMessages, "slowmode": discordgo.PermissionManageChannels, "youtube-test": discordgo.PermissionManageServer,
}

func commands() []*discordgo.ApplicationCommand {
	user := func() *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: discordgo.ApplicationCommandOptionUser, Name: "member", Description: "Member to moderate", Required: true}
	}
	reason := func() *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for the action", Required: true, MaxLength: 400}
	}
	integer := func(n, d string, min, max float64) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: discordgo.ApplicationCommandOptionInteger, Name: n, Description: d, Required: true, MinValue: ptr(min), MaxValue: max}
	}
	confirm := func() *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: discordgo.ApplicationCommandOptionBoolean, Name: "confirm", Description: "Set true to confirm this action", Required: true}
	}
	all := []*discordgo.ApplicationCommand{
		{Name: "help", Description: "Show the bot's commands"}, {Name: "ping", Description: "Check whether the bot is responding"},
		{Name: "warn", Description: "Save a warning for a member", Options: []*discordgo.ApplicationCommandOption{user(), reason()}},
		{Name: "warnings", Description: "View saved warnings for a member", Options: []*discordgo.ApplicationCommandOption{user()}},
		{Name: "remove-warning", Description: "Remove one saved warning", Options: []*discordgo.ApplicationCommandOption{user(), {Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Warning ID from /warnings", Required: true}}},
		{Name: "timeout", Description: "Timeout a member for up to 28 days", Options: []*discordgo.ApplicationCommandOption{user(), integer("minutes", "Timeout duration in minutes", 1, 40320), reason()}},
		{Name: "untimeout", Description: "Remove a member's timeout", Options: []*discordgo.ApplicationCommandOption{user(), reason()}},
		{Name: "kick", Description: "Remove a member from the server", Options: []*discordgo.ApplicationCommandOption{user(), reason(), confirm()}},
		{Name: "ban", Description: "Ban a member (keeps message history)", Options: []*discordgo.ApplicationCommandOption{user(), reason(), confirm()}},
		{Name: "unban", Description: "Unban a user by their Discord user ID", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "user_id", Description: "Discord user ID", Required: true}, reason()}},
		{Name: "purge", Description: "Delete up to 100 recent messages in this channel", Options: []*discordgo.ApplicationCommandOption{integer("count", "Messages to delete; skips messages older than 14 days", 1, 100), confirm()}},
		{Name: "slowmode", Description: "Change this channel's slowmode", Options: []*discordgo.ApplicationCommandOption{integer("seconds", "Seconds between messages; 0 disables slowmode", 0, 21600)}},
		{Name: "youtube-test", Description: "Post the latest video as a test without pinging anyone"},
	}
	all = append(all, communityCommands()...)
	for _, c := range all {
		c.DMPermission = ptr(false)
		if p := commandPermissions[c.Name]; p != 0 {
			c.DefaultMemberPermissions = ptr(p)
		}
	}
	return all
}
func (a *App) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if a.handleCommunityInteraction(s, i) {
		return
	}
	if i.Type == discordgo.InteractionMessageComponent {
		a.onRulesButton(s, i)
		return
	}
	if i.Type != discordgo.InteractionApplicationCommand || i.GuildID != a.config().GuildID || i.Member == nil || i.Member.User == nil {
		return
	}
	data := i.ApplicationCommandData()
	p := commandPermissions[data.Name]
	if p != 0 && !hasPermission(i.Member.Permissions, p) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: "You do not have permission to use this command.", Flags: discordgo.MessageFlagsEphemeral}})
		return
	}
	if e := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral}}); e != nil {
		a.log("Could not acknowledge command: " + a.errorText(e))
		return
	}
	text, e := a.executeCommand(s, i)
	if e != nil {
		text = "Action failed: " + a.errorText(e)
		a.log("/" + data.Name + ": " + a.errorText(e))
	}
	if len(text) > 1900 {
		text = text[:1900] + "…"
	}
	if _, e = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &text, AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}}); e != nil {
		a.log("Could not send command response: " + a.errorText(e))
	}
}
func (a *App) executeCommand(s *discordgo.Session, i *discordgo.InteractionCreate) (string, error) {
	d := i.ApplicationCommandData()
	if text, err, handled := a.executeCommunityCommand(s, i); handled {
		return text, err
	}
	opts := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range d.Options {
		opts[o.Name] = o
	}
	str := func(n string) string {
		if o := opts[n]; o != nil {
			return o.StringValue()
		}
		return ""
	}
	reason := str("reason")
	actor := i.Member.User.ID
	if d.Name == "help" {
		return "**Mubble's Bot**\n`/warn` · `/warnings` · `/remove-warning`\n`/timeout` · `/untimeout` · `/kick` · `/ban` · `/unban`\n`/purge` · `/slowmode` · `/youtube-test` · `/ping`\n/roles · /ticket · /report · /suggest · /bugreport · /faq\n/mods · /shaders · /youtube · /support · /voice\nStaff: /cases · /case · /case-note · /case-reverse · /raid-mode\nPosts: /release · /live · /poll · /event\nModeration commands require the matching Discord permissions. Configure features in your local panel.", nil
	}
	if d.Name == "ping" {
		return fmt.Sprintf("Pong! Gateway latency: %d ms.", s.HeartbeatLatency().Milliseconds()), nil
	}
	if d.Name == "youtube-test" {
		if e := a.testVideo(s); e != nil {
			return "", e
		}
		return "Latest video posted as a test, without a role ping.", nil
	}
	if d.Name == "kick" || d.Name == "ban" || d.Name == "purge" {
		if opts["confirm"] == nil || !opts["confirm"].BoolValue() {
			return "No action taken. Set `confirm` to true to proceed.", nil
		}
	}
	if d.Name == "purge" {
		p, e := s.UserChannelPermissions(s.State.User.ID, i.ChannelID)
		if e != nil {
			return "", e
		}
		if !hasPermission(p, discordgo.PermissionManageMessages) {
			return "", errors.New("The bot needs Manage Messages in this channel")
		}
		n := int(opts["count"].IntValue())
		messages, e := s.ChannelMessages(i.ChannelID, n, "", "", "")
		if e != nil {
			return "", e
		}
		ids := []string{}
		for _, m := range messages {
			t, e := discordgo.SnowflakeTimestamp(m.ID)
			if e == nil && t.After(time.Now().Add(-14*24*time.Hour+time.Minute)) {
				ids = append(ids, m.ID)
			}
		}
		if len(ids) == 0 {
			return "No messages were deleted; messages older than 14 days are skipped.", nil
		}
		if e = s.ChannelMessagesBulkDelete(i.ChannelID, ids); e != nil {
			return "", e
		}
		a.recordCase(s, "purge", actor, "", i.ChannelID, fmt.Sprintf("Deleted %d recent messages", len(ids)), "", nil)
		return fmt.Sprintf("Deleted %d messages. Older messages were skipped.", len(ids)), nil
	}
	if d.Name == "slowmode" {
		p, e := s.UserChannelPermissions(s.State.User.ID, i.ChannelID)
		if e != nil {
			return "", e
		}
		if !hasPermission(p, discordgo.PermissionManageChannels) {
			return "", errors.New("The bot needs Manage Channels in this channel")
		}
		n := int(opts["seconds"].IntValue())
		if _, e = s.ChannelEdit(i.ChannelID, &discordgo.ChannelEdit{RateLimitPerUser: &n}); e != nil {
			return "", e
		}
		a.recordCase(s, "slowmode", actor, "", i.ChannelID, fmt.Sprintf("Set slowmode to %d seconds", n), "", nil)
		return fmt.Sprintf("Slowmode set to %d seconds.", n), nil
	}
	if d.Name == "unban" {
		id := str("user_id")
		if !validID(id) {
			return "", errors.New("Invalid Discord user ID")
		}
		if e := s.GuildBanDelete(i.GuildID, id, discordgo.WithAuditLogReason(actor+": "+reason)); e != nil {
			return "", e
		}
		caseID := a.recordCase(s, "unban", actor, id, i.ChannelID, reason, "", nil)
		a.linkManualReversal(i.GuildID, id, "ban", "", actor, caseID)
		return "User unbanned.", nil
	}
	o := opts["member"]
	if o == nil {
		return "", errors.New("Unknown command")
	}
	target, ok := o.Value.(string)
	if !ok {
		return "", errors.New("Invalid member")
	}
	if d.Name == "warnings" {
		all := a.store.warnings(i.GuildID, target)
		if len(all) == 0 {
			return "This member has no saved warnings.", nil
		}
		out := fmt.Sprintf("**%d warnings** (showing latest 10):\n", len(all))
		start := len(all) - 10
		if start < 0 {
			start = 0
		}
		for _, w := range all[start:] {
			out += fmt.Sprintf("**#%s** · %s · moderator %s\n%s\n", w.ID, w.Created.Format("2006-01-02"), w.ModeratorID, clip(w.Reason, 120))
		}
		return out, nil
	}
	if e := a.checkTarget(s, i.GuildID, actor, target, d.Name); e != nil {
		return "", e
	}
	var caseUntil *time.Time
	switch d.Name {
	case "warn":
		w, e := a.store.warn(i.GuildID, target, actor, reason)
		if e != nil {
			return "", e
		}
		a.recordCase(s, "warn", actor, target, i.ChannelID, reason, w.ID, nil)
		return "Saved warning #" + w.ID + ".", nil
	case "remove-warning":
		if e := a.store.removeWarning(i.GuildID, target, str("id")); e != nil {
			return "", e
		}
		caseID := a.recordCase(s, "remove-warning", actor, target, i.ChannelID, "Removed warning #"+str("id"), str("id"), nil)
		a.linkManualReversal(i.GuildID, target, "warn", str("id"), actor, caseID)
		return "Warning removed.", nil
	case "timeout":
		until := time.Now().Add(time.Duration(opts["minutes"].IntValue()) * time.Minute)
		caseUntil = &until
		if e := s.GuildMemberTimeout(i.GuildID, target, &until, discordgo.WithAuditLogReason(actor+": "+reason)); e != nil {
			return "", e
		}
	case "untimeout":
		if e := s.GuildMemberTimeout(i.GuildID, target, nil, discordgo.WithAuditLogReason(actor+": "+reason)); e != nil {
			return "", e
		}
	case "kick":
		if e := s.GuildMemberDeleteWithReason(i.GuildID, target, actor+": "+reason); e != nil {
			return "", e
		}
	case "ban":
		if e := s.GuildBanCreateWithReason(i.GuildID, target, actor+": "+reason, 0); e != nil {
			return "", e
		}
	default:
		return "", errors.New("Unknown command")
	}
	caseID := a.recordCase(s, d.Name, actor, target, i.ChannelID, reason, "", caseUntil)
	if d.Name == "untimeout" {
		a.linkManualReversal(i.GuildID, target, "timeout", "", actor, caseID)
	}
	return "Done: /" + d.Name + " applied to the member.", nil
}
func hasPermission(p, w int64) bool { return p&discordgo.PermissionAdministrator != 0 || p&w == w }
func rolePosition(m *discordgo.Member, roles []*discordgo.Role) int {
	n := 0
	for _, r := range roles {
		for _, id := range m.Roles {
			if r.ID == id && r.Position > n {
				n = r.Position
			}
		}
	}
	return n
}
func guildPermissions(m *discordgo.Member, g string, roles []*discordgo.Role) int64 {
	var p int64
	for _, r := range roles {
		if r.ID == g {
			p |= r.Permissions
		}
		for _, id := range m.Roles {
			if id == r.ID {
				p |= r.Permissions
			}
		}
	}
	return p
}
func (a *App) checkTarget(s *discordgo.Session, g, actor, target, action string) error {
	if target == actor || target == s.State.User.ID {
		return errors.New("You cannot moderate yourself or this bot")
	}
	guild, e := s.Guild(g)
	if e != nil {
		return e
	}
	if target == guild.OwnerID {
		return errors.New("The server owner cannot be moderated")
	}
	roles, e := s.GuildRoles(g)
	if e != nil {
		return e
	}
	tm, e := s.GuildMember(g, target)
	if e != nil {
		return e
	}
	am, e := s.GuildMember(g, actor)
	if e != nil {
		return e
	}
	bm, e := s.GuildMember(g, s.State.User.ID)
	if e != nil {
		return e
	}
	if actor != guild.OwnerID && rolePosition(am, roles) <= rolePosition(tm, roles) {
		return errors.New("Your highest role must be above the member's highest role")
	}
	if action != "warn" && action != "remove-warning" && rolePosition(bm, roles) <= rolePosition(tm, roles) {
		return errors.New("Move the bot's role above this member's highest role")
	}
	if (action == "timeout" || action == "untimeout") && hasPermission(guildPermissions(tm, g, roles), discordgo.PermissionAdministrator) {
		return errors.New("Discord does not allow timeouts on administrators")
	}
	return nil
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
func (a *App) audit(s *discordgo.Session, text string) {
	a.log(text)
	c := a.config()
	if c.LogChannelID != "" {
		if _, e := s.ChannelMessageSendComplex(c.LogChannelID, &discordgo.MessageSend{Content: clip(text, 1800), AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}}); e != nil {
			a.log("Could not write moderation log: " + a.errorText(e))
		}
	}
}

func (a *App) assignRole(s *discordgo.Session, m *discordgo.Member) {
	c := a.config()
	if m == nil || m.User == nil || m.GuildID != c.GuildID || m.User.Bot || m.Pending || c.MemberRoleID == "" {
		return
	}
	for _, id := range m.Roles {
		if id == c.MemberRoleID {
			return
		}
	}
	if e := a.validatePublicRole(s, c.GuildID, c.MemberRoleID); e != nil {
		a.log("Auto-role configuration: " + a.errorText(e))
		return
	}
	if e := s.GuildMemberRoleAdd(c.GuildID, m.User.ID, c.MemberRoleID); e != nil {
		a.log("Auto-role failed for " + m.User.ID + ": " + a.errorText(e))
		return
	}
	a.audit(s, "Assigned Member role to "+m.User.ID)
}

var inviteRE = regexp.MustCompile(`(?i)(?:discord\.gg/|discord(?:app)?\.com/invite/)`)
var linkRE = regexp.MustCompile(`(?i)(?:https?://|www\.)`)

func (a *App) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	a.moderateMessage(s, m, true)
}
func (a *App) moderateMessage(s *discordgo.Session, m *discordgo.MessageCreate, countSpam bool) {
	c := a.config()
	if m.Author == nil || m.GuildID != c.GuildID {
		return
	}
	if s.State.User != nil && m.Author.ID == s.State.User.ID {
		return
	}
	if c.TrapChannelID != "" && m.ChannelID == c.TrapChannelID {
		if m.WebhookID != "" {
			if err := s.ChannelMessageDelete(m.ChannelID, m.ID); err != nil {
				a.log("Could not delete trap-channel webhook message: " + a.errorText(err))
			}
			a.audit(s, "Removed a bot-trap webhook message. Webhooks are not server members and cannot be banned.")
			return
		}
		a.banForPolicy(s, m, "posting in the bot-trap channel")
		return
	}
	if m.Author.Bot || m.WebhookID != "" {
		return
	}
	if a.mentionProtection(s, m) {
		return
	}
	policy, err := a.languagePolicy(c)
	if err != nil {
		a.log("Language filter configuration error: " + err.Error())
		return
	}
	if c.BanSlurs && policy.slurs.count(m.Content) > 0 {
		a.banForSlur(s, m)
		return
	}
	p, e := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
	if e != nil {
		a.log("Could not check automod permissions: " + a.errorText(e))
		return
	}
	if hasPermission(p, discordgo.PermissionManageMessages) {
		return
	}
	reason := ""
	if c.LimitSwearing {
		count := policy.swears.count(m.Content)
		total := a.recordSwears(c, m.Author.ID, m.ID, count, time.Now())
		if count > c.SwearsPerMessage {
			reason = fmt.Sprintf("Swearing limit exceeded (%d words; maximum %d per message)", count, c.SwearsPerMessage)
		} else if count > 0 && total > c.SwearsPerWindow {
			reason = fmt.Sprintf("Swearing limit exceeded (%d words within %d seconds; maximum %d)", total, c.SwearWindow, c.SwearsPerWindow)
		}
	}
	if m.ChannelID == c.LogChannelID && reason == "" {
		return
	}
	if c.BlockInvites && inviteRE.MatchString(m.Content) {
		reason = "Discord invite link"
	} else if c.BlockLinks && linkRE.MatchString(m.Content) {
		reason = "External link"
	}
	key := m.GuildID + ":" + m.ChannelID + ":" + m.Author.ID
	now := time.Now()
	a.spamMu.Lock()
	if c.AntiSpam && countSpam {
		recent := []time.Time{}
		for _, t := range a.spam[key] {
			if now.Sub(t) < time.Duration(c.SpamWindow)*time.Second {
				recent = append(recent, t)
			}
		}
		recent = append(recent, now)
		if len(recent) > c.SpamCount {
			recent = recent[len(recent)-c.SpamCount:]
		}
		a.spam[key] = recent
		if len(recent) >= c.SpamCount {
			reason = "Message spam"
		}
	}
	report := now.Sub(a.cooldown[key]) > 30*time.Second
	if reason != "" && report {
		a.cooldown[key] = now
	}
	if len(a.spam) > 10000 {
		for k, v := range a.spam {
			if len(v) == 0 || now.Sub(v[len(v)-1]) > time.Minute {
				delete(a.spam, k)
			}
		}
	}
	if len(a.cooldown) > 10000 {
		for k, t := range a.cooldown {
			if now.Sub(t) > time.Minute {
				delete(a.cooldown, k)
			}
		}
	}
	a.spamMu.Unlock()
	if reason != "" {
		if e := s.ChannelMessageDelete(m.ChannelID, m.ID); e != nil {
			if report {
				a.log("Automod could not delete message: " + a.errorText(e))
			}
			return
		}
		if report {
			a.audit(s, "Automod deleted a message from "+m.Author.ID+" in <#"+m.ChannelID+">: "+reason)
		}
	}
}

type NamedID struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Inspection struct {
	Categories    []NamedID `json:"categories"`
	VoiceChannels []NamedID `json:"voice_channels"`
	AllRoles      []NamedID `json:"all_roles"`
	Roles         []NamedID `json:"roles"`
	Channels      []NamedID `json:"channels"`
	Checks        []string  `json:"checks"`
	Invite        string    `json:"invite"`
}

func (a *App) inspect(s *discordgo.Session) (Inspection, error) {
	c := a.config()
	out := Inspection{Roles: []NamedID{}, Channels: []NamedID{}, Checks: []string{}}
	roles, e := s.GuildRoles(c.GuildID)
	if e != nil {
		return out, e
	}
	m, e := s.GuildMember(c.GuildID, s.State.User.ID)
	if e != nil {
		return out, e
	}
	pos := rolePosition(m, roles)
	p := guildPermissions(m, c.GuildID, roles)
	for _, r := range roles {
		if r.ID != c.GuildID {
			out.AllRoles = append(out.AllRoles, NamedID{r.ID, r.Name})
		}
		if r.ID != c.GuildID && !r.Managed && r.Position < pos {
			out.Roles = append(out.Roles, NamedID{r.ID, r.Name})
		}
	}
	if c.MemberRoleID != "" {
		found := false
		for _, r := range roles {
			if r.ID == c.MemberRoleID {
				found = true
				if r.Managed || r.ID == c.GuildID || r.Position >= pos {
					return out, errors.New("Member role must be a normal role below the bot's highest role")
				}
			}
		}
		if !found {
			return out, errors.New("Member role does not exist")
		}
		if !hasPermission(p, discordgo.PermissionManageRoles) {
			return out, errors.New("The bot needs Manage Roles for automatic Member roles")
		}
		out.Checks = append(out.Checks, "Member role hierarchy is correct")
	}
	for _, check := range []struct {
		p int64
		n string
	}{{discordgo.PermissionKickMembers, "Kick Members"}, {discordgo.PermissionBanMembers, "Ban Members"}, {discordgo.PermissionModerateMembers, "Timeout Members"}, {discordgo.PermissionManageMessages, "Manage Messages"}, {discordgo.PermissionManageChannels, "Manage Channels"}} {
		if !hasPermission(p, check.p) {
			out.Checks = append(out.Checks, "Missing permission: "+check.n)
		}
	}
	channels, e := s.GuildChannels(c.GuildID)
	if e != nil {
		return out, e
	}
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildCategory {
			out.Categories = append(out.Categories, NamedID{ch.ID, ch.Name})
		}
		if ch.Type == discordgo.ChannelTypeGuildVoice {
			out.VoiceChannels = append(out.VoiceChannels, NamedID{ch.ID, ch.Name})
		}
		if ch.Type == discordgo.ChannelTypeGuildText || ch.Type == discordgo.ChannelTypeGuildNews {
			out.Channels = append(out.Channels, NamedID{ch.ID, ch.Name})
		}
	}
	for _, id := range []string{c.NotifyChannelID, c.LogChannelID} {
		if id != "" {
			ch, e := s.Channel(id)
			if e != nil {
				return out, e
			}
			if ch.GuildID != c.GuildID {
				return out, errors.New("Announcement and log channels must belong to the configured server")
			}
			perms, e := s.UserChannelPermissions(s.State.User.ID, id)
			if e != nil {
				return out, e
			}
			if !hasPermission(perms, discordgo.PermissionViewChannel|discordgo.PermissionSendMessages) {
				return out, fmt.Errorf("Bot cannot view/send messages in #%s", ch.Name)
			}
			if id == c.NotifyChannelID && !hasPermission(perms, discordgo.PermissionEmbedLinks) {
				return out, errors.New("Bot needs Embed Links in the video announcement channel")
			}
		}
	}
	if c.NotifyRoleID != "" {
		found := false
		for _, r := range roles {
			if r.ID == c.NotifyRoleID {
				found = true
				if !r.Mentionable && !hasPermission(p, discordgo.PermissionMentionEveryone) {
					out.Checks = append(out.Checks, "Make the upload notification role mentionable to enable pings")
				}
			}
		}
		if !found {
			return out, errors.New("Notification role does not exist")
		}
	}
	sort.Slice(out.Roles, func(i, j int) bool { return out.Roles[i].Name < out.Roles[j].Name })
	sort.Slice(out.Channels, func(i, j int) bool { return out.Channels[i].Name < out.Channels[j].Name })
	if c.TrapChannelID != "" {
		ch, err := s.Channel(c.TrapChannelID)
		if err != nil {
			return out, err
		}
		if ch.GuildID != c.GuildID {
			return out, errors.New("Bot-trap channel must belong to the configured server")
		}
		cp, err := s.UserChannelPermissions(s.State.User.ID, c.TrapChannelID)
		if err != nil {
			return out, err
		}
		if !hasPermission(cp, discordgo.PermissionViewChannel) {
			return out, errors.New("The bot must be able to view the bot-trap channel")
		}
		if !hasPermission(p, discordgo.PermissionBanMembers) {
			return out, errors.New("The bot needs Ban Members for the bot-trap channel")
		}
		out.Checks = append(out.Checks, "Bot-trap channel is visible; this bot is exempt from its automatic bans")
	}
	permissions := int64(discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionEmbedLinks | discordgo.PermissionReadMessageHistory | discordgo.PermissionManageMessages | discordgo.PermissionManageRoles | discordgo.PermissionKickMembers | discordgo.PermissionBanMembers | discordgo.PermissionModerateMembers | discordgo.PermissionManageChannels | discordgo.PermissionAddReactions | discordgo.PermissionManageServer | discordgo.PermissionManageEvents | discordgo.PermissionCreateEvents | discordgo.PermissionSendPolls | discordgo.PermissionVoiceMoveMembers | discordgo.PermissionVoiceConnect | discordgo.PermissionVoiceSpeak | discordgo.PermissionAttachFiles)
	out.Invite = fmt.Sprintf("https://discord.com/oauth2/authorize?client_id=%s&scope=bot%%20applications.commands&permissions=%d", s.State.User.ID, permissions)
	out.Checks = append(out.Checks, a.inspectCommunity(s)...)
	out.Checks = append(out.Checks, "Server connection and slash commands are ready")
	return out, nil
}
