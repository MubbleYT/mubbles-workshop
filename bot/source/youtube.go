package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
)

var youtubeIDRE = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
var videoIDRE = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

type Video struct {
	ID        string    `xml:"videoId"`
	Title     string    `xml:"title"`
	Published time.Time `xml:"published"`
	Author    struct {
		Name string `xml:"name"`
	} `xml:"author"`
}
type VideoFeed struct {
	XMLName xml.Name `xml:"feed"`
	Videos  []Video  `xml:"entry"`
}

func parseFeed(reader io.Reader) ([]Video, error) {
	var f VideoFeed
	if e := xml.NewDecoder(io.LimitReader(reader, 2<<20)).Decode(&f); e != nil {
		return nil, e
	}
	for _, v := range f.Videos {
		if !videoIDRE.MatchString(v.ID) || v.Published.IsZero() {
			return nil, errors.New("YouTube returned an invalid video entry")
		}
	}
	sort.SliceStable(f.Videos, func(i, j int) bool { return f.Videos[i].Published.Before(f.Videos[j].Published) })
	return f.Videos, nil
}
func (a *App) fetchVideos(ctx context.Context, id string) ([]Video, error) {
	if !youtubeIDRE.MatchString(id) {
		return nil, errors.New("Set your YouTube channel ID in settings first")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://www.youtube.com/feeds/videos.xml?channel_id="+id, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "MubbleDiscordBot/"+version)
	resp, e := a.httpClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("YouTube feed returned HTTP %d. Check the channel ID; a temporary feed failure will be retried", resp.StatusCode)
	}
	return parseFeed(resp.Body)
}
func videoPayload(c Config, v Video, test bool) map[string]any {
	content := c.NotifyText
	mentions := &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	if test {
		content = "[TEST — no ping] " + content
	} else if c.NotifyRoleID != "" {
		content = "<@&" + c.NotifyRoleID + "> " + content
		mentions.Roles = []string{c.NotifyRoleID}
	}
	url := "https://www.youtube.com/watch?v=" + v.ID
	embed := &discordgo.MessageEmbed{Title: clip(v.Title, 256), URL: url, Color: 0x9564ff, Author: &discordgo.MessageEmbedAuthor{Name: clip(v.Author.Name, 256)}, Image: &discordgo.MessageEmbedImage{URL: "https://i.ytimg.com/vi/" + v.ID + "/hqdefault.jpg"}, Timestamp: v.Published.UTC().Format(time.RFC3339), Footer: &discordgo.MessageEmbedFooter{Text: "New YouTube upload"}}
	p := map[string]any{"content": content + "\n" + url, "embeds": []*discordgo.MessageEmbed{embed}, "allowed_mentions": mentions}
	if !test {
		h := sha256.Sum256([]byte(c.NotifyChannelID + ":" + v.ID))
		p["nonce"] = hex.EncodeToString(h[:12])
		p["enforce_nonce"] = true
	}
	return p
}
func (a *App) postVideo(ctx context.Context, s *discordgo.Session, c Config, v Video, test bool) error {
	ch, e := s.Channel(c.NotifyChannelID, discordgo.WithContext(ctx))
	if e != nil {
		return e
	}
	if ch.GuildID != c.GuildID {
		return errors.New("Notification channel must belong to the configured server")
	}
	_, e = s.RequestWithBucketID("POST", discordgo.EndpointChannelMessages(c.NotifyChannelID), videoPayload(c, v, test), discordgo.EndpointChannelMessages(c.NotifyChannelID), discordgo.WithContext(ctx))
	return e
}
func (a *App) testVideo(s *discordgo.Session) error {
	c := a.config()
	if c.NotifyChannelID == "" {
		return errors.New("Set a YouTube channel and announcement channel first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	videos, e := a.fetchVideos(ctx, c.YouTubeID)
	if e != nil {
		return e
	}
	if len(videos) == 0 {
		return errors.New("No public videos found in this channel feed")
	}
	if e = a.postVideo(ctx, s, c, videos[len(videos)-1], true); e != nil {
		return e
	}
	a.log("Posted a test video notification without a ping")
	return nil
}
func planVideos(f FeedState, videos []Video) (FeedState, []Video) {
	if !f.Initialized {
		f.Initialized = true
		for _, v := range videos {
			f.Seen = append(f.Seen, v.ID)
		}
		return f, nil
	}
	seen := map[string]bool{}
	for _, id := range f.Seen {
		seen[id] = true
	}
	out := []Video{}
	for _, v := range videos {
		if !seen[v.ID] {
			out = append(out, v)
			seen[v.ID] = true
		}
	}
	return f, out
}
func (a *App) youtubeLoop(ctx context.Context, s *discordgo.Session) {
	for {
		if ctx.Err() != nil {
			return
		}
		c := a.config()
		if c.YouTubeID != "" && c.NotifyChannelID != "" {
			if active, e := a.current(); e == nil && active == s {
				if e = a.checkVideos(ctx, s, c); e != nil && ctx.Err() == nil {
					a.log("Upload check: " + a.errorText(e))
				}
			}
		}
		timer := time.NewTimer(time.Duration(c.PollSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func (a *App) checkVideos(ctx context.Context, s *discordgo.Session, c Config) error {
	videos, e := a.fetchVideos(ctx, c.YouTubeID)
	if e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	a.mu.Lock()
	a.lastCheck = time.Now().Format("2006-01-02 15:04:05")
	a.mu.Unlock()
	key := c.GuildID + ":" + c.YouTubeID + ":" + c.NotifyChannelID
	old := a.store.feed(key)
	f, pending := planVideos(old, videos)
	if !old.Initialized {
		if e = a.store.saveFeed(key, f); e != nil {
			return e
		}
		a.log("Video watcher initialized; existing uploads will not be announced")
		return nil
	}
	for _, v := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e = a.postVideo(ctx, s, c, v, false); e != nil {
			return e
		}
		f.Seen = append(f.Seen, v.ID)
		if e = a.store.saveFeed(key, f); e != nil {
			return fmt.Errorf("Video posted but its delivery record could not be saved: %w", e)
		}
		a.mu.Lock()
		a.lastNotify = time.Now().Format("2006-01-02 15:04:05")
		a.mu.Unlock()
		a.log("Announced video: " + v.Title)
	}
	return nil
}
