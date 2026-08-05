package rag

import (
	"crypto/md5"
	"fmt"
	"sync"
)

// EmbeddingCache 基于内容的 Embedding 缓存
// 在同一 session 内，相同文本不会重复调用 Embedding API
// 线程安全
type EmbeddingCache struct {
	cache map[string][]float32
	mu    sync.RWMutex
}

// NewEmbeddingCache 创建 Embedding 缓存
func NewEmbeddingCache() *EmbeddingCache {
	return &EmbeddingCache{
		cache: make(map[string][]float32),
	}
}

// Get 获取缓存的 embedding，如果不存在返回 nil
func (ec *EmbeddingCache) Get(key string) ([]float32, bool) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	emb, ok := ec.cache[key]
	if !ok {
		return nil, false
	}
	// 返回副本，防止外部修改
	cp := make([]float32, len(emb))
	copy(cp, emb)
	return cp, true
}

// Set 设置缓存
func (ec *EmbeddingCache) Set(key string, embedding []float32) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	cp := make([]float32, len(embedding))
	copy(cp, embedding)
	ec.cache[key] = cp
}

// Clear 清空缓存
func (ec *EmbeddingCache) Clear() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.cache = make(map[string][]float32)
}

// Size 返回缓存大小
func (ec *EmbeddingCache) Size() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.cache)
}

// ContentHash 计算文本内容的 hash（用于缓存 key）
func ContentHash(text string) string {
	h := md5.Sum([]byte(text))
	return fmt.Sprintf("%x", h[:8])
}

// CachedEmbedder 带缓存的 Embedding 模型包装
type CachedEmbedder struct {
	embedder EmbeddingModel
	cache    *EmbeddingCache
}

// NewCachedEmbedder 创建带缓存的 Embedder
func NewCachedEmbedder(embedder EmbeddingModel, cache *EmbeddingCache) *CachedEmbedder {
	return &CachedEmbedder{
		embedder: embedder,
		cache:    cache,
	}
}

// EmbedWithCache 带缓存的单文本 Embedding
func (ce *CachedEmbedder) EmbedWithCache(text string) ([]float32, error) {
	key := ContentHash(text)
	if emb, ok := ce.cache.Get(key); ok {
		return emb, nil
	}

	emb, err := ce.embedder.Embed(text)
	if err != nil {
		return nil, err
	}

	ce.cache.Set(key, emb)
	return emb, nil
}

// Embed 实现 EmbeddingModel 接口（无缓存）
func (ce *CachedEmbedder) Embed(text string) ([]float32, error) {
	return ce.EmbedWithCache(text)
}

// EmbedBatch 批量 Embedding（带缓存）
func (ce *CachedEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	var uncachedTexts []string
	var uncachedIndices []int

	// 先查缓存
	for i, text := range texts {
		key := ContentHash(text)
		if emb, ok := ce.cache.Get(key); ok {
			results[i] = emb
		} else {
			uncachedTexts = append(uncachedTexts, text)
			uncachedIndices = append(uncachedIndices, i)
		}
	}

	if len(uncachedTexts) == 0 {
		return results, nil
	}

	// 批量计算未缓存的
	embeddings, err := ce.embedder.EmbedBatch(uncachedTexts)
	if err != nil {
		return nil, err
	}

	for j, idx := range uncachedIndices {
		if j < len(embeddings) {
			results[idx] = embeddings[j]
			// 写入缓存
			key := ContentHash(uncachedTexts[j])
			ce.cache.Set(key, embeddings[j])
		}
	}

	return results, nil
}

// ClearCache 清空缓存
func (ce *CachedEmbedder) ClearCache() {
	ce.cache.Clear()
}
