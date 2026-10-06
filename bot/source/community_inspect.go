package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
)

func (a *App) inspectCommunity(s *discordgo.Session) []string {
	c := a.config()
	cc := c.Community
	out := []string{}
	for _, item := range []struct{ name, id string }{{"Welcome", cc.WelcomeChannel}, {"Role menu", cc.RolesChannel}, {"Livestream announcements", cc.LiveChannel}, {"Release announcements", cc.ReleaseChannel}, {"Ticket panel", cc.TicketPanelChannel}, {"Suggestions", cc.SuggestChannel}, {"Bug reports", cc.BugChannel}, {"Submission panel", cc.SubmissionPanelChannel}, {"Event announcements", cc.EventChannel}} {
		if item.id == "" {
			continue
		}
		if e := a.postingChannel(s, item.id); e != nil {
			out = append(out, item.name+": "+a.errorText(e))
		} else {
			out = append(out, item.name+": channel and posting permissions OK")
		}
	}
	for _, r := range notificationRoles(c) {
		if e := a.validatePublicRole(s, c.GuildID, r.RoleID); e != nil {
			out = append(out, "Public role "+r.Label+": "+a.errorText(e))
		} else {
			out = append(out, "Public role "+r.Label+": safe role below the bot")
		}
	}
	if cc.TicketPanelChannel != "" || cc.TicketCategory != "" {
		if len(cc.TicketStaffRoles) == 0 {
			out = append(out, "Tickets: choose at least one staff role")
		}
		if e := a.checkStaffRoles(s); e != nil {
			out = append(out, "Tickets: "+a.errorText(e))
		}
		if e := a.botGuildPermission(s, discordgo.PermissionManageChannels); e != nil {
			out = append(out, "Tickets: bot needs Manage Channels")
		}
		if cc.TicketCategory != "" {
			if _, e := a.checkCommunityChannel(s, cc.TicketCategory, discordgo.ChannelTypeGuildCategory); e != nil {
				out = append(out, "Ticket category: "+a.errorText(e))
			}
		}
	}
	if cc.VoiceLobby != "" {
		if _, e := a.checkCommunityChannel(s, cc.VoiceLobby, discordgo.ChannelTypeGuildVoice); e != nil {
			out = append(out, "Voice lobby: "+a.errorText(e))
		}
		if _, e := a.checkCommunityChannel(s, cc.VoiceCategory, discordgo.ChannelTypeGuildCategory); e != nil {
			out = append(out, "Voice category: "+a.errorText(e))
		}
		if e := a.botGuildPermission(s, discordgo.PermissionManageChannels|discordgo.PermissionVoiceMoveMembers); e != nil {
			out = append(out, "Temporary rooms: bot needs Manage Channels and Move Members")
		}
	}
	if cc.RaidEnabled {
		if e := a.botGuildPermission(s, discordgo.PermissionModerateMembers); e != nil {
			out = append(out, "Anti-raid: bot needs Moderate Members")
		}
		out = append(out, fmt.Sprintf("Anti-raid: %d joins within %d seconds; temporary %d-minute restrictions", cc.RaidJoins, cc.RaidWindow, cc.RaidMinutes))
	}
	if cc.NativeAutoMod {
		if e := a.botGuildPermission(s, discordgo.PermissionManageServer); e != nil {
			out = append(out, "Discord AutoMod: bot needs Manage Server")
		}
		out = append(out, "Discord AutoMod: press Apply Discord AutoMod after saving keyword or mention settings")
	}
	if cc.EventChannel != "" {
		if e := a.botGuildPermission(s, discordgo.PermissionManageEvents|discordgo.PermissionCreateEvents); e != nil {
			out = append(out, "Events: grant Create Events and Manage Events")
		}
	}
	for _, p := range a.store.community(c.GuildID).Panels {
		if p.Kind == "roles" {
			current := map[string]bool{}
			for _, r := range notificationRoles(c) {
				current[r.RoleID] = true
			}
			for _, r := range p.Roles {
				if !current[r.RoleID] {
					out = append(out, "A posted role menu is outdated: publish the role menu again")
					break
				}
			}
		}
	}
	out = append(out, "Polls require Send Polls in the selected channel; event reminders require this app online.")
	return out
}
