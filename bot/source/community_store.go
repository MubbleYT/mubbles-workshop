package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

type SelfRole struct {
	Label       string `json:"label"`
	RoleID      string `json:"role_id"`
	Description string `json:"description"`
}
type FAQEntry struct {
	Name string `json:"name"`
	Text string `json:"text"`
}
type CommunityConfig struct {
	WelcomeChannel         string     `json:"welcome_channel"`
	WelcomeText            string     `json:"welcome_text"`
	RulesChannel           string     `json:"rules_channel"`
	ShowcaseChannel        string     `json:"showcase_channel"`
	RolesChannel           string     `json:"roles_channel"`
	SelfRoles              []SelfRole `json:"self_roles"`
	LiveRole               string     `json:"live_role"`
	ReleaseRole            string     `json:"release_role"`
	LiveChannel            string     `json:"live_channel"`
	ReleaseChannel         string     `json:"release_channel"`
	TicketPanelChannel     string     `json:"ticket_panel_channel"`
	TicketCategory         string     `json:"ticket_category"`
	TicketStaffRoles       []string   `json:"ticket_staff_roles"`
	SuggestChannel         string     `json:"suggest_channel"`
	BugChannel             string     `json:"bug_channel"`
	SubmissionPanelChannel string     `json:"submission_panel_channel"`
	VoiceLobby             string     `json:"voice_lobby"`
	VoiceCategory          string     `json:"voice_category"`
	VoiceLimit             int        `json:"voice_limit"`
	RaidEnabled            bool       `json:"raid_enabled"`
	RaidJoins              int        `json:"raid_joins"`
	RaidWindow             int        `json:"raid_window"`
	RaidMinutes            int        `json:"raid_minutes"`
	MentionLimit           int        `json:"mention_limit"`
	NativeAutoMod          bool       `json:"native_automod"`
	EventChannel           string     `json:"event_channel"`
	FAQ                    []FAQEntry `json:"faq"`
}

func defaultCommunityConfig() CommunityConfig {
	return CommunityConfig{WelcomeText: "Hey {user}, welcome to {server}!\nHave a look at {rules}, pick your interests in {roles}, and show us what you're working on in {showcase}.", SelfRoles: []SelfRole{}, TicketStaffRoles: []string{}, FAQ: []FAQEntry{{"mods", "The mod download links are not listed here yet. Check the release channel or ask Mubble."}, {"shaders", "The shader downloads and installation guide are not listed here yet. Check the release channel or ask Mubble."}, {"youtube", "The YouTube channel link is not listed here yet. Ask Mubble for it."}, {"support", "Use /ticket to contact the moderators privately, or /bugreport to report a project issue."}}, VoiceLimit: 8, RaidJoins: 10, RaidWindow: 20, RaidMinutes: 10, MentionLimit: 8}
}
func normalizeCommunity(c *CommunityConfig) {
	d := defaultCommunityConfig()
	if c.WelcomeText == "" {
		c.WelcomeText = d.WelcomeText
	}
	if c.VoiceLimit == 0 {
		c.VoiceLimit = d.VoiceLimit
	}
	if c.RaidJoins == 0 {
		c.RaidJoins = d.RaidJoins
	}
	if c.RaidWindow == 0 {
		c.RaidWindow = d.RaidWindow
	}
	if c.RaidMinutes == 0 {
		c.RaidMinutes = d.RaidMinutes
	}
	if c.MentionLimit == 0 {
		c.MentionLimit = d.MentionLimit
	}
	if c.SelfRoles == nil {
		c.SelfRoles = []SelfRole{}
	}
	if c.TicketStaffRoles == nil {
		c.TicketStaffRoles = []string{}
	}
	if c.FAQ == nil {
		c.FAQ = d.FAQ
	}
}
func validateCommunity(c *CommunityConfig, base Config) error {
	normalizeCommunity(c)
	ids := []*string{&c.WelcomeChannel, &c.RulesChannel, &c.ShowcaseChannel, &c.RolesChannel, &c.LiveRole, &c.ReleaseRole, &c.LiveChannel, &c.ReleaseChannel, &c.TicketPanelChannel, &c.TicketCategory, &c.SuggestChannel, &c.BugChannel, &c.SubmissionPanelChannel, &c.VoiceLobby, &c.VoiceCategory, &c.EventChannel}
	for _, p := range ids {
		*p = strings.TrimSpace(*p)
		if *p != "" && !validID(*p) {
			return errors.New("Community channel and role IDs must be valid Discord IDs")
		}
	}
	if len(c.SelfRoles) > 22 {
		return errors.New("Use up to 22 interest roles; three slots are reserved for notification roles")
	}
	used := map[string]bool{}
	for j := range c.SelfRoles {
		r := &c.SelfRoles[j]
		r.Label = strings.TrimSpace(r.Label)
		r.RoleID = strings.TrimSpace(r.RoleID)
		if !validID(r.RoleID) || r.RoleID == base.GuildID || used[r.RoleID] || utf8.RuneCountInString(r.Label) < 1 || utf8.RuneCountInString(r.Label) > 80 || utf8.RuneCountInString(r.Description) > 100 {
			return errors.New("Each interest role needs a unique role ID, a label (1–80 characters), and a description up to 100 characters")
		}
		used[r.RoleID] = true
	}
	for _, id := range []string{base.NotifyRoleID, c.LiveRole, c.ReleaseRole} {
		if id != "" && id == base.GuildID {
			return errors.New("Notification roles cannot be @everyone")
		}
	}
	if len(c.TicketStaffRoles) > 10 {
		return errors.New("Use up to 10 ticket staff roles")
	}
	seen := map[string]bool{}
	for _, id := range c.TicketStaffRoles {
		if !validID(id) || id == base.GuildID || seen[id] {
			return errors.New("Ticket staff roles must be unique role IDs, excluding @everyone")
		}
		seen[id] = true
	}
	if c.TicketPanelChannel != "" && len(c.TicketStaffRoles) == 0 {
		return errors.New("Choose at least one ticket staff role before publishing a ticket panel")
	}
	if utf8.RuneCountInString(c.WelcomeText) > 1800 {
		return errors.New("Welcome text must be under 1800 characters")
	}
	if c.VoiceLimit < 1 || c.VoiceLimit > 99 || c.RaidJoins < 3 || c.RaidJoins > 100 || c.RaidWindow < 5 || c.RaidWindow > 120 || c.RaidMinutes < 1 || c.RaidMinutes > 60 || c.MentionLimit < 2 || c.MentionLimit > 50 {
		return errors.New("Voice limit: 1–99. Raid: 3–100 joins within 5–120 seconds, restriction 1–60 minutes. Mention limit: 2–50")
	}
	if len(c.FAQ) > 30 {
		return errors.New("Use up to 30 FAQ entries")
	}
	names := map[string]bool{}
	for j := range c.FAQ {
		f := &c.FAQ[j]
		f.Name = strings.ToLower(strings.TrimSpace(f.Name))
		if f.Name == "" || len(f.Name) > 32 || names[f.Name] || utf8.RuneCountInString(f.Text) < 1 || utf8.RuneCountInString(f.Text) > 1800 {
			return errors.New("FAQ entries need unique names up to 32 characters and answers up to 1800 characters")
		}
		for _, r := range f.Name {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return errors.New("FAQ names can contain a–z, 0–9 and hyphens")
			}
		}
		names[f.Name] = true
	}
	if base.TrapChannelID != "" {
		for _, id := range []string{base.NotifyChannelID, base.LogChannelID, c.WelcomeChannel, c.RolesChannel, c.LiveChannel, c.ReleaseChannel, c.TicketPanelChannel, c.SuggestChannel, c.BugChannel, c.SubmissionPanelChannel, c.EventChannel} {
			if id == base.TrapChannelID {
				return errors.New("The bot-trap channel must be separate from public bot panels and announcement channels")
			}
		}
	}
	if c.VoiceLobby != "" && c.VoiceCategory == "" {
		return errors.New("Choose a category for temporary voice rooms")
	}
	return nil
}
func notificationRoles(c Config) []SelfRole {
	out := append([]SelfRole{}, c.Community.SelfRoles...)
	seen := map[string]bool{}
	for _, r := range out {
		seen[r.RoleID] = true
	}
	for _, r := range []SelfRole{{"New videos", c.NotifyRoleID, "Get pinged for new YouTube uploads"}, {"Livestreams", c.Community.LiveRole, "Get pinged when Mubble goes live"}, {"Mod & shader releases", c.Community.ReleaseRole, "Get pinged for project updates"}} {
		if r.RoleID != "" && !seen[r.RoleID] {
			out = append(out, r)
			seen[r.RoleID] = true
		}
	}
	return out
}

// Public self-role menus may never grant moderation or management powers.
const unsafeSelfRolePermissions int64 = discordgo.PermissionAdministrator | discordgo.PermissionKickMembers | discordgo.PermissionBanMembers | discordgo.PermissionManageChannels | discordgo.PermissionManageServer | discordgo.PermissionManageMessages | discordgo.PermissionManageRoles | discordgo.PermissionManageWebhooks | discordgo.PermissionModerateMembers | discordgo.PermissionMentionEveryone | discordgo.PermissionManageEvents | discordgo.PermissionManageThreads | discordgo.PermissionVoiceMoveMembers | discordgo.PermissionVoiceMuteMembers | discordgo.PermissionVoiceDeafenMembers | discordgo.PermissionManageNicknames | discordgo.PermissionViewAuditLogs | discordgo.PermissionManageEmojis | discordgo.PermissionCreateEvents | discordgo.PermissionCreateGuildExpressions

func (a *App) validatePublicRole(s *discordgo.Session, g, id string) error {
	return a.validateRulesRole(s, g, id)
}

type ModCase struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Actor        string     `json:"actor"`
	Target       string     `json:"target"`
	ChannelID    string     `json:"channel_id"`
	Reason       string     `json:"reason"`
	WarningID    string     `json:"warning_id,omitempty"`
	Created      time.Time  `json:"created"`
	Until        *time.Time `json:"until,omitempty"`
	ReversedBy   string     `json:"reversed_by,omitempty"`
	ReversalCase string     `json:"reversal_case,omitempty"`
	Note         string     `json:"note,omitempty"`
}
type CommunityPanel struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	ChannelID string     `json:"channel_id"`
	MessageID string     `json:"message_id"`
	Roles     []SelfRole `json:"roles,omitempty"`
}
type Ticket struct {
	ID         string     `json:"id"`
	Owner      string     `json:"owner"`
	ChannelID  string     `json:"channel_id"`
	MessageID  string     `json:"message_id"`
	StaffRoles []string   `json:"staff_roles"`
	Subject    string     `json:"subject"`
	Body       string     `json:"body"`
	Created    time.Time  `json:"created"`
	Closed     bool       `json:"closed"`
	ClosedBy   string     `json:"closed_by,omitempty"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
}
type Submission struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Author    string         `json:"author"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Version   string         `json:"version"`
	Expected  string         `json:"expected"`
	ImageURL  string         `json:"image_url"`
	Status    string         `json:"status"`
	ChannelID string         `json:"channel_id"`
	MessageID string         `json:"message_id"`
	Votes     map[string]int `json:"votes"`
	Created   time.Time      `json:"created"`
	UpdatedBy string         `json:"updated_by,omitempty"`
}
type VoiceRoom struct {
	ChannelID string    `json:"channel_id"`
	Owner     string    `json:"owner"`
	Created   time.Time `json:"created"`
}
type CommunityEvent struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ChannelID       string    `json:"channel_id"`
	RoleID          string    `json:"role_id"`
	Starts          time.Time `json:"starts"`
	Ends            time.Time `json:"ends"`
	ReminderMinutes int       `json:"reminder_minutes"`
	Reminded        bool      `json:"reminded"`
	Cancelled       bool      `json:"cancelled"`
}
type CommunityState struct {
	NextCase       uint64                    `json:"next_case"`
	Cases          []ModCase                 `json:"cases"`
	Panels         map[string]CommunityPanel `json:"panels"`
	Tickets        map[string]Ticket         `json:"tickets"`
	Submissions    map[string]Submission     `json:"submissions"`
	VoiceRooms     map[string]VoiceRoom      `json:"voice_rooms"`
	Events         map[string]CommunityEvent `json:"events"`
	NativeRules    map[string]string         `json:"native_rules"`
	RaidUntil      time.Time                 `json:"raid_until"`
	RaidRestricted map[string]time.Time      `json:"raid_restricted"`
	Welcomed       map[string]string         `json:"welcomed"`
}

func initCommunityState(c *CommunityState) {
	if c.Cases == nil {
		c.Cases = []ModCase{}
	}
	if c.Panels == nil {
		c.Panels = map[string]CommunityPanel{}
	}
	if c.Tickets == nil {
		c.Tickets = map[string]Ticket{}
	}
	if c.Submissions == nil {
		c.Submissions = map[string]Submission{}
	}
	if c.VoiceRooms == nil {
		c.VoiceRooms = map[string]VoiceRoom{}
	}
	if c.Events == nil {
		c.Events = map[string]CommunityEvent{}
	}
	if c.NativeRules == nil {
		c.NativeRules = map[string]string{}
	}
	if c.RaidRestricted == nil {
		c.RaidRestricted = map[string]time.Time{}
	}
	if c.Welcomed == nil {
		c.Welcomed = map[string]string{}
	}
}
func (s *Store) community(g string) CommunityState {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data.Community[g]
	b, _ := json.Marshal(v)
	var out CommunityState
	json.Unmarshal(b, &out)
	initCommunityState(&out)
	return out
}
func (s *Store) updateCommunity(g string, fn func(*CommunityState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Community == nil {
		s.data.Community = map[string]CommunityState{}
	}
	old := s.data.Community[g]
	b, err := json.Marshal(old)
	if err != nil {
		return err
	}
	var next CommunityState
	if err = json.Unmarshal(b, &next); err != nil {
		return err
	}
	initCommunityState(&next)
	if err = fn(&next); err != nil {
		return err
	}
	s.data.Community[g] = next
	if err = atomicJSON(s.path, s.data); err != nil {
		s.data.Community[g] = old
		return err
	}
	return nil
}
func (a *App) recordCase(s *discordgo.Session, kind, actor, target, channel, reason, warning string, until *time.Time) string {
	var id string
	g := a.config().GuildID
	err := a.store.updateCommunity(g, func(v *CommunityState) error {
		v.NextCase++
		id = fmt.Sprint(v.NextCase)
		v.Cases = append(v.Cases, ModCase{ID: id, Kind: kind, Actor: actor, Target: target, ChannelID: channel, Reason: clip(reason, 500), WarningID: warning, Created: time.Now().UTC(), Until: until})
		if len(v.Cases) > 10000 {
			v.Cases = v.Cases[len(v.Cases)-10000:]
		}
		return nil
	})
	if err != nil {
		a.log("Action succeeded but case could not be saved: " + err.Error())
		return ""
	}
	a.audit(s, fmt.Sprintf("Case #%s · %s · target %s · by %s · %s", id, kind, target, actor, clip(reason, 400)))
	return id
}
func findCase(c CommunityState, id string) (ModCase, bool) {
	for _, x := range c.Cases {
		if x.ID == id {
			return x, true
		}
	}
	return ModCase{}, false
}
func sortedSubmissions(c CommunityState) []Submission {
	out := []Submission{}
	for _, v := range c.Submissions {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}
