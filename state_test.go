package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateStoreRoundTrip(t *testing.T) {
	store := NewStateStore(filepath.Join(t.TempDir(), "nested", "state.json"))
	want := PersistentState{Working: []ProxyServer{{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: "uuid"}}, Candidates: []ProxyServer{}, UpdatedAt: time.Unix(456, 0)}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Working) != 1 || got.Working[0].Address != "1.2.3.4" {
		t.Fatalf("unexpected state: %#v", got)
	}
}

func TestStateStoreRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStateStore(path).Load(); err == nil {
		t.Fatal("expected decode error")
	}
}
