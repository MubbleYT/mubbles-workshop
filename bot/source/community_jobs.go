package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type JoinRecord struct {
	UserID string
	At     time.Time
}

func (a *App) checkCommunityChannel(s *discordgo.Session, id string, types ...discordgo.ChannelType) (*discordgo.Channel, error) {
	if !validID(id) {
		return nil, errors.New("Choose a channel first")
	}
	ch, e := s.Channel(id)
	if e != nil {
		return nil, e
	}
	if ch.GuildID != a.config().GuildID {
		return nil, errors.New("Channel must belong to the configured server")
	}
	for _, t := range types {
		if ch.Type == t {
			return ch, nil
		}
	}
	return nil, errors.New("Channel has the wrong type")
}
func (a *App) postingChannel(s *discordgo.Session, id string) error {
	if id == a.config().TrapChannelID {
		return errors.New("Cannot publish a community message in the bot-trap channel")
	}
	if _, err := a.checkCommunityChannel(s, id, discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews); err != nil {
		return err
	}
	p, e := s.UserChannelPermissions(s.State.User.ID, id)
	if e != nil {
		return e
	}
	if !hasPermission(p, discordgo.PermissionViewChannel|discordgo.PermissionSendMessages|discordgo.PermissionEmbedLinks) {
		return errors.New("Bot needs View Channel, Send Messages and Embed Links in that channel")
	}
	return nil
}
func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}
func (a *App) botGuildPermission(s *discordgo.Session, w int64) error {
	g := a.config().GuildID
	roles, e := s.GuildRoles(g)
	if e != nil {
		return e
	}
	m, e := s.GuildMember(g, s.State.User.ID)
	if e != nil {
		return e
	}
	if !hasPermission(guildPermissions(m, g, roles), w) {
		return errors.New("The bot is missing a required server permission; use Check setup")
	}
	return nil
}
func (a *App) onJoin(s *discordgo.Session, m *discordgo.Member) {
	if m == nil || m.User == nil || m.User.Bot || m.GuildID != a.config().GuildID {
		return
	}
	c := a.config().Community
	now := time.Now()
	a.joinMu.Lock()
	recent := []JoinRecord{}
	for _, j := range a.joins {
		if now.Sub(j.At) < time.Duration(c.RaidWindow)*time.Second && j.UserID != m.User.ID {
			recent = append(recent, j)
		}
	}
	recent = append(recent, JoinRecord{m.User.ID, now})
	if len(recent) > 100 {
		recent = recent[len(recent)-100:]
	}
	a.joins = recent
	a.joinMu.Unlock()
	active := a.store.community(m.GuildID).RaidUntil.After(now)
	if c.RaidEnabled && len(recent) >= c.RaidJoins && !active {
		until := now.Add(time.Duration(c.RaidMinutes) * time.Minute)
		if e := a.store.updateCommunity(m.GuildID, func(v *CommunityState) error { v.RaidUntil = until; return nil }); e != nil {
			a.log("Could not activate raid mode: " + e.Error())
		} else {
			active = true
			a.audit(s, fmt.Sprintf("Raid mode activated: %d newcomers within %d seconds. Newcomers receive temporary timeouts; no automatic join bans.", len(recent), c.RaidWindow))
			for _, j := range recent {
				if j.UserID != m.User.ID {
					if member, e := s.GuildMember(m.GuildID, j.UserID); e == nil {
						a.restrictNewcomer(s, member)
					}
				}
			}
		}
	}
	if active {
		a.restrictNewcomer(s, m)
	}
	a.onScreened(s, m)
}
func (a *App) restrictNewcomer(s *discordgo.Session, m *discordgo.Member) {
	a.raidMu.Lock()
	defer a.raidMu.Unlock()
	if m == nil || m.User == nil || m.User.Bot || m.User.ID == s.State.User.ID {
		return
	}
	g := a.config().GuildID
	now := time.Now()
	v := a.store.community(g)
	if !v.RaidUntil.After(now) {
		return
	}
	if old, ok := v.RaidRestricted[m.User.ID]; ok && old.After(now) {
		return
	}
	until := now.Add(time.Duration(a.config().Community.RaidMinutes) * time.Minute)
	if m.CommunicationDisabledUntil != nil && m.CommunicationDisabledUntil.After(now) {
		return
	}
	if e := s.GuildMemberTimeout(g, m.User.ID, &until, discordgo.WithAuditLogReason("Mubble raid mode: temporary newcomer restriction")); e != nil {
		a.log("Newcomer restriction failed for " + m.User.ID + ": " + a.errorText(e))
		return
	}
	if e := a.store.updateCommunity(g, func(v *CommunityState) error { v.RaidRestricted[m.User.ID] = until; return nil }); e != nil {
		a.log("Could not save newcomer restriction: " + e.Error())
	}
	a.recordCase(s, "raid-timeout", s.State.User.ID, m.User.ID, "", "Temporary restriction during join spike", "", &until)
}
func (a *App) onScreened(s *discordgo.Session, m *discordgo.Member) {
	if m == nil || m.User == nil || m.User.Bot || m.Pending || m.GuildID != a.config().GuildID {
		return
	}
	a.memberMu.Lock()
	defer a.memberMu.Unlock()
	a.assignRole(s, m)
	a.welcomeMember(s, m)
}
func (a *App) welcomeMember(s *discordgo.Session, m *discordgo.Member) {
	c := a.config()
	cc := c.Community
	if cc.WelcomeChannel == "" {
		return
	}
	key := m.JoinedAt.UTC().Format(time.RFC3339Nano)
	v := a.store.community(c.GuildID)
	if v.Welcomed[m.User.ID] == key {
		return
	}
	if e := a.postingChannel(s, cc.WelcomeChannel); e != nil {
		a.log("Welcome message: " + a.errorText(e))
		return
	}
	server := "Mubble’s Workshop"
	if g, e := s.State.Guild(c.GuildID); e == nil {
		server = g.Name
	}
	channelText := func(id, fallback string) string {
		if id != "" {
			return "<#" + id + ">"
		}
		return fallback
	}
	text := strings.NewReplacer("{user}", "<@"+m.User.ID+">", "{server}", server, "{rules}", channelText(cc.RulesChannel, "the rules channel"), "{roles}", channelText(cc.RolesChannel, "the role menu"), "{showcase}", channelText(cc.ShowcaseChannel, "the showcase channel")).Replace(cc.WelcomeText)
	nonce := "welcome:" + m.User.ID + ":" + key
	if _, e := a.sendStable(s, cc.WelcomeChannel, nonce, &discordgo.MessageSend{Content: clip(text, 2000), AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}, Users: []string{m.User.ID}}}); e != nil {
		a.log("Welcome message: " + a.errorText(e))
		return
	}
	if e := a.store.updateCommunity(c.GuildID, func(v *CommunityState) error {
		if len(v.Welcomed) >= 10000 {
			for id := range v.Welcomed {
				delete(v.Welcomed, id)
				break
			}
		}
		v.Welcomed[m.User.ID] = key
		return nil
	}); e != nil {
		a.log("Welcome delivery record: " + e.Error())
	}
}
func (a *App) mentionProtection(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	c := a.config()
	if !c.Community.RaidEnabled && !a.store.community(c.GuildID).RaidUntil.After(time.Now()) {
		return false
	}
	ids := map[string]bool{}
	for _, u := range m.Mentions {
		ids["u:"+u.ID] = true
	}
	for _, r := range m.MentionRoles {
		ids["r:"+r] = true
	}
	if m.MentionEveryone {
		ids["everyone"] = true
	}
	if len(ids) <= c.Community.MentionLimit && !m.MentionEveryone {
		return false
	}
	p, e := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
	if e == nil && hasPermission(p, discordgo.PermissionManageMessages) {
		return false
	}
	if m.ID != "" {
		if e = s.ChannelMessageDelete(m.ChannelID, m.ID); e != nil {
			a.log("Mass-mention deletion failed: " + a.errorText(e))
		}
	}
	until := time.Now().Add(time.Duration(c.Community.RaidMinutes) * time.Minute)
	if member, err := s.GuildMember(c.GuildID, m.Author.ID); err == nil && member.CommunicationDisabledUntil != nil && member.CommunicationDisabledUntil.After(until) {
		return true
	}
	if e = s.GuildMemberTimeout(c.GuildID, m.Author.ID, &until, discordgo.WithAuditLogReason("Mass mention spam")); e != nil {
		a.log("Mass-mention timeout failed: " + a.errorText(e))
	} else {
		a.recordCase(s, "timeout", s.State.User.ID, m.Author.ID, m.ChannelID, "Mass mention spam", "", &until)
	}
	return true
}
func (a *App) endRaid(s *discordgo.Session, actor string) error {
	a.raidMu.Lock()
	defer a.raidMu.Unlock()
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	g := a.config().GuildID
	v := a.store.community(g)
	failed := map[string]time.Time{}
	for id, until := range v.RaidRestricted {
		m, e := s.GuildMember(g, id)
		if e != nil {
			var rest *discordgo.RESTError
			if errors.As(e, &rest) && rest.Response.StatusCode == 404 {
				continue
			}
			failed[id] = until
			continue
		}
		if m.CommunicationDisabledUntil != nil && m.CommunicationDisabledUntil.Sub(until).Abs() < 2*time.Second {
			if e = s.GuildMemberTimeout(g, id, nil, discordgo.WithAuditLogReason("Raid mode ended by "+actor)); e != nil {
				failed[id] = until
				continue
			}
			caseID := a.recordCase(s, "untimeout", actor, id, "", "Raid mode ended", "", nil)
			a.linkManualReversal(g, id, "timeout", "", actor, caseID)
		}
	}
	if e := a.store.updateCommunity(g, func(v *CommunityState) error { v.RaidUntil = time.Time{}; v.RaidRestricted = failed; return nil }); e != nil {
		return e
	}
	a.audit(s, "Raid mode ended by "+actor+"; only this raid mode's unchanged timeouts were removed.")
	if len(failed) > 0 {
		return fmt.Errorf("Raid mode ended, but %d newcomer restrictions could not be removed; they will expire normally. Check permissions or try End raid mode again", len(failed))
	}
	return nil
}
func voiceOccupants(s *discordgo.Session, g, ch string) int {
	s.State.RLock()
	defer s.State.RUnlock()
	for _, guild := range s.State.Guilds {
		if guild.ID == g {
			if guild.Unavailable || guild.Channels == nil {
				return -1
			}
			n := 0
			for _, v := range guild.VoiceStates {
				if v.ChannelID == ch {
					n++
				}
			}
			return n
		}
	}
	return -1
}
func (a *App) onVoiceState(s *discordgo.Session, e *discordgo.VoiceStateUpdate) {
	if e == nil || e.VoiceState == nil || e.GuildID != a.config().GuildID {
		return
	}
	cc := a.config().Community
	if cc.VoiceLobby == "" || e.ChannelID != cc.VoiceLobby {
		return
	}
	a.voiceMu.Lock()
	defer a.voiceMu.Unlock()
	m := e.Member
	if m == nil {
		m, _ = s.GuildMember(e.GuildID, e.UserID)
	}
	if m == nil || m.User == nil || m.User.Bot || m.Pending {
		return
	}
	if current, err := s.State.VoiceState(e.GuildID, e.UserID); err != nil || current.ChannelID != cc.VoiceLobby {
		return
	}
	v := a.store.community(e.GuildID)
	for _, r := range v.VoiceRooms {
		if r.Owner == e.UserID {
			if ch, err := s.Channel(r.ChannelID); err == nil && ch.GuildID == e.GuildID && ch.Type == discordgo.ChannelTypeGuildVoice {
				s.GuildMemberMove(e.GuildID, e.UserID, &ch.ID)
				return
			}
		}
	}
	if len(v.VoiceRooms) >= 50 {
		a.log("Temporary voice room limit reached (50).")
		return
	}
	parent, err := a.checkCommunityChannel(s, cc.VoiceCategory, discordgo.ChannelTypeGuildCategory)
	if err != nil {
		a.log("Voice rooms: " + a.errorText(err))
		return
	}
	overwrites := append([]*discordgo.PermissionOverwrite{}, parent.PermissionOverwrites...)
	filtered := []*discordgo.PermissionOverwrite{}
	for _, o := range overwrites {
		if o.ID != e.UserID && o.ID != s.State.User.ID {
			filtered = append(filtered, o)
		}
	}
	filtered = append(filtered, &discordgo.PermissionOverwrite{ID: e.UserID, Type: discordgo.PermissionOverwriteTypeMember, Allow: discordgo.PermissionViewChannel | discordgo.PermissionVoiceConnect | discordgo.PermissionVoiceSpeak}, &discordgo.PermissionOverwrite{ID: s.State.User.ID, Type: discordgo.PermissionOverwriteTypeMember, Allow: discordgo.PermissionViewChannel | discordgo.PermissionManageChannels | discordgo.PermissionVoiceConnect | discordgo.PermissionVoiceMoveMembers})
	name := clip(m.User.Username+"’s room", 90)
	ch, err := s.GuildChannelCreateComplex(e.GuildID, discordgo.GuildChannelCreateData{Name: name, Type: discordgo.ChannelTypeGuildVoice, ParentID: cc.VoiceCategory, UserLimit: cc.VoiceLimit, PermissionOverwrites: filtered})
	if err != nil {
		a.log("Create voice room: " + a.errorText(err))
		return
	}
	room := VoiceRoom{ch.ID, e.UserID, time.Now().UTC()}
	if err = a.store.updateCommunity(e.GuildID, func(v *CommunityState) error { v.VoiceRooms[ch.ID] = room; return nil }); err != nil {
		s.ChannelDelete(ch.ID)
		a.log("Save voice room: " + err.Error())
		return
	}
	if err = s.GuildMemberMove(e.GuildID, e.UserID, &ch.ID); err != nil {
		a.log("Move member into voice room: " + a.errorText(err))
	}
}
func (a *App) cleanVoiceRooms(s *discordgo.Session) {
	a.voiceMu.Lock()
	defer a.voiceMu.Unlock()
	g := a.config().GuildID
	for id, r := range a.store.community(g).VoiceRooms {
		if time.Since(r.Created) < 30*time.Second || voiceOccupants(s, g, id) != 0 {
			continue
		}
		ch, e := s.Channel(id)
		if e != nil {
			var rest *discordgo.RESTError
			if !errors.As(e, &rest) || rest.Message == nil || rest.Message.Code != 10003 {
				continue
			}
		} else {
			if ch.GuildID != g || ch.Type != discordgo.ChannelTypeGuildVoice {
				continue
			}
			if _, e = s.ChannelDelete(id); e != nil {
				a.log("Delete empty room: " + a.errorText(e))
				continue
			}
		}
		a.store.updateCommunity(g, func(v *CommunityState) error { delete(v.VoiceRooms, id); return nil })
	}
}
func (a *App) communityLoop(ctx context.Context, s *discordgo.Session) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if current, e := a.current(); e == nil && current == s {
			a.processEventReminders(ctx, s)
			a.cleanVoiceRooms(s)
			g := a.config().GuildID
			if e := a.store.updateCommunityIfNeeded(g); e != nil {
				a.log("Community housekeeping: " + e.Error())
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *Store) updateCommunityIfNeeded(g string) error {
	v := s.community(g)
	now := time.Now()
	needed := false
	for _, until := range v.RaidRestricted {
		if until.Before(now) {
			needed = true
			break
		}
	}
	if !needed {
		return nil
	}
	return s.updateCommunity(g, func(v *CommunityState) error {
		for id, until := range v.RaidRestricted {
			if until.Before(now) {
				delete(v.RaidRestricted, id)
			}
		}
		return nil
	})
}
func (a *App) processEventReminders(ctx context.Context, s *discordgo.Session) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	g := a.config().GuildID
	for id, e := range a.store.community(g).Events {
		if ctx.Err() != nil {
			return
		}
		if e.Reminded || e.Cancelled || e.ReminderMinutes == 0 || time.Now().Before(e.Starts.Add(-time.Duration(e.ReminderMinutes)*time.Minute)) {
			continue
		}
		if time.Now().After(e.Starts) {
			a.store.updateCommunity(g, func(v *CommunityState) error { x := v.Events[id]; x.Reminded = true; v.Events[id] = x; return nil })
			continue
		}
		remote, err := s.GuildScheduledEvent(g, id, false, discordgo.WithContext(ctx))
		if err != nil {
			var rest *discordgo.RESTError
			if errors.As(err, &rest) && rest.Response.StatusCode == 404 {
				a.store.updateCommunity(g, func(v *CommunityState) error { x := v.Events[id]; x.Cancelled = true; v.Events[id] = x; return nil })
				continue
			}
			a.log("Event reminder: " + a.errorText(err))
			continue
		}
		if remote.Status != discordgo.GuildScheduledEventStatusScheduled {
			a.store.updateCommunity(g, func(v *CommunityState) error { x := v.Events[id]; x.Cancelled = true; v.Events[id] = x; return nil })
			continue
		}
		if !remote.ScheduledStartTime.Equal(e.Starts) {
			e.Starts = remote.ScheduledStartTime
			a.store.updateCommunity(g, func(v *CommunityState) error { x := v.Events[id]; x.Starts = e.Starts; v.Events[id] = x; return nil })
			continue
		}
		if err = a.postingChannel(s, e.ChannelID); err != nil {
			a.log("Event reminder: " + a.errorText(err))
			continue
		}
		content := fmt.Sprintf("**%s** starts <t:%d:R>.\nhttps://discord.com/events/%s/%s", clip(remote.Name, 100), e.Starts.Unix(), g, id)
		mentions := noMentions()
		if e.RoleID != "" {
			content = "<@&" + e.RoleID + "> " + content
			mentions.Roles = []string{e.RoleID}
		}
		_, err = a.sendStable(s, e.ChannelID, "event-reminder:"+id+":"+e.Starts.Format(time.RFC3339), &discordgo.MessageSend{Content: content, AllowedMentions: mentions})
		if err != nil {
			a.log("Event reminder: " + a.errorText(err))
			continue
		}
		if err = a.store.updateCommunity(g, func(v *CommunityState) error { x := v.Events[id]; x.Reminded = true; v.Events[id] = x; return nil }); err != nil {
			a.log("Reminder sent, delivery record failed: " + err.Error())
		}
	}
}
func (a *App) syncNativeAutoMod(s *discordgo.Session) error {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	c := a.config()
	if err := a.botGuildPermission(s, discordgo.PermissionManageServer); err != nil {
		return err
	}
	all, err := s.AutoModerationRules(c.GuildID)
	if err != nil {
		return err
	}
	v := a.store.community(c.GuildID)
	keywords := []string{}
	for _, w := range strings.FieldsFunc(c.SlurTerms, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' }) {
		w = strings.TrimSpace(w)
		if w != "" {
			if len([]rune(w)) > 60 {
				return errors.New("Discord AutoMod supports terms up to 60 characters; shorten longer slur-list entries first")
			}
			keywords = append(keywords, w)
		}
	}
	for _, kind := range []string{"slurs", "mentions"} {
		enabled := c.Community.NativeAutoMod
		if kind == "slurs" {
			enabled = enabled && c.BanSlurs && len(keywords) > 0
		}
		id := v.NativeRules[kind]
		var existing *discordgo.AutoModerationRule
		for _, r := range all {
			if r.ID == id {
				existing = r
				break
			}
		}
		if existing != nil && existing.CreatorID != s.State.User.ID {
			return errors.New("Refusing to edit an AutoMod rule created by another account")
		}
		if !enabled && existing == nil {
			continue
		}
		actions := []discordgo.AutoModerationAction{{Type: discordgo.AutoModerationRuleActionBlockMessage, Metadata: &discordgo.AutoModerationActionMetadata{CustomMessage: "This message breaks the workshop rules."}}}
		rule := &discordgo.AutoModerationRule{Name: "Mubble Workshop — " + kind, EventType: discordgo.AutoModerationEventMessageSend, Enabled: &enabled, Actions: actions, ExemptRoles: &[]string{}, ExemptChannels: &[]string{}}
		if kind == "slurs" {
			rule.TriggerType = discordgo.AutoModerationEventTriggerKeyword
			rule.TriggerMetadata = &discordgo.AutoModerationTriggerMetadata{KeywordFilter: keywords}
			if len(keywords) == 0 && existing != nil {
				rule.TriggerMetadata = existing.TriggerMetadata
			}
		} else {
			rule.TriggerType = discordgo.AutoModerationRuleTriggerType(5)
			rule.TriggerMetadata = &discordgo.AutoModerationTriggerMetadata{MentionTotalLimit: c.Community.MentionLimit}
		}
		if existing != nil {
			rule, err = s.AutoModerationRuleEdit(c.GuildID, existing.ID, rule)
		} else {
			rule, err = s.AutoModerationRuleCreate(c.GuildID, rule)
		}
		if err != nil {
			return fmt.Errorf("AutoMod %s: %w. Existing unrelated rules were left alone", kind, err)
		}
		if err = a.store.updateCommunity(c.GuildID, func(v *CommunityState) error { v.NativeRules[kind] = rule.ID; return nil }); err != nil {
			if existing == nil {
				s.AutoModerationRuleDelete(c.GuildID, rule.ID)
			}
			return err
		}
	}
	a.audit(s, "Discord AutoMod rules synchronized. Native blocks remain active while this app is offline; automatic bans require the bot online.")
	return nil
}
func (a *App) onNativeAutoMod(s *discordgo.Session, e *discordgo.AutoModerationActionExecution) {
	c := a.config()
	if e.GuildID != c.GuildID || e.Action.Type != discordgo.AutoModerationRuleActionBlockMessage || e.UserID == s.State.User.ID {
		return
	}
	v := a.store.community(c.GuildID)
	if e.RuleID == v.NativeRules["slurs"] && c.BanSlurs {
		m := &discordgo.MessageCreate{Message: &discordgo.Message{ID: e.MessageID, ChannelID: e.ChannelID, GuildID: e.GuildID, Content: e.Content, Author: &discordgo.User{ID: e.UserID}}}
		a.banForPolicy(s, m, "listed slur blocked by Discord AutoMod")
	}
}

// Stable nonces protect retries against duplicate announcements within Discord's nonce window.
func (a *App) sendStable(s *discordgo.Session, ch, key string, p *discordgo.MessageSend) (*discordgo.Message, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err = json.Unmarshal(b, &payload); err != nil {
		return nil, err
	}
	payload["nonce"] = stableNonce(key)
	payload["enforce_nonce"] = true
	b, err = s.RequestWithBucketID("POST", discordgo.EndpointChannelMessages(ch), payload, discordgo.EndpointChannelMessages(ch))
	if err != nil {
		return nil, err
	}
	var out discordgo.Message
	err = json.Unmarshal(b, &out)
	return &out, err
}

func (a *App) onCommunityEventUpdate(s *discordgo.Session, e *discordgo.GuildScheduledEventUpdate) {
	if e == nil || e.GuildScheduledEvent == nil || e.GuildID != a.config().GuildID {
		return
	}
	g := e.GuildID
	if _, ok := a.store.community(g).Events[e.ID]; !ok {
		return
	}
	if err := a.store.updateCommunity(g, func(v *CommunityState) error {
		x := v.Events[e.ID]
		if !x.Starts.Equal(e.ScheduledStartTime) && e.ScheduledStartTime.After(time.Now()) {
			x.Reminded = false
		}
		x.Name = e.Name
		x.Starts = e.ScheduledStartTime
		if e.ScheduledEndTime != nil {
			x.Ends = *e.ScheduledEndTime
		}
		x.Cancelled = e.Status == discordgo.GuildScheduledEventStatusCanceled || e.Status == discordgo.GuildScheduledEventStatusCompleted
		v.Events[e.ID] = x
		return nil
	}); err != nil {
		a.log("Event update: " + err.Error())
	}
}
func (a *App) onCommunityEventDelete(s *discordgo.Session, e *discordgo.GuildScheduledEventDelete) {
	if e == nil || e.GuildScheduledEvent == nil || e.GuildID != a.config().GuildID {
		return
	}
	if _, ok := a.store.community(e.GuildID).Events[e.ID]; !ok {
		return
	}
	if err := a.store.updateCommunity(e.GuildID, func(v *CommunityState) error { x := v.Events[e.ID]; x.Cancelled = true; v.Events[e.ID] = x; return nil }); err != nil {
		a.log("Event deletion: " + err.Error())
	}
}
