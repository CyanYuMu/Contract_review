package gateway

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"contract_review/app/internal/middleware/redis"

	"go.uber.org/zap"
)

// semanticCache 语义缓存：对 query 做 embedding，按功能维度在 Redis 中
// 存储缓存条目；查询时取全部条目做余弦比对，命中阈值则直接返回上次结果。
// 适用于低/中 QPS 场景；如需更高吞吐可升级为 Milvus 专用缓存 collection。
type semanticCache struct {
	redis      *redis.RedisClient
	embedder   Embedder
	threshold  float64
	ttl        time.Duration
	maxEntries int
}

type cacheEntry struct {
	ID       string    `json:"id"`
	Emb      []float32 `json:"emb"`
	Response string    `json:"resp"`
	Query    string    `json:"query"`
	Ts       int64     `json:"ts"`
}

func newSemanticCache(rc *redis.RedisClient, embedder Embedder, threshold float64, ttl time.Duration, maxEntries int) *semanticCache {
	return &semanticCache{redis: rc, embedder: embedder, threshold: threshold, ttl: ttl, maxEntries: maxEntries}
}

func (c *semanticCache) hashKey(feature string) string {
	return fmt.Sprintf("gw:semcache:%s", feature)
}

func cacheID(feature, query string) string {
	h := md5.Sum([]byte(feature + "\x00" + query))
	return hex.EncodeToString(h[:])
}

func (c *semanticCache) lookup(ctx context.Context, feature, query string) (string, bool) {
	if c == nil || c.embedder == nil || c.redis == nil {
		return "", false
	}
	client := c.redis.Client()
	if client == nil {
		return "", false
	}
	emb, err := c.embedder.Embed(query)
	if err != nil || len(emb) == 0 {
		return "", false
	}
	entries, err := c.allEntries(ctx, feature)
	if err != nil || len(entries) == 0 {
		return "", false
	}
	bestSim := 0.0
	bestResp := ""
	hit := false
	for _, e := range entries {
		s := cosine(emb, e.Emb)
		if s > bestSim {
			bestSim = s
			bestResp = e.Response
			hit = true
		}
	}
	if hit && bestSim >= c.threshold {
		zap.L().Debug("语义缓存命中", zap.Float64("sim", bestSim), zap.String("feature", feature))
		return bestResp, true
	}
	return "", false
}

func (c *semanticCache) store(ctx context.Context, feature, query, response string) {
	if c == nil || c.embedder == nil || c.redis == nil || response == "" {
		return
	}
	client := c.redis.Client()
	if client == nil {
		return
	}
	emb, err := c.embedder.Embed(query)
	if err != nil || len(emb) == 0 {
		return
	}
	id := cacheID(feature, query)
	entry := cacheEntry{ID: id, Emb: emb, Response: response, Query: query, Ts: time.Now().Unix()}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	key := c.hashKey(feature)
	if err := client.HSet(ctx, key, id, data).Err(); err != nil {
		zap.L().Warn("语义缓存写入失败", zap.Error(err))
		return
	}
	_ = client.Expire(ctx, key, c.ttl).Err()
	c.evict(ctx, feature)
}

func (c *semanticCache) allEntries(ctx context.Context, feature string) ([]cacheEntry, error) {
	client := c.redis.Client()
	hm, err := client.HGetAll(ctx, c.hashKey(feature)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]cacheEntry, 0, len(hm))
	for _, raw := range hm {
		var e cacheEntry
		if json.Unmarshal([]byte(raw), &e) == nil && len(e.Emb) > 0 {
			out = append(out, e)
		}
	}
	return out, nil
}

func (c *semanticCache) evict(ctx context.Context, feature string) {
	client := c.redis.Client()
	hm, err := client.HGetAll(ctx, c.hashKey(feature)).Result()
	if err != nil || len(hm) <= c.maxEntries {
		return
	}
	entries := make([]cacheEntry, 0, len(hm))
	for _, raw := range hm {
		var e cacheEntry
		if json.Unmarshal([]byte(raw), &e) == nil {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Ts < entries[j].Ts })
	toRemove := len(entries) - c.maxEntries
	for i := 0; i < toRemove && i < len(entries); i++ {
		_ = client.HDel(ctx, c.hashKey(feature), entries[i].ID).Err()
	}
}

func cosine(a, b []float32) float64 {
	n := len(a)
	if n == 0 || n != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
