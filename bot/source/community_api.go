package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

type AnnouncementRequest struct {
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Version string `json:"version"`
	Text    string `json:"text"`
	URL     string `json:"url"`
}

func (a *App) publishAnnouncement(s *discordgo.Session, r AnnouncementRequest) (string, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	c := a.config()
	ch := c.Community.ReleaseChannel
	role := c.Community.ReleaseRole
	title := r.Title
	footer := "Project release"
	if r.Kind == "live" {
		ch = c.Community.LiveChannel
		role = c.Community.LiveRole
		footer = "Live now"
	} else if r.Kind == "release" {
		title += " · " + r.Version
		if strings.TrimSpace(r.Version) == "" {
			return "", errors.New("Enter a release version")
		}
	} else {
		return "", errors.New("Choose a release or livestream announcement")
	}
	if utf8.RuneCountInString(r.Title) < 1 || utf8.RuneCountInString(r.Title) > 100 || utf8.RuneCountInString(r.Version) > 40 || utf8.RuneCountInString(r.Text) > 1800 || !validEmbedURL(r.URL) || len(r.URL) > 300 {
		return "", errors.New("Enter a title, a valid HTTPS link, and text up to 1800 characters")
	}
	if e := a.postingChannel(s, ch); e != nil {
		return "", e
	}
	mentions := noMentions()
	content := r.URL
	if role != "" {
		if e := a.validatePublicRole(s, c.GuildID, role); e != nil {
			return "", e
		}
		content = "<@&" + role + "> " + content
		mentions.Roles = []string{role}
	}
	p := &discordgo.MessageSend{Content: content, Embeds: []*discordgo.MessageEmbed{{Title: clip(title, 256), Description: r.Text, URL: r.URL, Color: 0xb18aff, Footer: &discordgo.MessageEmbedFooter{Text: footer}}}, AllowedMentions: mentions}
	msg, e := a.sendStable(s, ch, "announcement:"+ch+":"+r.Kind+":"+title+":"+r.URL+":"+r.Text, p)
	if e != nil {
		return "", e
	}
	return "https://discord.com/channels/" + c.GuildID + "/" + ch + "/" + msg.ID, nil
}

type PollRequest struct {
	ChannelID string   `json:"channel_id"`
	Question  string   `json:"question"`
	Answers   []string `json:"answers"`
	Hours     int      `json:"hours"`
	Multiple  bool     `json:"multiple"`
}

func pollPayload(r PollRequest) (*discordgo.MessageSend, error) {
	if utf8.RuneCountInString(strings.TrimSpace(r.Question)) < 1 || utf8.RuneCountInString(r.Question) > 300 || len(r.Answers) < 2 || len(r.Answers) > 10 || r.Hours < 1 || r.Hours > 768 {
		return nil, errors.New("Polls need a question up to 300 characters, 2–10 answers and a duration of 1–768 hours")
	}
	answers := []discordgo.PollAnswer{}
	seen := map[string]bool{}
	for _, answer := range r.Answers {
		answer = strings.TrimSpace(answer)
		if utf8.RuneCountInString(answer) < 1 || utf8.RuneCountInString(answer) > 55 || seen[answer] {
			return nil, errors.New("Poll answers must be unique and 1–55 characters")
		}
		seen[answer] = true
		answers = append(answers, discordgo.PollAnswer{Media: &discordgo.PollMedia{Text: answer}})
	}
	return &discordgo.MessageSend{Poll: &discordgo.Poll{Question: discordgo.PollMedia{Text: r.Question}, Answers: answers, Duration: r.Hours, AllowMultiselect: r.Multiple, LayoutType: discordgo.PollLayoutTypeDefault}, AllowedMentions: noMentions()}, nil
}
func (a *App) publishPoll(s *discordgo.Session, r PollRequest) (string, error) {
	p, e := pollPayload(r)
	if e != nil {
		return "", e
	}
	if e = a.postingChannel(s, r.ChannelID); e != nil {
		return "", e
	}
	perms, e := s.UserChannelPermissions(s.State.User.ID, r.ChannelID)
	if e != nil {
		return "", e
	}
	if !hasPermission(perms, discordgo.PermissionSendPolls) {
		return "", errors.New("The bot needs Send Polls in that channel")
	}
	payload, _ := json.Marshal(r)
	msg, e := a.sendStable(s, r.ChannelID, "poll:"+string(payload), p)
	if e != nil {
		return "", e
	}
	return "https://discord.com/channels/" + a.config().GuildID + "/" + r.ChannelID + "/" + msg.ID, nil
}

type EventRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Start           string `json:"start"`
	End             string `json:"end"`
	Location        string `json:"location"`
	ChannelID       string `json:"channel_id"`
	RoleID          string `json:"role_id"`
	ReminderMinutes int    `json:"reminder_minutes"`
}

func validateEvent(r EventRequest) (time.Time, time.Time, error) {
	start, e := time.Parse(time.RFC3339, r.Start)
	if e != nil {
		return time.Time{}, time.Time{}, errors.New("Start time needs an explicit timezone (e.g. 2026-10-04T18:00:00+02:00)")
	}
	end, e := time.Parse(time.RFC3339, r.End)
	if e != nil {
		return start, end, errors.New("End time needs an explicit timezone")
	}
	if !start.After(time.Now()) || !end.After(start) || utf8.RuneCountInString(r.Name) < 1 || utf8.RuneCountInString(r.Name) > 100 || utf8.RuneCountInString(r.Description) > 1000 || utf8.RuneCountInString(r.Location) < 1 || utf8.RuneCountInString(r.Location) > 100 || r.ReminderMinutes < 0 || r.ReminderMinutes > 1440 {
		return start, end, errors.New("Event needs a name, location, future start and later end; reminders may be 0–1440 minutes before start")
	}
	return start.UTC(), end.UTC(), nil
}
func (a *App) createCommunityEvent(s *discordgo.Session, r EventRequest) (CommunityEvent, error) {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	c := a.config()
	start, end, e := validateEvent(r)
	if e != nil {
		return CommunityEvent{}, e
	}
	if r.ChannelID == "" {
		r.ChannelID = c.Community.EventChannel
	}
	if r.ReminderMinutes > 0 {
		if e = a.postingChannel(s, r.ChannelID); e != nil {
			return CommunityEvent{}, e
		}
	}
	if r.RoleID != "" {
		if !validID(r.RoleID) || r.RoleID == c.GuildID {
			return CommunityEvent{}, errors.New("Select a dedicated reminder role")
		}
		if e = a.validatePublicRole(s, c.GuildID, r.RoleID); e != nil {
			return CommunityEvent{}, e
		}
	}
	v := a.store.community(c.GuildID)
	if len(v.Events) >= 500 {
		return CommunityEvent{}, errors.New("Event record limit reached (500)")
	}
	for _, x := range v.Events {
		if x.Name == r.Name && x.Starts.Equal(start) && !x.Cancelled {
			return CommunityEvent{}, errors.New("An event with this name and start time already exists")
		}
	}
	event, e := s.GuildScheduledEventCreate(c.GuildID, &discordgo.GuildScheduledEventParams{Name: r.Name, Description: r.Description, ScheduledStartTime: &start, ScheduledEndTime: &end, PrivacyLevel: discordgo.GuildScheduledEventPrivacyLevelGuildOnly, EntityType: discordgo.GuildScheduledEventEntityTypeExternal, EntityMetadata: &discordgo.GuildScheduledEventEntityMetadata{Location: r.Location}})
	if e != nil {
		return CommunityEvent{}, e
	}
	x := CommunityEvent{ID: event.ID, Name: r.Name, ChannelID: r.ChannelID, RoleID: r.RoleID, Starts: start, Ends: end, ReminderMinutes: r.ReminderMinutes}
	if e = a.store.updateCommunity(c.GuildID, func(v *CommunityState) error { v.Events[event.ID] = x; return nil }); e != nil {
		s.GuildScheduledEventDelete(c.GuildID, event.ID)
		return CommunityEvent{}, e
	}
	if r.ChannelID != "" {
		p := &discordgo.MessageSend{Content: "https://discord.com/events/" + c.GuildID + "/" + event.ID, Embeds: []*discordgo.MessageEmbed{{Title: r.Name, Description: r.Description + fmt.Sprintf("\n\nStarts <t:%d:F>\nLocation: %s", start.Unix(), r.Location), Color: 0xb18aff}}, AllowedMentions: noMentions()}
		if err := a.postingChannel(s, r.ChannelID); err == nil {
			_, err = a.sendStable(s, r.ChannelID, "event-created:"+event.ID, p)
			if err != nil {
				a.log("Event created, announcement failed: " + a.errorText(err))
			}
		}
	}
	return x, nil
}
func (a *App) cancelCommunityEvent(s *discordgo.Session, id string) error {
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	g := a.config().GuildID
	x, ok := a.store.community(g).Events[id]
	if !ok {
		return errors.New("Event was not created by this bot")
	}
	if x.Cancelled {
		return nil
	}
	if e := s.GuildScheduledEventDelete(g, id); e != nil {
		return e
	}
	return a.store.updateCommunity(g, func(v *CommunityState) error { x := v.Events[id]; x.Cancelled = true; v.Events[id] = x; return nil })
}

type BotBackup struct {
	Schema  int        `json:"schema"`
	Version string     `json:"version"`
	Created time.Time  `json:"created"`
	Config  Config     `json:"config"`
	State   SavedState `json:"state"`
}

func (s *Store) snapshot() SavedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.data)
	var v SavedState
	json.Unmarshal(b, &v)
	return v
}
func (a *App) exportBackup() BotBackup {
	c := a.config()
	c.Token = ""
	return BotBackup{1, version, time.Now().UTC(), c, a.store.snapshot()}
}
func validateBackup(b *BotBackup, current Config) error {
	if b.Schema != 1 {
		return errors.New("Unsupported backup format")
	}
	if b.Config.GuildID != current.GuildID {
		return errors.New("Restore only to the same Discord server. IDs and existing message controls cannot be moved between servers")
	}
	b.Config.Token = current.Token
	if e := validateConfig(&b.Config); e != nil {
		return e
	}
	if len(b.State.Embeds) > 100 {
		return errors.New("Backup has too many embed templates")
	}
	for id, p := range b.State.Embeds {
		if id != p.ID || len(id) != 24 || !validID(p.GuildID) {
			return errors.New("Invalid embed record")
		}
		if e := validateEmbed(&p.Draft); e != nil {
			return e
		}
		if p.Published != nil {
			if !validID(p.Published.ChannelID) || !validID(p.Published.MessageID) {
				return errors.New("Invalid published embed")
			}
			if e := validateEmbed(&p.Published.Draft); e != nil {
				return e
			}
		}
	}
	for g, v := range b.State.Community {
		if !validID(g) || len(v.Panels) > 100 || len(v.Tickets) > 2000 || len(v.Submissions) > 2500 || len(v.VoiceRooms) > 50 || len(v.Events) > 500 || len(v.Cases) > 10000 || len(v.Welcomed) > 10000 || len(v.RaidRestricted) > 10000 {
			return errors.New("Backup community records exceed limits or have an invalid server")
		}
		initCommunityState(&v)
		for id, p := range v.Panels {
			if id != p.ID || len(id) != 24 || !validID(p.ChannelID) || !validID(p.MessageID) || len(p.Roles) > 25 {
				return errors.New("Invalid panel binding")
			}
			if p.Kind != "roles" && p.Kind != "ticket" && p.Kind != "submissions" {
				return errors.New("Invalid panel type")
			}
			for _, r := range p.Roles {
				if !validID(r.RoleID) || r.RoleID == g {
					return errors.New("Invalid public role binding")
				}
			}
		}
		for id, t := range v.Tickets {
			if id != t.ID || !validID(t.Owner) || !validID(t.ChannelID) || !validID(t.MessageID) || len(t.StaffRoles) > 10 {
				return errors.New("Invalid ticket binding")
			}
			for _, role := range t.StaffRoles {
				if !validID(role) || role == g {
					return errors.New("Invalid ticket staff role")
				}
			}
		}
		for id, x := range v.Submissions {
			if id != x.ID || !validID(x.Author) || !validID(x.ChannelID) || !validID(x.MessageID) || len(x.Votes) > 5000 {
				return errors.New("Invalid submission binding")
			}
			if x.Kind != "bug" && x.Kind != "suggest" {
				return errors.New("Invalid submission type")
			}
		}
		for id, r := range v.VoiceRooms {
			if id != r.ChannelID || !validID(id) || !validID(r.Owner) {
				return errors.New("Invalid voice room binding")
			}
		}
		for id, e := range v.Events {
			if id != e.ID || !validID(id) || (!validID(e.ChannelID) && e.ChannelID != "") || e.ReminderMinutes < 0 || e.ReminderMinutes > 1440 {
				return errors.New("Invalid event binding")
			}
		}
		for kind, id := range v.NativeRules {
			if (kind != "slurs" && kind != "mentions") || !validID(id) {
				return errors.New("Invalid AutoMod binding")
			}
		}
		b.State.Community[g] = v
	}
	if b.State.Warnings == nil {
		b.State.Warnings = map[string][]Warning{}
	}
	if b.State.Feeds == nil {
		b.State.Feeds = map[string]FeedState{}
	}
	if b.State.Embeds == nil {
		b.State.Embeds = map[string]EmbedPost{}
	}
	if b.State.Community == nil {
		b.State.Community = map[string]CommunityState{}
	}
	return nil
}
func (a *App) restoreBackup(b BotBackup) error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.RLock()
	running := a.session != nil
	a.mu.RUnlock()
	if running {
		return errors.New("Stop the bot before restoring a backup")
	}
	a.youtubeMu.Lock()
	defer a.youtubeMu.Unlock()
	a.embedMu.Lock()
	defer a.embedMu.Unlock()
	a.communityMu.Lock()
	defer a.communityMu.Unlock()
	old := a.config()
	if e := validateBackup(&b, old); e != nil {
		return e
	}
	if e := atomicJSON(filepath.Join(a.dir, "config.json"), b.Config); e != nil {
		return e
	}
	a.store.mu.Lock()
	e := atomicJSON(a.store.path, b.State)
	if e == nil {
		a.store.data = b.State
	}
	a.store.mu.Unlock()
	if e != nil {
		if err := atomicJSON(filepath.Join(a.dir, "config.json"), old); err != nil {
			return fmt.Errorf("Restore failed (%v); previous config could not be restored (%v)", e, err)
		}
		return e
	}
	a.mu.Lock()
	a.cfg = b.Config
	a.mu.Unlock()
	a.log("Backup restored. The existing bot token was kept; start the bot to connect.")
	return nil
}
func (a *App) communityRoutes(mux *http.ServeMux) {
	decode := func(w http.ResponseWriter, r *http.Request, d any) bool {
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(d); e != nil {
			apiError(w, e)
			return false
		}
		return true
	}
	finish := func(w http.ResponseWriter, v any, e error) {
		if e != nil {
			apiError(w, errors.New(a.errorText(e)))
			return
		}
		sendJSON(w, v)
	}
	mux.HandleFunc("/api/community", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		c := a.config()
		v := a.store.community(c.GuildID)
		cases := v.Cases
		if len(cases) > 100 {
			cases = cases[len(cases)-100:]
		}
		sendJSON(w, map[string]any{"config": c.Community, "panels": v.Panels, "tickets": v.Tickets, "submissions": sortedSubmissions(v), "events": v.Events, "cases": cases, "raid_until": v.RaidUntil, "native_rules": v.NativeRules, "voice_rooms": v.VoiceRooms})
	})
	mux.HandleFunc("/api/community-save", func(w http.ResponseWriter, r *http.Request) {
		a.lifecycle.Lock()
		defer a.lifecycle.Unlock()
		a.mu.RLock()
		running := a.session != nil
		a.mu.RUnlock()
		if running {
			apiError(w, errors.New("Stop the bot before saving community settings"))
			return
		}
		var cc CommunityConfig
		if !decode(w, r, &cc) {
			return
		}
		c := a.config()
		if !validID(c.GuildID) {
			apiError(w, errors.New("Save your server connection first"))
			return
		}
		if e := validateCommunity(&cc, c); e != nil {
			apiError(w, e)
			return
		}
		c.Community = cc
		if e := atomicJSON(filepath.Join(a.dir, "config.json"), c); e != nil {
			apiError(w, e)
			return
		}
		a.mu.Lock()
		a.cfg = c
		a.mu.Unlock()
		sendJSON(w, map[string]any{"ok": true, "config": cc})
	})
	mux.HandleFunc("/api/community-publish", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Kind string `json:"kind"`
		}
		if !decode(w, r, &req) {
			return
		}
		s, e := a.current()
		if e != nil {
			finish(w, nil, e)
			return
		}
		p, e := a.publishCommunityPanel(s, req.Kind)
		finish(w, map[string]any{"panel": p, "url": "https://discord.com/channels/" + a.config().GuildID + "/" + p.ChannelID + "/" + p.MessageID}, e)
	})
	mux.HandleFunc("/api/community-announcement", func(w http.ResponseWriter, r *http.Request) {
		var req AnnouncementRequest
		if !decode(w, r, &req) {
			return
		}
		s, e := a.current()
		if e != nil {
			finish(w, nil, e)
			return
		}
		link, e := a.publishAnnouncement(s, req)
		finish(w, map[string]string{"url": link}, e)
	})
	mux.HandleFunc("/api/community-poll", func(w http.ResponseWriter, r *http.Request) {
		var req PollRequest
		if !decode(w, r, &req) {
			return
		}
		s, e := a.current()
		if e != nil {
			finish(w, nil, e)
			return
		}
		link, e := a.publishPoll(s, req)
		finish(w, map[string]string{"url": link}, e)
	})
	mux.HandleFunc("/api/community-event", func(w http.ResponseWriter, r *http.Request) {
		var req EventRequest
		if !decode(w, r, &req) {
			return
		}
		s, e := a.current()
		if e != nil {
			finish(w, nil, e)
			return
		}
		event, e := a.createCommunityEvent(s, req)
		finish(w, map[string]any{"event": event, "url": "https://discord.com/events/" + a.config().GuildID + "/" + event.ID}, e)
	})
	mux.HandleFunc("/api/community-event-cancel", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		if !decode(w, r, &req) {
			return
		}
		s, e := a.current()
		if e == nil {
			e = a.cancelCommunityEvent(s, req.ID)
		}
		finish(w, map[string]bool{"ok": true}, e)
	})
	mux.HandleFunc("/api/community-automod", func(w http.ResponseWriter, r *http.Request) {
		s, e := a.current()
		if e == nil {
			e = a.syncNativeAutoMod(s)
		}
		finish(w, map[string]bool{"ok": true}, e)
	})
	mux.HandleFunc("/api/community-raid", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Active  bool `json:"active"`
			Minutes int  `json:"minutes"`
		}
		if !decode(w, r, &req) {
			return
		}
		s, e := a.current()
		if e != nil {
			finish(w, nil, e)
			return
		}
		if req.Active {
			if req.Minutes < 1 || req.Minutes > 60 {
				apiError(w, errors.New("Raid mode duration must be 1–60 minutes"))
				return
			}
			e = a.store.updateCommunity(a.config().GuildID, func(v *CommunityState) error {
				v.RaidUntil = time.Now().Add(time.Duration(req.Minutes) * time.Minute)
				return nil
			})
			if e == nil {
				a.audit(s, "Raid mode enabled from the local control panel")
			}
		} else {
			e = a.endRaid(s, s.State.User.ID)
		}
		finish(w, map[string]bool{"ok": true}, e)
	})
	mux.HandleFunc("/api/community-backup", func(w http.ResponseWriter, r *http.Request) { sendJSON(w, a.exportBackup()) })
	mux.HandleFunc("/api/community-restore", func(w http.ResponseWriter, r *http.Request) {
		var b BotBackup
		if !decode(w, r, &b) {
			return
		}
		finish(w, map[string]bool{"ok": true}, a.restoreBackup(b))
	})

	mux.HandleFunc("/api/community-case-note", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID   string `json:"id"`
			Note string `json:"note"`
		}
		if !decode(w, r, &req) {
			return
		}
		if utf8.RuneCountInString(strings.TrimSpace(req.Note)) < 1 || utf8.RuneCountInString(req.Note) > 400 {
			apiError(w, errors.New("Enter a note up to 400 characters"))
			return
		}
		g := a.config().GuildID
		e := a.store.updateCommunity(g, func(v *CommunityState) error {
			for j := range v.Cases {
				if v.Cases[j].ID == req.ID {
					v.Cases[j].Note = clip(v.Cases[j].Note+"\n"+time.Now().UTC().Format(time.RFC3339)+" · local panel: "+req.Note, 1800)
					return nil
				}
			}
			return errors.New("Case not found")
		})
		finish(w, map[string]bool{"ok": true}, e)
	})
	mux.HandleFunc("/api/community-case-reverse", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID      string `json:"id"`
			Reason  string `json:"reason"`
			Confirm bool   `json:"confirm"`
		}
		if !decode(w, r, &req) {
			return
		}
		if !req.Confirm || utf8.RuneCountInString(strings.TrimSpace(req.Reason)) < 1 || utf8.RuneCountInString(req.Reason) > 400 {
			apiError(w, errors.New("Enter a reason and confirm the reversal"))
			return
		}
		s, e := a.current()
		if e != nil {
			finish(w, nil, e)
			return
		}
		x, ok := findCase(a.store.community(a.config().GuildID), req.ID)
		if !ok {
			apiError(w, errors.New("Case not found"))
			return
		}
		i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{GuildID: a.config().GuildID, Member: &discordgo.Member{User: s.State.User, Permissions: discordgo.PermissionAdministrator}}}
		text, e := a.reverseCase(s, i, x, "Local control panel: "+req.Reason)
		finish(w, map[string]string{"message": text}, e)
	})
	mux.HandleFunc("/api/community-ticket", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string `json:"id"`
			Closed bool   `json:"closed"`
		}
		if !decode(w, r, &req) {
			return
		}
		s, e := a.current()
		if e != nil {
			finish(w, nil, e)
			return
		}
		t, ok := a.store.community(a.config().GuildID).Tickets[req.ID]
		if !ok {
			apiError(w, errors.New("Ticket not found"))
			return
		}
		m := &discordgo.Member{User: s.State.User, Permissions: discordgo.PermissionAdministrator}
		text, e := a.setTicketClosed(s, m, t, req.Closed)
		finish(w, map[string]string{"message": text}, e)
	})
}
