package store

import (
	"context"
	"testing"

	"github.com/aeen/v2ray-scrapper/internal/proxy"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisSiteCacheRoundTripAndTTL(t *testing.T) {
	server := miniredis.RunT(t)
	store, err := NewRedisStore(context.Background(), "redis://"+server.Addr(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	servers := []proxy.ProxyServer{{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: "uuid"}}
	if err := store.SetSiteCache(context.Background(), "https://example.com", servers, time.Minute); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.GetSiteCache(context.Background(), "https://example.com")
	if err != nil || !ok || len(loaded) != 1 || loaded[0].Address != "1.2.3.4" {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
	server.FastForward(time.Minute + time.Second)
	if _, ok, err := store.GetSiteCache(context.Background(), "https://example.com"); err != nil || ok {
		t.Fatalf("expired cache ok=%v err=%v", ok, err)
	}
}
