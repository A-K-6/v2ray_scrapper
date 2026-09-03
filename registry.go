package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// fileRegistry is the standalone replacement for Redis-backed
// subscription/site management. It persists to a small JSON file so
// `v2rays sources add` etc. work without Docker or Redis.
type fileRegistry struct {
	path          string
	Subscriptions []string     `json:"subscriptions"`
	Sites         []SiteConfig `json:"sites"`
	initialized   bool
}

func loadFileRegistry(path string, defaults []string, defaultSites []SiteConfig) (*fileRegistry, error) {
	r := &fileRegistry{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		r.Subscriptions = append([]string(nil), defaults...)
		r.Sites = append([]SiteConfig(nil), defaultSites...)
		r.initialized = true
		if err := r.save(); err != nil {
			return nil, err
		}
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	r.initialized = true
	// Merge in any new defaults (first-run seeds from env/config).
	r.Subscriptions = mergeStrings(r.Subscriptions, defaults)
	known := make(map[string]bool, len(r.Sites))
	for _, s := range r.Sites {
		known[s.URL] = true
	}
	for _, s := range defaultSites {
		if !known[s.URL] {
			r.Sites = append(r.Sites, s)
		}
	}
	sort.Strings(r.Subscriptions)
	sort.Slice(r.Sites, func(i, j int) bool { return r.Sites[i].URL < r.Sites[j].URL })
	return r, nil
}

func (r *fileRegistry) save() error {
	if r == nil || r.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".registry-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, r.path)
}

func (r *fileRegistry) addSubscriptions(urls []string) []string {
	r.Subscriptions = mergeStrings(r.Subscriptions, urls)
	_ = r.save()
	return append([]string(nil), r.Subscriptions...)
}

func (r *fileRegistry) removeSubscriptions(urls []string) []string {
	removed := make(map[string]bool, len(urls))
	for _, u := range urls {
		removed[u] = true
	}
	kept := r.Subscriptions[:0]
	for _, v := range r.Subscriptions {
		if !removed[v] {
			kept = append(kept, v)
		}
	}
	r.Subscriptions = kept
	_ = r.save()
	return append([]string(nil), r.Subscriptions...)
}

func (r *fileRegistry) putSite(site SiteConfig) {
	for i := range r.Sites {
		if r.Sites[i].URL == site.URL {
			r.Sites[i] = site
			_ = r.save()
			return
		}
	}
	r.Sites = append(r.Sites, site)
	_ = r.save()
}

func (r *fileRegistry) removeSite(target string) {
	kept := r.Sites[:0]
	for _, s := range r.Sites {
		if s.URL != target {
			kept = append(kept, s)
		}
	}
	r.Sites = kept
	_ = r.save()
}
