MUBBLE DISCORD BOT — WINDOWS 64-BIT — VERSION 1.4.0

START HERE
1. Extract this ZIP into a folder.
2. Double-click MubbleDiscordBot.exe. No Python, Node.js, or Go installation is needed.
3. A local control panel opens in your browser. Keep the bot's console window open.
4. Follow "First-time setup" in the panel, save settings, and click Start bot.
5. To stop, click Stop bot, or press Ctrl+C in the bot window.

DISCORD SETUP
Create your application at https://discord.com/developers/applications .
Open Bot and copy the BOT TOKEN. Do not use your personal Discord account token.
Enable Server Members Intent and Message Content Intent under Privileged Gateway Intents.
In OAuth2 -> URL Generator, select bot and applications.commands.
Select: View Channels, Send Messages, Embed Links, Read Message History,
Manage Messages, Manage Roles, Moderate Members, Kick Members, Ban Members,
Manage Channels, Add Reactions. Community features additionally use Manage Server,
Create Events, Manage Events, Send Polls, Attach Files, Connect, Speak, Move Members. Open the generated URL and invite the bot to your server.
Enable Discord User Settings -> Advanced -> Developer Mode to copy IDs.
The bot's highest role must sit above Member and the members it should moderate.
After connecting, "Check setup / load role IDs" populates role/channel suggestions.
Channel permission overrides can restrict moderation actions even when server
permissions are correct. Use the setup check and activity log to diagnose this.

YOUR BOT'S IDENTITY
Use any valid 2–32 character bot name. To change its actual Discord username
and picture, connect the bot, then use "Bot name & profile picture".
Choose a PNG, JPG or GIF below 2 MB. Leave the file blank to keep the current
avatar. Check "Reset" to remove the avatar. Discord rate limits profile changes.
The app/application label and icon in the Developer Portal are separate;
you can edit those there as well.

AUTOMATIC MEMBER ROLE
Create a normal Member role in Discord and enter/select its ID in settings.
Human members joining while the bot is online receive the role automatically.
If Discord membership screening is enabled, assignment waits until they accept
the rules. Bots are excluded. Existing members are not bulk-modified.
The Member role must be below the bot's highest role, and the bot needs Manage Roles.

COMMUNITY TOOLKIT
================================
All twelve suggested additions are included. New features stay inactive until
configured; your existing settings, rules embeds and video watcher are retained.

QUICK SETUP
1. Close the old bot window, then run this version of MubbleDiscordBot.exe.
2. Open Community toolkit in the local panel. Stop the bot to edit settings.
3. Select the channels and roles for the features you want, then Save community
   settings. Blank channel fields disable the corresponding optional feature.
4. Start the bot, then use Check community setup. This loads text-channel,
   category, voice-channel and assignable-role IDs into the matching fields.
5. Use Invite bot / update permissions from Bot settings if permissions are
   missing. Grant only the features you use; Administrator is not required.
6. Publish the role menu, ticket panel and submission panel. Publishing again
   updates the existing panel in the same channel. Changing channels creates a
   new panel. Keep outdated panels out of view if you change their channel.
7. Customize FAQ answers, welcome text and notification roles to suit your server.

WELCOME AND ONBOARDING
A customizable message welcomes new human members after native membership
screening. Supported placeholders: {user}, {server}, {rules}, {roles}, {showcase}.
Only the joining member is pinged. A saved join timestamp prevents duplicate
welcome messages during screening updates. It does not send retroactive welcomes
to every existing member when you upgrade. Member auto-role assignment continues.

INTEREST ROLES AND OPTIONAL NOTIFICATIONS
Add up to 22 interest roles with labels and descriptions. The role dropdown also
includes the upload role from Bot settings, plus optional livestream and release
roles. Members select every offered role they want to keep; unchecked offered
roles are removed. An empty selection opts out of all menu roles. Other roles,
including a separate rules role, are retained.
Use ordinary roles below the bot, without staff, management, or channel-specific
moderation permissions. Roles with these powers are rejected by the menu, rules
buttons/reactions, and automatic Member-role assignment. Complete membership
screening first. /roles links to the posted menu. Role changes validate permissions
again at use time and try to roll back partial failures.
Make notification roles mentionable. Announcements only permit the matching role
ping; typed @everyone or unrelated role mentions do not create mass pings.

PRIVATE TICKETS AND REPORTS
Choose a ticket panel channel, optional category, and one or more staff role IDs.
The panel's button, /ticket or /report opens a short private-contact form. A new
private text channel is created for the submitting member and selected staff.
Server administrators can also see it, as Discord administrators bypass channel
restrictions. Details are posted in the private ticket, not the public panel.
Members may have two open tickets and must wait a minute between submissions.
Closing a ticket retains its private channel and history. The owner can read it
but cannot send; selected staff can reopen it. Use its Close button, /ticket-close
inside the ticket, or the panel's ticket list. Tickets remain saved across restarts.
This bot does not delete closed ticket history or copy it into a public log.

SUGGESTIONS AND STRUCTURED BUG REPORTS
Choose public suggestion and bug-report channels, then publish a submission panel.
/suggest and /bugreport also open the forms. Bug reports ask for a title, project
and Minecraft versions, reproduction steps, expected behavior, and an optional
HTTPS screenshot or clip link. Submissions are public in the chosen channels.
Members vote Support or Against; each member has one current vote per submission.
A vote can be changed or removed. Moderators with Manage Messages can set New,
Under review, Planned, In progress, Fixed or Declined through the status menu.
Votes and status survive restarts. Submissions are limited to one per member per
minute. Use the local panel to open the relevant Discord cards.

MODERATION CASES AND REVERSALS
Successful warnings, timeouts, kicks, bans, unbans, purges and slowmode actions
receive a saved case number. Automatic slur bans, trap bans and raid timeouts are
also recorded. The panel shows the latest 100 cases with search, notes and eligible
reversals. /cases member:USER shows recent cases for a member; /case id:NUMBER
shows details; /case-note adds a staff note. The latest 10,000 cases are retained.
/case-reverse id:NUMBER reason:TEXT confirm:true reverses a ban, matching active
timeout or saved warning. It requires the relevant Discord permission and creates
a linked reversal case. Newer punishments are protected from an older reversal.
Kicks and deleted messages cannot be restored automatically; add a note instead.
Ordinary /unban, /untimeout and /remove-warning also link their reversal to a
matching recorded case. Local-panel reversals are attributed to the bot and marked
as requested through the local panel. Discord role hierarchy still applies.

RELEASE AND LIVESTREAM ANNOUNCEMENTS
Set separate announcement channels and optional opt-in ping roles. Use the local
posting form or /release project:NAME version:VERSION changes:TEXT url:HTTPS_URL.
A consistent embed includes the project, version, changes and download link.
/live title:TITLE url:HTTPS_URL posts a manual livestream announcement. Automatic
YouTube upload detection uses the original public channel feed with no API key. Notifications are explicitly opt-in via roles.

ANTI-RAID AND MASS-MENTION PROTECTION
Enable the join-spike and mass-mention options in Community toolkit. Defaults are
10 human joins within 20 seconds, a 10-minute newcomer timeout, and at most 8 unique
mentions in a message. A join spike temporarily restricts the recent newcomers and
members joining while raid mode is active. It never bans people for joining or
having a young account. Existing longer timeouts are left alone. Discord protects
the owner, administrators and members the bot cannot moderate.
Mass-mention spam is deleted and the sender is timed out; Manage Messages moderators
are exempt. /raid-mode mode:on minutes:10 or the panel enables raid mode manually.
Ending raid mode removes only tracked timeouts that still match the bot's raid
restriction, preserving independently changed punishments. Natural timeouts expire
through Discord even if the app is offline. Failed removals are reported for retry.

NATIVE DISCORD AUTOMOD
Enable native filters, save, start, then Apply Discord AutoMod. The bot creates its
own keyword rule from your slur list and a mention-spam rule from your mention limit.
These native blocks keep operating while your PC is off. With the bot online,
blocked listed slurs also trigger the existing automatic-ban policy and case log.
The native keyword matcher follows Discord's matching rules; the existing bot-side
normalization remains active for messages that are posted. General swearing stays
subject to the bot's configured limits; no native profanity preset is added.
After changing the lists or limits, apply again. Turning the native option off also
requires applying to disable the previously installed rules. Unrelated rules are
not modified; if Discord's rule quota is full, the panel reports the conflict.
Requires Manage Server. Native Discord rules have Discord's own exemptions, so
bot-side enforcement and role hierarchy remain relevant.

TEMPORARY VOICE ROOMS
Choose an existing Create-a-room voice channel and a category. Joining that lobby
creates a voice room, inherits the category permissions, and moves the member in.
The default size is configurable. Bots and members still in screening are ignored.
/voice name:NAME or /voice limit:NUMBER lets the room's owner adjust it.
Empty tracked rooms are removed after a 30-second grace period, normally within
the next 30-second housekeeping check. Occupied rooms and unrelated channels are
kept. Saved rooms are reconciled after restart once the guild cache is ready.
Requires Manage Channels, Move Members, Connect and the relevant channel access.
The bot does not need to stream audio or join the voice channel itself.

POLLS AND COMMUNITY EVENTS
Use the panel or /poll question:TEXT answers:ONE|TWO|THREE channel:CHANNEL hours:24.
Native Discord polls accept 2–10 answers, support multiple selections, and close
through Discord even with the bot offline. The bot needs Send Polls in that channel.
Use the event form or /event to create a native Discord event with a location or
link and RSVP. Panel date inputs use your computer's timezone. Slash-command dates
need an explicit timezone: e.g. 2026-10-04T18:00:00+02:00. Choose an optional reminder
channel, role and minutes before start (0 disables). Native events remain visible
while offline; custom reminders need the bot running. Late reminders after the
scheduled start are skipped. Discord event edits/deletions update saved reminders
while online. The panel can cancel events created by this bot. Requires Create
Events and Manage Events, plus send/embed access in the announcement channel.

FAQ AND QUICK LINKS
Customize /mods, /shaders, /youtube and /support by editing their matching FAQ
entries. Up to 30 topics are supported, with answers up to 1800 characters. Other
topics use /faq topic:NAME. These answers are private command responses and support
Discord Markdown and HTTPS links without allowing mass mentions.

BACKUPS AND SETUP CHECKS
Download backup exports settings, warnings, delivery records, embeds, cases and
community bindings as JSON. The bot token is excluded. Keep the backup private: it
contains ticket subjects/details, moderation history and member IDs. Discord
channel contents and full message history are not copied into the backup.
To restore, stop the bot, select a backup, review its server and timestamp, and
click Restore selected backup. Saved data is replaced for the same server only;
the current bot token is kept. Published message IDs, role menus, tickets, votes and
event bindings are retained. If restoring on another PC, enter the bot token and
same server ID first. Missing/deleted Discord resources still need to be recreated or
reconfigured. Use Check community setup to identify missing permissions, invalid
roles, wrong channel types, stale menus and bot-trap channel conflicts.

RUNNING AND LIMITS
Most new features require this app and your PC online. Existing role messages,
cases and community bindings persist; native AutoMod blocks, Discord polls and
scheduled event listings run on Discord. This app cannot act on joins or trap
messages missed while it was offline, and a bot cannot make itself unbannable.
Closed tickets are retained privately. Limits: 100 open / 2,000 saved tickets,
2,500 submissions, 5,000 voters per submission, 50 simultaneous temporary rooms,
500 recorded events, 100 saved community panels, and 10,000 moderation cases.
The panel reports reached limits. Earlier warnings and upload records are retained.

EMBED MESSAGES & READ-THE-RULES ROLE (VERSION 1.2.0)
Open the "Embed messages" tab in the local control panel.
Choose "Use workshop rules" for a ready-to-edit rules message, or start blank.
Set a saved name, channel ID, title and description. The colored left border,
author/header, author icon, thumbnail, image, footer, links and extra fields
are customizable. Discord Markdown and Unicode emoji work in the text.
Images and links use public HTTPS URLs. The local preview approximates Discord's
formatting; the actual background follows each member's Discord theme.
"Save draft" keeps your work locally. It does NOT change a message already posted.
"Publish message" saves and posts it; "Update posted message" edits that same
message in place. Pick any saved message from the dropdown to edit it again.
Use "New message" or "Copy as new" to create additional embeds or post elsewhere.
Published messages remain tied to their original channel. If you delete one
manually in Discord, copy it as new to post a replacement.

Under Role action, choose:
- No role action: an ordinary embed.
- Button: a customizable label/color, including the red button in your screenshot.
- Emoji reaction: the bot adds your selected emoji; members react with the same one.
Choose the existing "read the rules" role. The bot needs Manage Roles, and that
role must be an unmanaged role below the bot's highest role.
Only the configured posted message/button/emoji can assign the configured role.
Membership screening must be completed before the role can be given.
Button confirmations are private. Removing an emoji does not remove the role.
The bot needs View Channel, Send Messages and Embed Links in the posting channel.
Emoji mode also needs Add Reactions and Read Message History. Custom emoji use
<:name:ID>, <a:name:ID> or name:ID; emoji from another server may need extra access.
Published message bindings survive app restarts. Emoji reactions added while
it was offline are checked on startup (up to 100 pages of 100 users per message).
Buttons need the bot online when clicked; an offline failed click must be retried.
Changing a saved draft's role does not affect the live message until published.
Already-granted roles stay assigned if you change or disable a role action.
There is no need to enable an additional privileged intent for emoji reactions.

BOT-TRAP CHANNEL (VERSION 1.2.0)
In Bot settings, enter the trap channel ID. Leave it empty to disable this feature.
Each new user or other bot message in that channel triggers immediate deletion
and a permanent ban attempt, even if its author has Manage Messages and even if
its text is harmless. Edited messages in that channel are also checked.
THIS BOT IS EXEMPT by its account ID and cannot be caught by its own trap.
The bot must be able to see the channel, and it needs Ban Members. Manage Messages
is needed to remove other users' offending messages. A deletion failure does not
suppress the ban attempt. Failed bans appear in Activity and the moderation log.
Discord still protects the server owner and members at/above the bot's role.
No bot can make itself immune to removal by the owner or higher authorized roles.
Webhook messages are deleted and logged; a webhook is not a bannable server member.
This feature handles messages while the bot is online and does not scan old chat
history or remove all of an author's old messages.


YOUTUBE NOTIFICATIONS
Enter the UC... channel ID (not your @handle). For your own channel, find it at
YouTube -> Settings -> Advanced settings: https://www.youtube.com/account_advanced .
An https://www.youtube.com/channel/UC... URL works too.
Enter your Discord announcement channel ID. The default interval is 120 seconds.
The bot uses the original public Atom feed. No YouTube API key, Google account,
or Google Cloud project is needed. Private/unlisted uploads normally do not appear
in the feed, but feed caching or later privacy changes cannot be reliably checked.
This version does not promise automatic removal of old video announcements.

The FIRST successful check remembers current feed entries silently. Subsequent
new entries are announced oldest first with title, thumbnail, and a video link.
IDs already delivered are saved and survive restarts; title edits do not trigger
another announcement. Delivery failures are retried. A deterministic Discord
nonce also prevents duplicate posts during its recent-message deduplication window.
Use "Post latest video as a test" or /youtube-test to test without pinging a role.
An optional notification role is pinged only for real new-video announcements.
Make that role mentionable. @everyone/@here and user mentions are disabled.
Feed refresh delays add to the configured interval. Shorts, live broadcasts,
and premieres may appear as feed entries; this version does not classify them.
YouTube exposes only a recent feed window (typically 15 entries), so a very long
offline period can miss uploads that have fallen out of that window.
Changing the YouTube or Discord announcement channel establishes a new quiet baseline.

MODERATION
/warn member reason              Save a persistent warning
/warnings member                 Show the latest 10 warnings and their IDs
/remove-warning member id        Remove one warning
/timeout member minutes reason   Timeout for 1 minute to 28 days
/untimeout member reason         Remove timeout
/kick member reason confirm      Kick; confirm must be true
/ban member reason confirm       Ban; keeps message history; confirm must be true
/unban user_id reason            Unban by Discord user ID
/purge count confirm             Delete 1–100 recent messages; confirm must be true
                                 Messages older than 14 days are skipped
/slowmode seconds                0 disables it; maximum 21600
/youtube-test                    Send latest video without a ping
/help, /ping                     Help and connectivity check

Commands are registered for the configured server and require the corresponding
Discord permissions. Warn/warnings/removal require Moderate Members. YouTube
tests require Manage Server. Commands respect role hierarchy. Server owners,
yourself, and this bot are protected from member moderation. Replies are private
to the invoking moderator. An optional log channel records moderation actions.

AUTOMOD
Optional spam deletion: by default the sixth message within 8 seconds, and
subsequent messages while over the threshold, are deleted. Earlier messages remain.
Discord invite deletion defaults to on; general HTTP/HTTPS/www link deletion
defaults to off. Moderators with Manage Messages and bot/webhook messages are exempt.
SLURS & SWEARING (VERSION 1.1.0)
Automatic slur bans are enabled by default, including when upgrading from 1.0.0.
The control panel contains an editable English/German starter list. Matches on
that list delete the message and attempt a permanent server ban. No old message
history is scanned. New messages and message edits are checked.
Slur checks also apply to moderators and to user messages in the log channel.
Bots/webhooks are ignored. Discord protects the owner and members whose highest
role is not below the bot's role; failed bans are recorded in the activity/log.
The bot needs Ban Members and must sit above the members it should ban. A failed
message deletion does not prevent it from attempting the ban. Ban does not
bulk-delete the member's message history.
Matching uses whole-word boundaries, ignoring case and Unicode width variants,
removing zero-width characters, and normalizing common numeric substitutions.
Words such as gay, queer and trans are not in the default ban list. Ordinary
profanity, including fuck, shit and cunt, is counted in the separate swear list.
Exact list matches apply to quotations too; this is a configurable word filter,
not a contextual hate-speech classifier. Punctuation spacing or unlisted
spellings can evade it. Add the variants you want to prohibit.
Defaults allow up to 3 ordinary swear words per message and 8 within 60 seconds.
Limits are editable. Exceeding either deletes the current message; swearing
alone never triggers a ban or timeout. Each member's rolling count spans all
channels. Editing a message updates its count instead of double-counting it.
Members with Manage Messages are exempt from swear limits and spam/link filters,
but are still checked for slurs. Language lists are limited to 200 entries each.
Other automod does not produce automatic bans, timeouts, or warning counts.

ONLINE UPDATES (NEW IN 1.4.0)
Download this updater-enabled executable once and replace your older executable.
After that, new bot versions are downloaded and installed online automatically.
Automatic installation is enabled by default: a check runs about 10 seconds after
startup and then every six hours while this app remains open. To check sooner,
use Bot updates -> Check for updates -> Install update & restart.
"Install bot updates automatically" in Bot settings turns scheduled installs off.
No GitHub login, token, paid service or extra update setup is required.

Downloads come only from MubbleYT/mubbles-workshop bot releases over HTTPS.
Every download must match its announced byte count and SHA-256 checksum and be a
Windows 64-bit executable. Bad or interrupted downloads leave your current bot
running. Keep the executable in a writable folder; a protected folder requires
moving it before updates can be installed. The updater does not request admin.

Once verified, a helper waits for the old process to exit, keeps the old binary
as MubbleDiscordBot.exe.previous beside the executable, replaces the file and
opens the new panel. A bot that was connected resumes after restart; this does
not change your saved "Start bot when this app opens" preference. The bot briefly
goes offline during installation. Settings, tokens, warnings, reaction roles,
embeds, moderation cases and community records stay in the same data folder.

If the new program cannot open its panel, the helper restores the previous
executable and starts it again. Automatic retry for that failed version pauses
until a newer release; manual retry is available in the panel. Errors appear in
Bot updates. Recovery backup: rename MubbleDiscordBot.exe.previous to an .exe
while the bot is closed if manual recovery is ever needed. Updates need internet;
an unreachable update channel leaves the existing bot running normally.

RELEASES FOR FUTURE DEVELOPMENT
Authoritative source: bot/source/ in https://github.com/MubbleYT/mubbles-workshop .
To publish a future update, edit source and bump bot/VERSION; push to main.
The Discord bot release workflow tests Linux and native Windows update/rollback,
builds the Windows executable, publishes a versioned bot-vX.Y.Z release, then
updates bot/update.json only after the executable and source package are online.
Existing release versions are never overwritten. No bot secrets belong in GitHub.

UPDATING FROM EARLIER VERSIONS
Close the old bot window, replace the EXE with the new one, and launch it.
Existing token, server settings, warning history and video history are retained.
Trap-channel enforcement is disabled until you select a channel. Embed drafts
and bindings are saved alongside your warnings and video history.
Language defaults are added if upgrading from 1.0.0; stop, edit, save, and restart
if you want to customize the lists or limits before moderation runs.

RUNNING LOCALLY
Your PC, internet connection, and this app must remain on for the bot to work.
You may close the browser panel; keep the console window open. Its URL lets
you reopen the panel. A fresh random local port is chosen each launch.
The app prevents a second instance from running with the same local settings.
Enable "Start bot when this app opens" to reconnect automatically on future launches.
To launch at Windows login, press Win+R, enter shell:startup, then put a shortcut
to MubbleDiscordBot.exe in that folder. This does not prevent PC sleep.

DATA
Settings: %APPDATA%\MubbleDiscordBot\config.json
Warnings and delivery history: %APPDATA%\MubbleDiscordBot\state.json
These persist when you move/replace the executable. Back up the folder if needed.
The bot token is stored in the local config file; do not share that folder/file.
The bot token goes only to Discord. The control
panel never returns it. Its API uses a random session key and listens on loopback.
The bot contacts Discord and YouTube directly; update downloads come from GitHub. There is no hosted backend.
No live Discord login or production-server test was performed during development;
you supply the bot token on your own PC. Source tests, panel checks and Windows build checks are run before release.

SOURCE & REBUILD
Source is included in source/. Use Go 1.27.1 or newer:
  go test ./...
  go build -buildvcs=false -trimpath -ldflags="-s -w" -o MubbleDiscordBot.exe .
For cross-compilation from Linux/macOS: set GOOS=windows GOARCH=amd64 CGO_ENABLED=0.
BUILD-WINDOWS.cmd is included in source/ for rebuilding on Windows.
THIRD-PARTY-LICENSES.txt contains dependency licenses.
