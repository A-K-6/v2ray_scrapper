package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
	prefix string
}

type redisSiteCache struct {
	Servers []ProxyServer `json:"servers"`
}

func NewRedisStore(ctx context.Context, rawURL, prefix string) (*RedisStore, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, nil
	}
	var options *redis.Options
	var err error
	if strings.Contains(rawURL, "://") {
		options, err = redis.ParseURL(rawURL)
	} else {
		options = &redis.Options{Addr: rawURL}
	}
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	store := &RedisStore{client: redis.NewClient(options), prefix: strings.TrimSuffix(prefix, ":")}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := store.client.Ping(pingCtx).Err(); err != nil {
		_ = store.client.Close()
		return nil, fmt.Errorf("connect to Redis: %w", err)
	}
	return store, nil
}

func (s *RedisStore) Close() error {
	if s == nil {
		return nil
	}
	return s.client.Close()
}

func (s *RedisStore) key(suffix string) string { return s.prefix + ":" + suffix }

func (s *RedisStore) SeedAndLoad(ctx context.Context, subscriptions []string, sites []SiteConfig) ([]string, []SiteConfig, error) {
	if s == nil {
		return append([]string(nil), subscriptions...), append([]SiteConfig(nil), sites...), nil
	}
	initialized, err := s.client.Exists(ctx, s.key("registry-initialized")).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("initialize Redis registry: %w", err)
	}
	if initialized == 0 {
		pipe := s.client.Pipeline()
		if len(subscriptions) > 0 {
			values := make([]any, len(subscriptions))
			for i := range subscriptions {
				values[i] = subscriptions[i]
			}
			pipe.SAdd(ctx, s.key("subscriptions"), values...)
		}
		for _, site := range sites {
			data, err := json.Marshal(site)
			if err != nil {
				return nil, nil, err
			}
			pipe.HSet(ctx, s.key("sites"), site.URL, data)
		}
		pipe.Set(ctx, s.key("registry-initialized"), time.Now().UTC().Format(time.RFC3339), 0)
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, nil, fmt.Errorf("seed Redis registry: %w", err)
		}
	}
	loadedSubscriptions, err := s.client.SMembers(ctx, s.key("subscriptions")).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("load Redis subscriptions: %w", err)
	}
	storedSites, err := s.client.HGetAll(ctx, s.key("sites")).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("load Redis sites: %w", err)
	}
	loadedSites := make([]SiteConfig, 0, len(storedSites))
	for _, data := range storedSites {
		var site SiteConfig
		if json.Unmarshal([]byte(data), &site) == nil && site.URL != "" {
			loadedSites = append(loadedSites, site)
		}
	}
	sort.Strings(loadedSubscriptions)
	sort.Slice(loadedSites, func(i, j int) bool { return loadedSites[i].URL < loadedSites[j].URL })
	return loadedSubscriptions, loadedSites, nil
}

func (s *RedisStore) AddSubscriptions(ctx context.Context, urls []string) error {
	if s == nil {
		return fmt.Errorf("Redis is not configured")
	}
	values := make([]any, len(urls))
	for i := range urls {
		values[i] = urls[i]
	}
	return s.client.SAdd(ctx, s.key("subscriptions"), values...).Err()
}

func (s *RedisStore) RemoveSubscriptions(ctx context.Context, urls []string) error {
	if s == nil {
		return fmt.Errorf("Redis is not configured")
	}
	values := make([]any, len(urls))
	for i := range urls {
		values[i] = urls[i]
	}
	return s.client.SRem(ctx, s.key("subscriptions"), values...).Err()
}

func (s *RedisStore) PutSite(ctx context.Context, site SiteConfig) error {
	if s == nil {
		return fmt.Errorf("Redis is not configured")
	}
	data, err := json.Marshal(site)
	if err != nil {
		return err
	}
	return s.client.HSet(ctx, s.key("sites"), site.URL, data).Err()
}

func (s *RedisStore) RemoveSite(ctx context.Context, target string) error {
	if s == nil {
		return fmt.Errorf("Redis is not configured")
	}
	pipe := s.client.Pipeline()
	pipe.HDel(ctx, s.key("sites"), target)
	pipe.Del(ctx, s.siteCacheKey(target))
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisStore) GetSiteCache(ctx context.Context, target string) ([]ProxyServer, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	data, err := s.client.Get(ctx, s.siteCacheKey(target)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var cached redisSiteCache
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false, err
	}
	return cached.Servers, true, nil
}

func (s *RedisStore) SetSiteCache(ctx context.Context, target string, servers []ProxyServer, ttl time.Duration) error {
	if s == nil {
		return nil
	}
	data, err := json.Marshal(redisSiteCache{Servers: servers})
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.siteCacheKey(target), data, ttl).Err()
}

func (s *RedisStore) siteCacheKey(target string) string {
	digest := sha256.Sum256([]byte(target))
	return s.key("site-cache:" + hex.EncodeToString(digest[:]))
}
