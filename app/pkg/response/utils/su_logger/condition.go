package su_logger

import (
	"runtime"
	"time"

	"go.uber.org/zap"
)

type Condition interface {
	ShouldLog() (should bool, field *zap.Field)
}

type TimeElapsedCondition struct {
	Interval time.Duration
	lastTime time.Time
}

func (t *TimeElapsedCondition) ShouldLog() (should bool, field *zap.Field) {
	if t == nil {
		return false, nil
	}
	now := time.Now()
	itv := now.Sub(t.lastTime)
	if itv >= t.Interval {
		t.lastTime = now
		f := zap.String("time_elapsed", itv.String())
		return true, &f
	}
	return false, nil
}

type MemoryUsageCondition struct {
	MaxMemoryUsage uint64
}

func (m *MemoryUsageCondition) ShouldLog() (should bool, field *zap.Field) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	if memStats.Alloc >= m.MaxMemoryUsage {
		f := zap.Uint64("memory_usage", memStats.Alloc)
		return true, &f
	} else {
		return false, nil
	}
}

func NewTimeElapsedCondition(interval time.Duration) *TimeElapsedCondition {
	return &TimeElapsedCondition{
		Interval: interval,
		lastTime: time.Now(),
	}
}

func NewMemoryUsageCondition(maxByte int64) *MemoryUsageCondition {
	return &MemoryUsageCondition{
		MaxMemoryUsage: uint64(maxByte),
	}
}
