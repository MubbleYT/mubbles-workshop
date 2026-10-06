package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Warning struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	ModeratorID string    `json:"moderator_id"`
	Reason      string    `json:"reason"`
	Created     time.Time `json:"created"`
}
type FeedState struct {
	Seen        []string `json:"seen"`
	Initialized bool     `json:"initialized"`
}
type SavedState struct {
	Warnings    map[string][]Warning      `json:"warnings"`
	Feeds       map[string]FeedState      `json:"feeds"`
	NextWarning uint64                    `json:"next_warning"`
	Embeds      map[string]EmbedPost      `json:"embeds"`
	Community   map[string]CommunityState `json:"community"`
}
type Store struct {
	mu   sync.Mutex
	path string
	data SavedState
}

func openStore(path string) (*Store, error) {
	s := &Store{path: path, data: SavedState{Warnings: map[string][]Warning{}, Feeds: map[string]FeedState{}, Embeds: map[string]EmbedPost{}}}
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(b, &s.data); e != nil {
		return nil, e
	}
	if s.data.Warnings == nil {
		s.data.Warnings = map[string][]Warning{}
	}
	if s.data.Feeds == nil {
		s.data.Feeds = map[string]FeedState{}
	}
	if s.data.Embeds == nil {
		s.data.Embeds = map[string]EmbedPost{}
	}
	return s, nil
}
func atomicJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".mubble-*.tmp")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, path)
}
func (s *Store) warnings(g, u string) []Warning {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Warning{}, s.data.Warnings[g+":"+u]...)
}
func (s *Store) warn(g, u, m, reason string) (Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := g + ":" + u
	s.data.NextWarning++
	w := Warning{fmt.Sprint(s.data.NextWarning), u, m, reason, time.Now().UTC()}
	old := s.data.Warnings[key]
	s.data.Warnings[key] = append(append([]Warning{}, old...), w)
	if e := atomicJSON(s.path, s.data); e != nil {
		s.data.Warnings[key] = old
		s.data.NextWarning--
		return Warning{}, e
	}
	return w, nil
}
func (s *Store) removeWarning(g, u, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := g + ":" + u
	old := s.data.Warnings[key]
	out := []Warning{}
	found := false
	for _, w := range old {
		if w.ID == id {
			found = true
		} else {
			out = append(out, w)
		}
	}
	if !found {
		return fmt.Errorf("Warning #%s was not found for this member", id)
	}
	s.data.Warnings[key] = out
	if e := atomicJSON(s.path, s.data); e != nil {
		s.data.Warnings[key] = old
		return e
	}
	return nil
}
func (s *Store) feed(key string) FeedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.data.Feeds[key]
	f.Seen = append([]string{}, f.Seen...)
	return f
}
func (s *Store) saveFeed(key string, f FeedState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.data.Feeds[key]
	if len(f.Seen) > 300 {
		f.Seen = f.Seen[len(f.Seen)-300:]
	}
	s.data.Feeds[key] = f
	if e := atomicJSON(s.path, s.data); e != nil {
		s.data.Feeds[key] = old
		return e
	}
	return nil
}
