package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

const communityStaff = "723456789012345678"
const communityCategory = "823456789012345678"
const communityRoom = "923456789012345678"
const communityLobby = "113456789012345678"
const communityAdmin = "133456789012345678"
const communityInterest = "143456789012345678"
const communityMod = "153456789012345678"

type communityCall struct {
	Method, Path string
	Body         map[string]any
}
type communityTransport struct {
	mu       sync.Mutex
	calls    []communityCall
	roles    []*discordgo.Role
	members  map[string]*discordgo.Member
	channels map[string]*discordgo.Channel
	native   []*discordgo.AutoModerationRule
	event    *discordgo.GuildScheduledEvent
	failRole string
}

func (tr *communityTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	var body map[string]any
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
	}
	tr.calls = append(tr.calls, communityCall{r.Method, r.URL.Path, body})
	var result any = map[string]any{}
	code := 200
	path := strings.TrimPrefix(r.URL.Path, "/api/v9")
	switch {
	case strings.HasSuffix(path, "/roles") && r.Method == "GET":
		result = tr.roles
	case strings.Contains(path, "/members/"):
		tail := strings.Split(path, "/members/")[1]
		parts := strings.Split(tail, "/")
		member := tr.members[parts[0]]
		if member == nil {
			member = &discordgo.Member{GuildID: embedGuild, User: &discordgo.User{ID: parts[0]}, Roles: []string{}}
			tr.members[parts[0]] = member
		}
		if len(parts) == 1 && r.Method == "GET" {
			result = member
		}
		if len(parts) > 1 && parts[1] == "roles" {
			if parts[2] == tr.failRole && r.Method == "PUT" {
				code = 403
				result = map[string]any{"code": 50013, "message": "Missing Permissions"}
			} else if r.Method == "PUT" {
				member.Roles = append(member.Roles, parts[2])
			} else if r.Method == "DELETE" {
				out := []string{}
				for _, id := range member.Roles {
					if id != parts[2] {
						out = append(out, id)
					}
				}
				member.Roles = out
			}
		}
		if r.Method == "PATCH" {
			if v, ok := body["communication_disabled_until"].(string); ok {
				until, _ := time.Parse(time.RFC3339Nano, v)
				member.CommunicationDisabledUntil = &until
			} else {
				member.CommunicationDisabledUntil = nil
			}
		}
	case strings.HasSuffix(path, "/channels") && r.Method == "GET":
		list := []*discordgo.Channel{}
		for _, ch := range tr.channels {
			list = append(list, ch)
		}
		result = list
	case strings.HasSuffix(path, "/channels") && r.Method == "POST":
		typ := discordgo.ChannelType(int(body["type"].(float64)))
		ch := &discordgo.Channel{ID: communityRoom, GuildID: embedGuild, Type: typ, Name: body["name"].(string)}
		tr.channels[ch.ID] = ch
		result = ch
	case strings.Contains(path, "/auto-moderation/rules"):
		if r.Method == "GET" {
			result = tr.native
		} else {
			b, _ := json.Marshal(body)
			var rule discordgo.AutoModerationRule
			json.Unmarshal(b, &rule)
			rule.CreatorID = embedBot
			rule.ID = "163456789012345678"
			if rule.TriggerType == 5 {
				rule.ID = "173456789012345678"
			}
			tr.native = append(tr.native, &rule)
			result = rule
		}
	case strings.Contains(path, "/scheduled-events"):
		if r.Method == "POST" {
			b, _ := json.Marshal(body)
			var params discordgo.GuildScheduledEventParams
			json.Unmarshal(b, &params)
			tr.event = &discordgo.GuildScheduledEvent{ID: "183456789012345678", GuildID: embedGuild, Name: params.Name, ScheduledStartTime: *params.ScheduledStartTime, Status: discordgo.GuildScheduledEventStatusScheduled}
			result = tr.event
		} else if r.Method == "GET" {
			result = tr.event
		}
	case strings.HasSuffix(path, "/messages") && r.Method == "POST":
		result = &discordgo.Message{ID: embedMessage, ChannelID: strings.Split(strings.TrimPrefix(path, "/channels/"), "/")[0]}
	case strings.Contains(path, "/messages/") && r.Method == "PATCH":
		result = &discordgo.Message{ID: embedMessage, ChannelID: embedChannel}
	case strings.HasPrefix(path, "/channels/") && !strings.Contains(path, "/messages"):
		id := strings.Split(strings.TrimPrefix(path, "/channels/"), "/")[0]
		if ch := tr.channels[id]; ch != nil {
			result = ch
			if r.Method == "DELETE" {
				delete(tr.channels, id)
			}
		} else {
			code = 404
			result = map[string]any{"code": 10003, "message": "Unknown Channel"}
		}
	case path == "/guilds/"+embedGuild:
		result = &discordgo.Guild{ID: embedGuild, OwnerID: "193456789012345678", Name: "Workshop"}
	}
	b, _ := json.Marshal(result)
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b))), Request: r}, nil
}
func communityFixture(t *testing.T) (*App, *discordgo.Session, *communityTransport) {
	t.Helper()
	s, _ := discordgo.New("Bot fake")
	s.State.User = &discordgo.User{ID: embedBot, Bot: true}
	roles := []*discordgo.Role{{ID: embedGuild, Position: 0, Permissions: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionReadMessageHistory | discordgo.PermissionVoiceConnect}, {ID: "botrole", Position: 10, Permissions: discordgo.PermissionAdministrator}, {ID: embedRole, Position: 1, Permissions: discordgo.PermissionViewChannel}, {ID: communityInterest, Position: 2, Permissions: discordgo.PermissionVoiceConnect}, {ID: communityStaff, Position: 8, Permissions: discordgo.PermissionManageMessages | discordgo.PermissionModerateMembers | discordgo.PermissionManageChannels}, {ID: communityAdmin, Position: 9, Permissions: discordgo.PermissionAdministrator}}
	members := map[string]*discordgo.Member{embedBot: {GuildID: embedGuild, User: s.State.User, Roles: []string{"botrole"}}, embedUser: {GuildID: embedGuild, User: &discordgo.User{ID: embedUser}, Roles: []string{}}, communityMod: {GuildID: embedGuild, User: &discordgo.User{ID: communityMod}, Roles: []string{communityStaff}}}
	channels := map[string]*discordgo.Channel{embedChannel: {ID: embedChannel, GuildID: embedGuild, Type: discordgo.ChannelTypeGuildText}, communityCategory: {ID: communityCategory, GuildID: embedGuild, Type: discordgo.ChannelTypeGuildCategory}, communityLobby: {ID: communityLobby, GuildID: embedGuild, Type: discordgo.ChannelTypeGuildVoice}}
	tr := &communityTransport{roles: roles, members: members, channels: channels}
	s.Client = &http.Client{Transport: tr}
	list := []*discordgo.Channel{}
	for _, ch := range channels {
		list = append(list, ch)
	}
	s.State.GuildAdd(&discordgo.Guild{ID: embedGuild, OwnerID: "193456789012345678", Name: "Workshop", Roles: roles, Channels: list, Members: []*discordgo.Member{members[embedBot], members[embedUser], members[communityMod]}})
	store, e := openStore(filepath.Join(t.TempDir(), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	c := defaultConfig()
	c.Token = "kept-secret-token"
	c.GuildID = embedGuild
	return &App{cfg: c, store: store, dir: filepath.Dir(store.path), session: s, ready: true, spam: map[string][]time.Time{}, cooldown: map[string]time.Time{}}, s, tr
}
func componentInteraction(user, kind, id, message string, values []string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{ID: "interaction", AppID: embedBot, Token: "fake", Type: discordgo.InteractionMessageComponent, GuildID: embedGuild, ChannelID: embedChannel, Message: &discordgo.Message{ID: message}, Member: &discordgo.Member{User: &discordgo.User{ID: user}}, Data: discordgo.MessageComponentInteractionData{CustomID: "mw:" + kind + ":" + id, Values: values}}}
}
func TestPublicRolesRejectStaffAndRollbackPartialChanges(t *testing.T) {
	a, s, tr := communityFixture(t)
	if e := a.validatePublicRole(s, embedGuild, communityAdmin); e == nil {
		t.Fatal("Admin role accepted")
	}
	a.cfg.Community.SelfRoles = []SelfRole{{"Member interest", embedRole, ""}, {"Minecraft", communityInterest, ""}}
	panel := CommunityPanel{Roles: a.cfg.Community.SelfRoles}
	tr.failRole = communityInterest
	if _, e := a.applySelectedRoles(s, tr.members[embedUser], panel, []string{embedRole, communityInterest}); e == nil {
		t.Fatal("Expected role update failure")
	}
	if len(tr.members[embedUser].Roles) != 0 {
		t.Fatal("Partial changes not rolled back")
	}
	tr.failRole = ""
	if _, e := a.applySelectedRoles(s, tr.members[embedUser], panel, []string{communityAdmin}); e == nil {
		t.Fatal("Forged role accepted")
	}
	if _, e := a.applySelectedRoles(s, tr.members[embedUser], panel, []string{embedRole}); e != nil {
		t.Fatal(e)
	}
	if _, e := a.applySelectedRoles(s, tr.members[embedUser], panel, nil); e != nil || len(tr.members[embedUser].Roles) != 0 {
		t.Fatal("Opt-out failed")
	}
	tr.members[embedUser].Pending = true
	if _, e := a.applySelectedRoles(s, tr.members[embedUser], panel, []string{embedRole}); e == nil {
		t.Fatal("Screening bypass")
	}
}
func TestPanelBindingsRejectCopiedMessage(t *testing.T) {
	a, s, tr := communityFixture(t)
	a.cfg.Community.SelfRoles = []SelfRole{{"Minecraft", embedRole, ""}}
	a.cfg.Community.RolesChannel = embedChannel
	p, e := a.publishCommunityPanel(s, "roles")
	if e != nil {
		t.Fatal(e)
	}
	before := len(tr.calls)
	i := componentInteraction(embedUser, "roles", p.ID, "forged", []string{embedRole})
	a.handleCommunityInteraction(s, i)
	for _, call := range tr.calls[before:] {
		if call.Method == "PUT" {
			t.Fatal("Copied message granted a role")
		}
	}
	saved, e := openStore(a.store.path)
	if e != nil || saved.community(embedGuild).Panels[p.ID].MessageID != embedMessage {
		t.Fatal("Panel binding lost after restart")
	}
}
func TestTicketsArePrivateAndCloseRetainsHistory(t *testing.T) {
	a, s, tr := communityFixture(t)
	a.cfg.Community.TicketStaffRoles = []string{communityStaff}
	a.cfg.Community.TicketCategory = communityCategory
	text, e := a.createTicket(s, tr.members[embedUser], "Help", "Private details")
	if e != nil || !strings.Contains(text, communityRoom) {
		t.Fatal(text, e)
	}
	var create communityCall
	for _, call := range tr.calls {
		if call.Method == "POST" && strings.HasSuffix(call.Path, "/channels") {
			create = call
		}
	}
	overwrites := create.Body["permission_overwrites"].([]any)
	foundDeny := false
	for _, raw := range overwrites {
		o := raw.(map[string]any)
		if o["id"] == embedGuild {
			foundDeny = o["deny"] == fmt.Sprint(discordgo.PermissionViewChannel)
		}
	}
	if !foundDeny {
		t.Fatal("Private ticket did not deny @everyone")
	}
	v := a.store.community(embedGuild)
	var ticket Ticket
	for _, x := range v.Tickets {
		ticket = x
	}
	if _, e = a.setTicketClosed(s, &discordgo.Member{User: &discordgo.User{ID: communityInterest}}, ticket, true); e == nil {
		t.Fatal("Unrelated member closed ticket")
	}
	if _, e = a.setTicketClosed(s, tr.members[embedUser], ticket, true); e != nil {
		t.Fatal(e)
	}
	for _, call := range tr.calls {
		if call.Method == "DELETE" && strings.HasSuffix(call.Path, "/channels/"+communityRoom) {
			t.Fatal("Closing destroyed history")
		}
	}
	state, e := openStore(a.store.path)
	if e != nil || !state.community(embedGuild).Tickets[ticket.ID].Closed {
		t.Fatal("Close state did not persist")
	}
	if _, e = a.setTicketClosed(s, tr.members[embedUser], ticket, false); e == nil {
		t.Fatal("Owner reopened staff-only closed ticket")
	}
	mod := *tr.members[communityMod]
	mod.Permissions = discordgo.PermissionManageChannels
	if _, e = a.setTicketClosed(s, &mod, ticket, false); e != nil {
		t.Fatal(e)
	}
}
func TestSubmissionVotesUniqueStatusStaffOnly(t *testing.T) {
	a, s, tr := communityFixture(t)
	a.cfg.Community.SuggestChannel = embedChannel
	_, e := a.createSubmission(s, tr.members[embedUser], "suggest", map[string]string{"title": "A video idea", "body": "Explore Minecraft history"})
	if e != nil {
		t.Fatal(e)
	}
	var x Submission
	for _, r := range a.store.community(embedGuild).Submissions {
		x = r
	}
	i := componentInteraction(embedUser, "up", x.ID, embedMessage, nil)
	for n := 0; n < 2; n++ {
		if _, e = a.updateSubmission(s, i, "up", x.ID); e != nil {
			t.Fatal(e)
		}
	}
	if len(a.store.community(embedGuild).Submissions[x.ID].Votes) != 1 {
		t.Fatal("Repeated click duplicated votes")
	}
	i.Data = discordgo.MessageComponentInteractionData{CustomID: "mw:status:" + x.ID, Values: []string{"Fixed"}}
	if _, e = a.updateSubmission(s, i, "status", x.ID); e == nil {
		t.Fatal("Non-staff changed status")
	}
	i.Member.Permissions = discordgo.PermissionManageMessages
	if _, e = a.updateSubmission(s, i, "status", x.ID); e != nil {
		t.Fatal(e)
	}
	if a.store.community(embedGuild).Submissions[x.ID].Status != "Fixed" {
		t.Fatal("Status not saved")
	}
	i.Message.ID = "wrong"
	if _, e = a.updateSubmission(s, i, "up", x.ID); e == nil {
		t.Fatal("Wrong message accepted")
	}
}
func TestRaidRestrictsJoinSpikeWithoutBans(t *testing.T) {
	a, s, tr := communityFixture(t)
	a.cfg.Community.RaidEnabled = true
	a.cfg.Community.RaidJoins = 3
	for j := 0; j < 3; j++ {
		m := &discordgo.Member{GuildID: embedGuild, User: &discordgo.User{ID: fmt.Sprintf("%d", 203456789012345678+j)}}
		a.onJoin(s, m)
	}
	v := a.store.community(embedGuild)
	if !v.RaidUntil.After(time.Now()) || len(v.RaidRestricted) != 3 {
		t.Fatal("Join spike did not restrict newcomers", v)
	}
	for _, call := range tr.calls {
		if strings.Contains(call.Path, "/bans/") {
			t.Fatal("Joining caused a ban")
		}
	}
	if e := a.endRaid(s, s.State.User.ID); e != nil {
		t.Fatal(e)
	}
	if a.store.community(embedGuild).RaidUntil.After(time.Now()) {
		t.Fatal("Raid mode stayed active")
	}
	for id := range v.RaidRestricted {
		if tr.members[id].CommunicationDisabledUntil != nil {
			t.Fatal("Matching raid timeout not cleared")
		}
	}
}
func TestRaidDoesNotShortenExistingTimeout(t *testing.T) {
	a, s, tr := communityFixture(t)
	until := time.Now().Add(time.Hour)
	m := tr.members[embedUser]
	m.CommunicationDisabledUntil = &until
	a.restrictNewcomer(s, m)
	if len(tr.calls) != 0 || !m.CommunicationDisabledUntil.Equal(until) {
		t.Fatal("Existing timeout overwritten")
	}
}
func TestWelcomeDeduplicatesAndOnlyPingsNewMember(t *testing.T) {
	a, s, tr := communityFixture(t)
	a.cfg.Community.WelcomeChannel = embedChannel
	a.cfg.Community.RulesChannel = embedChannel
	m := tr.members[embedUser]
	m.JoinedAt = time.Now()
	a.onScreened(s, m)
	a.onScreened(s, m)
	posts := 0
	for _, call := range tr.calls {
		if call.Method == "POST" && strings.HasSuffix(call.Path, "/messages") {
			posts++
			mentions := call.Body["allowed_mentions"].(map[string]any)
			if len(mentions["users"].([]any)) != 1 || len(mentions["parse"].([]any)) != 0 {
				t.Fatal("Unsafe welcome mentions")
			}
			if strings.Contains(call.Body["content"].(string), "{user}") {
				t.Fatal("Placeholder not substituted")
			}
		}
	}
	if posts != 1 {
		t.Fatal("Duplicate welcome", posts)
	}
}
func TestNativeAutoModIsOwnedAndBlockedSlursStillBan(t *testing.T) {
	a, s, tr := communityFixture(t)
	a.cfg.Community.NativeAutoMod = true
	if e := a.syncNativeAutoMod(s); e != nil {
		t.Fatal(e)
	}
	v := a.store.community(embedGuild)
	if v.NativeRules["slurs"] == "" || v.NativeRules["mentions"] == "" {
		t.Fatal("Missing native rule bindings")
	}
	before := len(tr.calls)
	a.onNativeAutoMod(s, &discordgo.AutoModerationActionExecution{GuildID: embedGuild, RuleID: v.NativeRules["slurs"], UserID: embedUser, ChannelID: embedChannel, Action: discordgo.AutoModerationAction{Type: discordgo.AutoModerationRuleActionBlockMessage}})
	banned := false
	for _, call := range tr.calls[before:] {
		if call.Method == "PUT" && strings.Contains(call.Path, "/bans/") {
			banned = true
		}
		if call.Method == "DELETE" && strings.Contains(call.Path, "/messages/") {
			t.Fatal("Blocked message has no ID to delete")
		}
	}
	if !banned {
		t.Fatal("Native block disabled ban policy")
	}
	for _, r := range tr.native {
		r.CreatorID = "other"
	}
	if e := a.syncNativeAutoMod(s); e == nil {
		t.Fatal("Edited another account's native rule")
	}
}
func TestPollAndEventValidation(t *testing.T) {
	a, s, _ := communityFixture(t)
	if _, e := pollPayload(PollRequest{Question: "Choose", Answers: []string{"One", "One"}, Hours: 24}); e == nil {
		t.Fatal("Duplicate answers accepted")
	}
	if _, e := a.publishPoll(s, PollRequest{ChannelID: embedChannel, Question: "Next video?", Answers: []string{"Mods", "Survival"}, Hours: 24}); e != nil {
		t.Fatal(e)
	}
	r := EventRequest{Name: "Minecraft night", Start: time.Now().Add(time.Hour).Format(time.RFC3339), End: time.Now().Add(2 * time.Hour).Format(time.RFC3339), Location: "Minecraft server", ChannelID: embedChannel, ReminderMinutes: 30}
	e, err := a.createCommunityEvent(s, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.createCommunityEvent(s, r); err == nil {
		t.Fatal("Duplicate event created")
	}
	if err = a.cancelCommunityEvent(s, e.ID); err != nil {
		t.Fatal(err)
	}
	if !a.store.community(embedGuild).Events[e.ID].Cancelled {
		t.Fatal("Cancel not persisted")
	}
	r.Start = "2026-10-04T18:00:00"
	if _, _, err = validateEvent(r); err == nil {
		t.Fatal("Ambiguous timezone accepted")
	}
}
func TestReminderDoesNotSendAfterStartAndPersistsDelivery(t *testing.T) {
	a, s, tr := communityFixture(t)
	start := time.Now().Add(5 * time.Minute)
	id := "183456789012345678"
	tr.event = &discordgo.GuildScheduledEvent{ID: id, Name: "Game night", Status: discordgo.GuildScheduledEventStatusScheduled, ScheduledStartTime: start}
	a.store.updateCommunity(embedGuild, func(v *CommunityState) error {
		v.Events[id] = CommunityEvent{ID: id, Name: "Game night", Starts: start, Ends: start.Add(time.Hour), ChannelID: embedChannel, ReminderMinutes: 10}
		return nil
	})
	a.processEventReminders(context.Background(), s)
	a.processEventReminders(context.Background(), s)
	posts := 0
	for _, call := range tr.calls {
		if call.Method == "POST" && strings.HasSuffix(call.Path, "/messages") {
			posts++
		}
	}
	if posts != 1 || !a.store.community(embedGuild).Events[id].Reminded {
		t.Fatal("Reminder duplicate or unsaved")
	}
	a.store.updateCommunity(embedGuild, func(v *CommunityState) error {
		x := v.Events[id]
		x.Starts = time.Now().Add(-time.Minute)
		x.Reminded = false
		v.Events[id] = x
		return nil
	})
	before := len(tr.calls)
	a.processEventReminders(context.Background(), s)
	if len(tr.calls) != before {
		t.Fatal("Late reminder sent")
	}
}
func TestCaseReversalRecordsActorAndCannotRunTwice(t *testing.T) {
	a, s, tr := communityFixture(t)
	until := time.Now().Add(time.Hour)
	tr.members[embedUser].CommunicationDisabledUntil = &until
	id := a.recordCase(s, "timeout", communityMod, embedUser, embedChannel, "Spam", "", &until)
	x, _ := findCase(a.store.community(embedGuild), id)
	i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{GuildID: embedGuild, Member: &discordgo.Member{User: &discordgo.User{ID: communityMod}, Roles: []string{communityStaff}, Permissions: discordgo.PermissionModerateMembers}}}
	if _, e := a.reverseCase(s, i, x, "Resolved"); e != nil {
		t.Fatal(e)
	}
	updated, _ := findCase(a.store.community(embedGuild), id)
	if updated.ReversedBy != communityMod || updated.ReversalCase == "" {
		t.Fatal("Missing reversal trail")
	}
	if _, e := a.reverseCase(s, i, x, "Again"); e == nil {
		t.Fatal("Repeated reversal accepted")
	}
}
func TestBackupExcludesTokenPreservesBindingsAndRejectsOtherGuild(t *testing.T) {
	a, _, _ := communityFixture(t)
	a.session = nil
	a.cfg.Community.FAQ = []FAQEntry{{"mods", "https://example.com/mods"}}
	a.store.updateCommunity(embedGuild, func(v *CommunityState) error { v.NativeRules["slurs"] = "163456789012345678"; return nil })
	b := a.exportBackup()
	raw, _ := json.Marshal(b)
	if strings.Contains(string(raw), a.cfg.Token) {
		t.Fatal("Backup leaked token")
	}
	b.Config.BotName = "Restored name"
	if e := a.restoreBackup(b); e != nil {
		t.Fatal(e)
	}
	if a.config().Token != "kept-secret-token" || a.config().BotName != "Restored name" || a.store.community(embedGuild).NativeRules["slurs"] == "" {
		t.Fatal("Restore changed token or lost state")
	}
	b.Config.GuildID = communityRoom
	if e := a.restoreBackup(b); e == nil {
		t.Fatal("Cross-guild backup accepted")
	}
	a.session = &discordgo.Session{}
	b.Config.GuildID = embedGuild
	if e := a.restoreBackup(b); e == nil {
		t.Fatal("Live restore accepted")
	}
}
func TestCommunityAPIRequiresKeyAndStoppedSettings(t *testing.T) {
	a, _, _ := communityFixture(t)
	a.csrf = "panel-key"
	a.host = "127.0.0.1:9999"
	handler := a.routes()
	req := httptest.NewRequest("GET", "http://"+a.host+"/api/community", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("Community API missing key gate")
	}
	req.Header.Set("X-Panel-Key", a.csrf)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), a.cfg.Token) {
		t.Fatal("Community status leaked token or failed")
	}
	data, _ := json.Marshal(a.cfg.Community)
	req = httptest.NewRequest("POST", "http://"+a.host+"/api/community-save", strings.NewReader(string(data)))
	req.Header.Set("X-Panel-Key", a.csrf)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal("Settings changed while running")
	}
	req = httptest.NewRequest("GET", "http://"+a.host+"/api/community-backup", nil)
	req.Header.Set("X-Panel-Key", a.csrf)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 405 {
		t.Fatal("Backup route method gate missing")
	}
}
func TestCommunityStateRollsBackWriteFailure(t *testing.T) {
	a, _, _ := communityFixture(t)
	a.store.path = filepath.Join(t.TempDir(), "missing", "state.json")
	if e := a.store.updateCommunity(embedGuild, func(v *CommunityState) error { v.NextCase = 7; return nil }); e == nil {
		t.Fatal("Expected disk failure")
	}
	if a.store.community(embedGuild).NextCase != 0 {
		t.Fatal("Failed write changed in-memory state")
	}
}
func TestTemporaryVoiceCreatesMovesAndOnlyDeletesTrackedEmptyRooms(t *testing.T) {
	a, s, tr := communityFixture(t)
	a.cfg.Community.VoiceLobby = communityLobby
	a.cfg.Community.VoiceCategory = communityCategory
	guild, _ := s.State.Guild(embedGuild)
	guild.VoiceStates = []*discordgo.VoiceState{{GuildID: embedGuild, UserID: embedUser, ChannelID: communityLobby}}
	a.onVoiceState(s, &discordgo.VoiceStateUpdate{VoiceState: &discordgo.VoiceState{GuildID: embedGuild, UserID: embedUser, ChannelID: communityLobby, Member: tr.members[embedUser]}})
	if len(a.store.community(embedGuild).VoiceRooms) != 1 {
		t.Fatal("Room not saved")
	}
	moved := false
	for _, call := range tr.calls {
		if call.Method == "PATCH" && call.Body["channel_id"] == communityRoom {
			moved = true
		}
	}
	if !moved {
		t.Fatal("Member not moved")
	}
	a.store.updateCommunity(embedGuild, func(v *CommunityState) error {
		x := v.VoiceRooms[communityRoom]
		x.Created = time.Now().Add(-time.Minute)
		v.VoiceRooms[communityRoom] = x
		return nil
	})
	guild.VoiceStates = []*discordgo.VoiceState{{GuildID: embedGuild, UserID: embedUser, ChannelID: communityRoom}}
	a.cleanVoiceRooms(s)
	if tr.channels[communityRoom] == nil {
		t.Fatal("Occupied room deleted")
	}
	guild.VoiceStates = nil
	a.cleanVoiceRooms(s)
	if tr.channels[communityRoom] != nil || len(a.store.community(embedGuild).VoiceRooms) != 0 {
		t.Fatal("Empty room not cleaned")
	}
	if tr.channels[communityLobby] == nil {
		t.Fatal("Untracked lobby deleted")
	}
}

func TestModalIsBoundToMemberAndUsesDecodedDiscordFields(t *testing.T) {
	a, s, _ := communityFixture(t)
	a.cfg.Community.SuggestChannel = embedChannel
	id := "0123456789abcdef01234567"
	a.modals = map[string]ModalSession{id: {User: embedUser, Guild: embedGuild, Kind: "suggest", Expires: time.Now().Add(time.Minute)}}
	raw := fmt.Sprintf(`{"id":"233456789012345678","application_id":"%s","token":"fake","type":5,"guild_id":"%s","channel_id":"%s","member":{"user":{"id":"%s"}},"data":{"custom_id":"mw:form:%s","components":[{"type":1,"components":[{"type":4,"custom_id":"title","value":"Decoded title","style":1}]},{"type":1,"components":[{"type":4,"custom_id":"body","value":"Decoded suggestion details","style":2}]}]}}`, embedBot, embedGuild, embedChannel, communityInterest, id)
	var interaction discordgo.Interaction
	if e := json.Unmarshal([]byte(raw), &interaction); e != nil {
		t.Fatal(e)
	}
	i := &discordgo.InteractionCreate{Interaction: &interaction}
	a.handleCommunityInteraction(s, i)
	if len(a.store.community(embedGuild).Submissions) != 0 || len(a.modals) != 1 {
		t.Fatal("Wrong member submitted or consumed the form")
	}
	i.Member.User.ID = embedUser
	a.handleCommunityInteraction(s, i)
	all := a.store.community(embedGuild).Submissions
	if len(all) != 1 || len(a.modals) != 0 {
		t.Fatal("Decoded form was not accepted exactly once")
	}
	for _, x := range all {
		if x.Title != "Decoded title" || x.Body != "Decoded suggestion details" {
			t.Fatal("Discord modal fields were lost")
		}
	}
	a.handleCommunityInteraction(s, i)
	if len(a.store.community(embedGuild).Submissions) != 1 {
		t.Fatal("Form replay duplicated submission")
	}
}
func TestPublicRoleChannelOverridesCannotGrantStaffAccess(t *testing.T) {
	a, s, tr := communityFixture(t)
	tr.channels[embedChannel].PermissionOverwrites = []*discordgo.PermissionOverwrite{{ID: embedRole, Type: discordgo.PermissionOverwriteTypeRole, Allow: discordgo.PermissionManageMessages}}
	if e := a.validatePublicRole(s, embedGuild, embedRole); e == nil {
		t.Fatal("A channel-specific staff role was accepted")
	}
}
func TestEventsUpdatedInDiscordRefreshStoredReminderTime(t *testing.T) {
	a, s, _ := communityFixture(t)
	id := "183456789012345678"
	old := time.Now().Add(time.Hour)
	a.store.updateCommunity(embedGuild, func(v *CommunityState) error {
		v.Events[id] = CommunityEvent{ID: id, Name: "Old name", Starts: old, Reminded: true}
		return nil
	})
	updated := old.Add(time.Hour)
	a.onCommunityEventUpdate(s, &discordgo.GuildScheduledEventUpdate{GuildScheduledEvent: &discordgo.GuildScheduledEvent{ID: id, GuildID: embedGuild, Name: "New name", ScheduledStartTime: updated, Status: discordgo.GuildScheduledEventStatusScheduled}})
	x := a.store.community(embedGuild).Events[id]
	if !x.Starts.Equal(updated) || x.Name != "New name" || x.Reminded {
		t.Fatal("Discord edit did not update the reminder")
	}
	a.onCommunityEventDelete(s, &discordgo.GuildScheduledEventDelete{GuildScheduledEvent: &discordgo.GuildScheduledEvent{ID: id, GuildID: embedGuild}})
	if !a.store.community(embedGuild).Events[id].Cancelled {
		t.Fatal("Discord deletion did not cancel reminder")
	}
}
func TestVoiceCleanupWaitsForFullGuildCache(t *testing.T) {
	a, s, tr := communityFixture(t)
	g, _ := s.State.Guild(embedGuild)
	g.Unavailable = true
	tr.channels[communityRoom] = &discordgo.Channel{ID: communityRoom, GuildID: embedGuild, Type: discordgo.ChannelTypeGuildVoice}
	a.store.updateCommunity(embedGuild, func(v *CommunityState) error {
		v.VoiceRooms[communityRoom] = VoiceRoom{ChannelID: communityRoom, Owner: embedUser, Created: time.Now().Add(-time.Hour)}
		return nil
	})
	a.cleanVoiceRooms(s)
	if tr.channels[communityRoom] == nil {
		t.Fatal("Room deleted before voice-state cache was ready")
	}
}
