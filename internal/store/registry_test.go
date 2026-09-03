package store

import (
	"path/filepath"
	"testing"

	"github.com/aeen/v2ray-scrapper/internal/config"
)

func TestFileRegistryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	enabled := true
	r, err := LoadRegistry(path, []string{"https://a.example/sub"}, []config.SiteConfig{{URL: "https://www.google.com", Filename: "google.txt", Enabled: &enabled}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Subscriptions) != 1 || len(r.Sites) != 1 {
		t.Fatalf("seed=%#v", r)
	}
	r.AddSubscriptions([]string{"https://b.example/sub", "https://a.example/sub"})
	if len(r.Subscriptions) != 2 {
		t.Fatalf("dedup=%v", r.Subscriptions)
	}
	r.RemoveSubscriptions([]string{"https://a.example/sub"})
	if len(r.Subscriptions) != 1 || r.Subscriptions[0] != "https://b.example/sub" {
		t.Fatalf("remove=%v", r.Subscriptions)
	}
	// Reload persists.
	r2, err := LoadRegistry(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Subscriptions) != 1 || len(r2.Sites) != 1 {
		t.Fatalf("reload=%#v", r2)
	}
	r2.RemoveSite("https://www.google.com")
	if len(r2.Sites) != 0 {
		t.Fatalf("site remove=%#v", r2.Sites)
	}
}

func TestRegistryPathRespectsOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv("V2RAYS_REGISTRY_PATH", custom)
	if got := RegistryPath(); got != custom {
		t.Fatalf("path=%q", got)
	}
}
