package hotspotperception

import "context"

type Perception interface {
	Get(ctx context.Context, key string) int64
	Mark(ctx context.Context, key string, count int64)
	GetTopHotspots(ctx context.Context, n int) []HotspotItem
}

// HotspotItem 热点项
type HotspotItem struct {
	Key   string
	Count int64
}
