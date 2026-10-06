package main

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/unicode/norm"
)

// Explicit terms only. Ordinary profanity and identity labels are separate.
const defaultSlurs = "nigger\nniggers\nnigga\nniggas\nfaggot\nfaggots\nkike\nkikes\nwetback\nwetbacks\ntranny\ntrannies\nschwuchtel\nschwuchteln\njudensau\nkanake\nkanaken"
const defaultSwears = "fuck\nfucking\nfucked\nfucker\nfuckers\nmotherfucker\nmotherfuckers\nshit\nshitty\nshitting\nbullshit\nbitch\nbitches\nbastard\nbastards\nass\narse\nasshole\nassholes\ncunt\ncunts\ndick\ndicks\ncock\ncocks\npiss\npissed\nscheisse\nscheiße\nbeschissen\nverdammt\nverfickt\narsch\narschloch\nwichser\nhurensohn"

type WordMatcher struct{ pattern *regexp.Regexp }
type LanguagePolicy struct {
	slurSource  string
	swearSource string
	slurs       WordMatcher
	swears      WordMatcher
}
type SwearUse struct {
	ID    string
	Count int
	At    time.Time
}

func normalizeLanguage(text string) string {
	text = strings.ToLower(norm.NFKC.String(text))
	var b strings.Builder
	for _, r := range text {
		if unicode.Is(unicode.Cf, r) {
			continue
		}
		switch r {
		case '0':
			r = 'o'
		case '1':
			r = 'i'
		case '3':
			r = 'e'
		case '4':
			r = 'a'
		case '5':
			r = 's'
		case '7':
			r = 't'
		}
		b.WriteRune(r)
	}
	return b.String()
}
func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) }
func newWordMatcher(list string) (WordMatcher, error) {
	if len(list) > 12000 {
		return WordMatcher{}, errors.New("Each language list must be under 12000 bytes")
	}
	lines := strings.FieldsFunc(list, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
	if len(lines) > 200 {
		return WordMatcher{}, errors.New("Each language list can have at most 200 entries")
	}
	patterns := []string{}
	seen := map[string]bool{}
	for _, line := range lines {
		term := normalizeLanguage(strings.TrimSpace(line))
		term = strings.Join(strings.Fields(term), " ")
		if term == "" {
			continue
		}
		if len([]rune(term)) < 3 || len([]rune(term)) > 64 {
			return WordMatcher{}, errors.New("Blocked terms must have 3–64 characters")
		}
		for _, r := range term {
			if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != ' ' && r != '\'' {
				return WordMatcher{}, errors.New("Language list entries must be words or phrases, one per line")
			}
		}
		if !seen[term] {
			seen[term] = true
			patterns = append(patterns, regexp.QuoteMeta(term))
		}
	}
	if len(patterns) == 0 {
		return WordMatcher{}, nil
	}
	re, e := regexp.Compile("(?:" + strings.Join(patterns, "|") + ")")
	if e == nil {
		re.Longest()
	}
	return WordMatcher{re}, e
}
func (m WordMatcher) count(text string) int {
	if m.pattern == nil {
		return 0
	}
	text = normalizeLanguage(text)
	n := 0
	for _, span := range m.pattern.FindAllStringIndex(text, -1) {
		if span[0] > 0 {
			r, _ := utf8.DecodeLastRuneInString(text[:span[0]])
			if wordRune(r) {
				continue
			}
		}
		if span[1] < len(text) {
			r, _ := utf8.DecodeRuneInString(text[span[1]:])
			if wordRune(r) {
				continue
			}
		}
		n++
	}
	return n
}
func buildLanguagePolicy(c Config) (*LanguagePolicy, error) {
	slurs, e := newWordMatcher(c.SlurTerms)
	if e != nil {
		return nil, e
	}
	swears, e := newWordMatcher(c.SwearTerms)
	if e != nil {
		return nil, e
	}
	if c.BanSlurs && slurs.pattern == nil {
		return nil, errors.New("Add at least one blocked slur, or disable automatic slur bans")
	}
	if c.LimitSwearing && swears.pattern == nil {
		return nil, errors.New("Add at least one swear word, or disable the swearing limit")
	}
	return &LanguagePolicy{c.SlurTerms, c.SwearTerms, slurs, swears}, nil
}
func (a *App) languagePolicy(c Config) (*LanguagePolicy, error) {
	a.languageMu.Lock()
	defer a.languageMu.Unlock()
	if a.language != nil && a.language.slurSource == c.SlurTerms && a.language.swearSource == c.SwearTerms {
		return a.language, nil
	}
	p, e := buildLanguagePolicy(c)
	if e == nil {
		a.language = p
	}
	return p, e
}
func (a *App) recordSwears(c Config, user, id string, count int, now time.Time) int {
	a.swearMu.Lock()
	defer a.swearMu.Unlock()
	if a.swears == nil {
		a.swears = map[string][]SwearUse{}
	}
	key := c.GuildID + ":" + user
	out := []SwearUse{}
	found := false
	total := 0
	for _, use := range a.swears[key] {
		if now.Sub(use.At) >= time.Duration(c.SwearWindow)*time.Second {
			continue
		}
		if use.ID == id {
			use.Count = count
			found = true
		}
		if use.Count > 0 {
			out = append(out, use)
			total += use.Count
		}
	}
	if !found && count > 0 {
		out = append(out, SwearUse{id, count, now})
		total += count
	}
	if len(out) == 0 {
		delete(a.swears, key)
	} else {
		a.swears[key] = out
	}
	if len(a.swears) > 10000 {
		for k, v := range a.swears {
			if len(v) == 0 || now.Sub(v[len(v)-1].At) > time.Duration(c.SwearWindow)*time.Second {
				delete(a.swears, k)
			}
		}
	}
	return total
}
func (a *App) banForSlur(s *discordgo.Session, m *discordgo.MessageCreate) {
	a.banForPolicy(s, m, "prohibited slur language")
}
func (a *App) banForPolicy(s *discordgo.Session, m *discordgo.MessageCreate, policy string) {
	if m.Author == nil || (s.State.User != nil && m.Author.ID == s.State.User.ID) {
		return
	}
	var deletionErr error
	if m.ID != "" && m.ChannelID != "" {
		deletionErr = s.ChannelMessageDelete(m.ChannelID, m.ID)
	}
	if deletionErr != nil {
		a.log("Automatic-ban message could not be deleted: " + a.errorText(deletionErr))
	}
	// Gate overlapping events, but still remove each offending message.
	key := m.GuildID + ":" + m.Author.ID
	now := time.Now()
	a.banMu.Lock()
	if a.banLast == nil {
		a.banLast = map[string]time.Time{}
	}
	if t, ok := a.banLast[key]; ok && now.Sub(t) < 30*time.Second {
		a.banMu.Unlock()
		return
	}
	a.banLast[key] = now
	if len(a.banLast) > 10000 {
		for k, t := range a.banLast {
			if now.Sub(t) > time.Minute {
				delete(a.banLast, k)
			}
		}
	}
	a.banMu.Unlock()
	reason := "Automatic ban: " + policy
	if e := s.GuildBanCreateWithReason(m.GuildID, m.Author.ID, reason, 0); e != nil {
		a.audit(s, "Auto-ban FAILED for "+m.Author.ID+" ("+policy+") in <#"+m.ChannelID+">: "+a.errorText(e)+". Check Ban Members permission and role hierarchy.")
		return
	}
	a.recordCase(s, "ban", s.State.User.ID, m.Author.ID, m.ChannelID, reason+"; message "+m.ID, "", nil)
}
