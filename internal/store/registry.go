package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/aeen/v2ray-scrapper/internal/config"
	"github.com/aeen/v2ray-scrapper/internal/xdg"
)

// Registry is the standalone replacement for Redis-backed
// subscription/site management. It persists to a small JSON file so
// `v2rays sources add` etc. work without Docker or Redis.
type Registry struct {
	path          string
	Subscriptions []string            `json:"subscriptions"`
	Sites         []config.SiteConfig `json:"sites"`
	initialized   bool
}

// RegistryPath returns the local registry location.
func RegistryPath() string {
	if v := os.Getenv("V2RAYS_REGISTRY_PATH"); v != "" {
		return v
	}
	// Keep repo-local behaviour for docker-compose checkouts.
	if _, err := os.Stat("data"); err == nil {
		return filepath.Join("data", "registry.json")
	}
	return filepath.Join(xdg.DataDir(), "registry.json")
}

// MergeStrings unions string groups preserving first-seen order, then sorts.
func MergeStrings(groups ...[]string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, group := range groups {
		for _, value := range group {
			if !seen[value] {
				seen[value] = true
				result = append(result, value)
			}
		}
	}
	sort.Strings(result)
	return result
}

// LoadRegistry opens (or seeds) the file registry.
func LoadRegistry(path string, defaults []string, defaultSites []config.SiteConfig) (*Registry, error) {
	r := &Registry{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		r.Subscriptions = append([]string(nil), defaults...)
		r.Sites = append([]config.SiteConfig(nil), defaultSites...)
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
	r.Subscriptions = MergeStrings(r.Subscriptions, defaults)
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

func (r *Registry) save() error {
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

// AddSubscriptions unions urls into the registry and persists.
func (r *Registry) AddSubscriptions(urls []string) []string {
	r.Subscriptions = MergeStrings(r.Subscriptions, urls)
	_ = r.save()
	return append([]string(nil), r.Subscriptions...)
}

// RemoveSubscriptions drops urls from the registry and persists.
func (r *Registry) RemoveSubscriptions(urls []string) []string {
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

// PutSite adds or replaces a preloaded site check.
func (r *Registry) PutSite(site config.SiteConfig) {
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

// RemoveSite drops a preloaded site check.
func (r *Registry) RemoveSite(target string) {
	kept := r.Sites[:0]
	for _, s := range r.Sites {
		if s.URL != target {
			kept = append(kept, s)
		}
	}
	r.Sites = kept
	_ = r.save()
}
