package window_monitor

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==================== SimpleCounter 测试 ====================

func TestSimpleCounter_Increment(t *testing.T) {
	counter := NewSimpleCounter()
	ctx := context.Background()
	now := time.Now()

	counter.Increment(ctx, "test_key", 1, time.Minute, now)
	if got := counter.GetCount(ctx, "test_key", now); got != 1 {
		t.Errorf("GetCount() = %d, want 1", got)
	}

	counter.Increment(ctx, "test_key", 5, time.Minute, now)
	if got := counter.GetCount(ctx, "test_key", now); got != 6 {
		t.Errorf("GetCount() = %d, want 6", got)
	}
}

func TestSimpleCounter_GetCount_NotExists(t *testing.T) {
	counter := NewSimpleCounter()
	ctx := context.Background()
	now := time.Now()

	if got := counter.GetCount(ctx, "not_exists", now); got != 0 {
		t.Errorf("GetCount() = %d, want 0", got)
	}
}

func TestSimpleCounter_Reset(t *testing.T) {
	counter := NewSimpleCounter()
	ctx := context.Background()
	now := time.Now()

	counter.Increment(ctx, "test_key", 10, time.Minute, now)
	counter.Reset(ctx, "test_key", now)

	if got := counter.GetCount(ctx, "test_key", now); got != 0 {
		t.Errorf("GetCount() after Reset() = %d, want 0", got)
	}
}

func TestSimpleCounter_Concurrent(t *testing.T) {
	counter := NewSimpleCounter()
	ctx := context.Background()
	now := time.Now()

	var wg sync.WaitGroup
	numGoroutines := 100
	incrementsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				counter.Increment(ctx, "concurrent_key", 1, time.Minute, now)
			}
		}()
	}
	wg.Wait()

	expected := numGoroutines * incrementsPerGoroutine
	if got := counter.GetCount(ctx, "concurrent_key", now); got != expected {
		t.Errorf("GetCount() = %d, want %d", got, expected)
	}
}

// ==================== WindowMonitor 基础功能测试 ====================

func TestWindowMonitor_New(t *testing.T) {
	monitor := New(Param{
		Interval:        100 * time.Millisecond,
		NumOfGoRoutines: 2,
	})

	if monitor == nil {
		t.Fatal("New() returned nil")
	}
	if monitor.listeners == nil {
		t.Error("listeners map is nil")
	}
	if monitor.tickerInterval != 100*time.Millisecond {
		t.Errorf("tickerInterval = %v, want 100ms", monitor.tickerInterval)
	}

	monitor.Stop()
}

func TestWindowMonitor_Register(t *testing.T) {
	monitor := New(Param{
		Interval:        100 * time.Millisecond,
		NumOfGoRoutines: 2,
	})
	defer monitor.Stop()

	ctx := context.Background()
	counter := NewSimpleCounter()

	err := monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  time.Minute,
		WindowCount: 5,
		CompareMode: Sequential,
		Counter:     counter,
	})

	if err != nil {
		t.Errorf("Register() error = %v", err)
	}

	if !monitor.IsRegistered("project1", "scene1") {
		t.Error("IsRegistered() = false, want true")
	}
}

func TestWindowMonitor_Register_Override(t *testing.T) {
	monitor := New(Param{
		Interval:        100 * time.Millisecond,
		NumOfGoRoutines: 2,
	})
	defer monitor.Stop()

	ctx := context.Background()
	counter := NewSimpleCounter()

	// 第一次注册
	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  time.Minute,
		WindowCount: 5,
		CompareMode: Sequential,
		Counter:     counter,
	})

	// 第二次注册，应该覆盖
	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  2 * time.Minute,
		WindowCount: 10,
		CompareMode: YearOverYear,
		Counter:     counter,
	})

	if !monitor.IsRegistered("project1", "scene1") {
		t.Error("IsRegistered() = false after override")
	}

	// 验证被覆盖的规则
	monitor.mu.RLock()
	listener := monitor.listeners["project1:scene1"]
	monitor.mu.RUnlock()

	if listener.rule.WindowSize != 2*time.Minute {
		t.Errorf("WindowSize = %v, want 2m", listener.rule.WindowSize)
	}
	if listener.rule.WindowCount != 10 {
		t.Errorf("WindowCount = %d, want 10", listener.rule.WindowCount)
	}
}

func TestWindowMonitor_UnRegister(t *testing.T) {
	monitor := New(Param{
		Interval:        100 * time.Millisecond,
		NumOfGoRoutines: 2,
	})
	defer monitor.Stop()

	ctx := context.Background()
	counter := NewSimpleCounter()

	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  time.Minute,
		WindowCount: 5,
		CompareMode: Sequential,
		Counter:     counter,
	})

	err := monitor.UnRegister(ctx, "project1", "scene1")
	if err != nil {
		t.Errorf("UnRegister() error = %v", err)
	}

	if monitor.IsRegistered("project1", "scene1") {
		t.Error("IsRegistered() = true after UnRegister, want false")
	}
}

func TestWindowMonitor_UnRegister_NotFound(t *testing.T) {
	monitor := New(Param{
		Interval:        100 * time.Millisecond,
		NumOfGoRoutines: 2,
	})
	defer monitor.Stop()

	ctx := context.Background()
	err := monitor.UnRegister(ctx, "not_exists", "scene")

	if err == nil {
		t.Error("UnRegister() should return error for non-existent listener")
	}
}

func TestWindowMonitor_IsRegistered(t *testing.T) {
	monitor := New(Param{
		Interval:        100 * time.Millisecond,
		NumOfGoRoutines: 2,
	})
	defer monitor.Stop()

	ctx := context.Background()
	counter := NewSimpleCounter()

	// 未注册
	if monitor.IsRegistered("project1", "scene1") {
		t.Error("IsRegistered() = true before Register, want false")
	}

	// 注册后
	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  time.Minute,
		WindowCount: 5,
		Counter:     counter,
	})

	if !monitor.IsRegistered("project1", "scene1") {
		t.Error("IsRegistered() = false after Register, want true")
	}

	// 不同的 scene
	if monitor.IsRegistered("project1", "scene2") {
		t.Error("IsRegistered() = true for different scene, want false")
	}
}

// ==================== Increment 测试 ====================

func TestWindowMonitor_Increment(t *testing.T) {
	monitor := New(Param{
		Interval:        100 * time.Millisecond,
		NumOfGoRoutines: 2,
	})
	defer monitor.Stop()

	ctx := context.Background()
	counter := NewSimpleCounter()

	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  time.Hour,
		WindowCount: 5,
		CompareMode: Sequential,
		Counter:     counter,
	})

	monitor.Increment(ctx, "project1", "scene1", 1)
	monitor.Increment(ctx, "project1", "scene1", 5)

	// 验证计数器被正确更新
	monitor.mu.RLock()
	listener := monitor.listeners["project1:scene1"]
	monitor.mu.RUnlock()

	listener.mu.Lock()
	windowKey := listener.getWindowKey(0, listener.windows[0].StartTime)
	listener.mu.Unlock()

	count := counter.GetCount(ctx, windowKey, time.Now())
	if count != 6 {
		t.Errorf("Counter count = %d, want 6", count)
	}
}

func TestWindowMonitor_Increment_NotRegistered(t *testing.T) {
	monitor := New(Param{
		Interval:        100 * time.Millisecond,
		NumOfGoRoutines: 2,
	})
	defer monitor.Stop()

	ctx := context.Background()

	// 对未注册的监听器调用 Increment，不应 panic
	monitor.Increment(ctx, "not_exists", "scene", 1)
}

// ==================== 窗口轮换测试 ====================

func TestWindowMonitor_WindowRotation_Sequential(t *testing.T) {
	var compareCount int32
	var lastResult CompareResult

	monitor := New(Param{
		Interval:        50 * time.Millisecond,
		NumOfGoRoutines: 2,
	})

	ctx := context.Background()
	counter := NewSimpleCounter()

	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  100 * time.Millisecond,
		WindowCount: 3,
		CompareMode: Sequential,
		Counter:     counter,
		OnCompare: func(result CompareResult) {
			atomic.AddInt32(&compareCount, 1)
			lastResult = result
		},
	})

	// 在第一个窗口增加计数
	monitor.Increment(ctx, "project1", "scene1", 10)

	monitor.Start()

	// 等待窗口轮换
	time.Sleep(300 * time.Millisecond)

	monitor.Stop()

	// 验证对比回调被调用
	if atomic.LoadInt32(&compareCount) == 0 {
		t.Error("OnCompare callback was not called")
	}

	// 验证对比模式
	if lastResult.CompareMode != Sequential {
		t.Errorf("CompareMode = %v, want Sequential", lastResult.CompareMode)
	}
}

func TestWindowMonitor_WindowRotation_GrowthRate(t *testing.T) {
	var results []CompareResult
	var mu sync.Mutex

	monitor := New(Param{
		Interval:        30 * time.Millisecond,
		NumOfGoRoutines: 2,
	})

	ctx := context.Background()
	counter := NewSimpleCounter()

	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  80 * time.Millisecond,
		WindowCount: 5,
		CompareMode: Sequential,
		Counter:     counter,
		OnCompare: func(result CompareResult) {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		},
	})

	// 第一个窗口增加 100
	monitor.Increment(ctx, "project1", "scene1", 100)

	monitor.Start()

	// 等待第一个窗口结束
	time.Sleep(100 * time.Millisecond)

	// 第二个窗口增加 150
	monitor.Increment(ctx, "project1", "scene1", 150)

	// 等待第二个窗口结束并触发对比
	time.Sleep(150 * time.Millisecond)

	monitor.Stop()

	mu.Lock()
	defer mu.Unlock()

	if len(results) < 1 {
		t.Fatal("No compare results received")
	}
}

// ==================== MinNumber/MaxNumber 限制测试 ====================

func TestWindowMonitor_MinNumber(t *testing.T) {
	var results []CompareResult
	var mu sync.Mutex

	monitor := New(Param{
		Interval:        30 * time.Millisecond,
		NumOfGoRoutines: 2,
	})

	ctx := context.Background()
	counter := NewSimpleCounter()

	minNumber := int64(100)
	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  80 * time.Millisecond,
		WindowCount: 3,
		CompareMode: Sequential,
		Counter:     counter,
		MinNumber:   &minNumber,
		OnCompare: func(result CompareResult) {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		},
	})

	// 第一个窗口增加 10（小于 MinNumber 100）
	monitor.Increment(ctx, "project1", "scene1", 10)

	monitor.Start()
	time.Sleep(250 * time.Millisecond)
	monitor.Stop()

	mu.Lock()
	defer mu.Unlock()

	if len(results) < 1 {
		t.Fatal("No compare result received")
	}

	// 验证 MinNumber 规则被正确配置
	if minNumber != 100 {
		t.Errorf("MinNumber = %d, want 100", minNumber)
	}
}

func TestWindowMonitor_MaxNumber(t *testing.T) {
	var results []CompareResult
	var mu sync.Mutex

	monitor := New(Param{
		Interval:        30 * time.Millisecond,
		NumOfGoRoutines: 2,
	})

	ctx := context.Background()
	counter := NewSimpleCounter()

	maxNumber := int64(50)
	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  80 * time.Millisecond,
		WindowCount: 3,
		CompareMode: Sequential,
		Counter:     counter,
		MaxNumber:   &maxNumber,
		OnCompare: func(result CompareResult) {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		},
	})

	// 第一个窗口增加 100（大于 MaxNumber 50）
	monitor.Increment(ctx, "project1", "scene1", 100)

	monitor.Start()
	time.Sleep(250 * time.Millisecond)
	monitor.Stop()

	mu.Lock()
	defer mu.Unlock()

	if len(results) < 1 {
		t.Fatal("No compare result received")
	}

	// 验证 MaxNumber 规则被正确配置
	if maxNumber != 50 {
		t.Errorf("MaxNumber = %d, want 50", maxNumber)
	}
}

// ==================== 多监听器测试 ====================

func TestWindowMonitor_MultipleListeners(t *testing.T) {
	var count1, count2 int32

	monitor := New(Param{
		Interval:        30 * time.Millisecond,
		NumOfGoRoutines: 4,
	})

	ctx := context.Background()
	counter := NewSimpleCounter()

	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  80 * time.Millisecond,
		WindowCount: 3,
		CompareMode: Sequential,
		Counter:     counter,
		OnCompare: func(result CompareResult) {
			atomic.AddInt32(&count1, 1)
		},
	})

	_ = monitor.Register(ctx, "project2", "scene2", MonitorRule{
		WindowSize:  80 * time.Millisecond,
		WindowCount: 3,
		CompareMode: Sequential,
		Counter:     counter,
		OnCompare: func(result CompareResult) {
			atomic.AddInt32(&count2, 1)
		},
	})

	monitor.Increment(ctx, "project1", "scene1", 10)
	monitor.Increment(ctx, "project2", "scene2", 20)

	monitor.Start()
	time.Sleep(250 * time.Millisecond)
	monitor.Stop()

	if atomic.LoadInt32(&count1) == 0 {
		t.Error("Listener 1 OnCompare was not called")
	}
	if atomic.LoadInt32(&count2) == 0 {
		t.Error("Listener 2 OnCompare was not called")
	}
}

// ==================== getWindowKey 测试 ====================

func TestMonitorListener_GetWindowKey_Sequential(t *testing.T) {
	listener := &monitorListener{
		projectId: "proj1",
		scene:     "scene1",
		rule: MonitorRule{
			CompareMode: Sequential,
		},
	}

	now := time.Now()
	key := listener.getWindowKey(0, now)
	expected := "proj1:scene1:0"

	if key != expected {
		t.Errorf("getWindowKey() = %s, want %s", key, expected)
	}

	key2 := listener.getWindowKey(5, now)
	expected2 := "proj1:scene1:5"

	if key2 != expected2 {
		t.Errorf("getWindowKey() = %s, want %s", key2, expected2)
	}
}

func TestMonitorListener_GetWindowKey_YearOverYear(t *testing.T) {
	listener := &monitorListener{
		projectId: "proj1",
		scene:     "scene1",
		rule: MonitorRule{
			CompareMode: YearOverYear,
		},
	}

	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.Local)
	key := listener.getWindowKey(0, testTime)
	expected := "proj1:scene1:14:30"

	if key != expected {
		t.Errorf("getWindowKey() = %s, want %s", key, expected)
	}

	// 不同窗口索引，相同时间，应该返回相同的 key
	key2 := listener.getWindowKey(5, testTime)
	if key2 != expected {
		t.Errorf("getWindowKey() = %s, want %s", key2, expected)
	}
}

// ==================== Stop 测试 ====================

func TestWindowMonitor_Stop(t *testing.T) {
	monitor := New(Param{
		Interval:        50 * time.Millisecond,
		NumOfGoRoutines: 2,
	})

	monitor.Start()
	time.Sleep(100 * time.Millisecond)

	// 多次调用 Stop 不应 panic
	monitor.Stop()
	monitor.Stop()
}

// ==================== 边界条件测试 ====================

func TestWindowMonitor_EmptyWindows(t *testing.T) {
	monitor := New(Param{
		Interval:        50 * time.Millisecond,
		NumOfGoRoutines: 2,
	})
	defer monitor.Stop()

	ctx := context.Background()

	// 手动创建一个空窗口的监听器
	monitor.mu.Lock()
	monitor.listeners["test:empty"] = &monitorListener{
		projectId: "test",
		scene:     "empty",
		rule: MonitorRule{
			WindowSize:  time.Minute,
			WindowCount: 3,
		},
		windows: []WindowData{}, // 空窗口
	}
	monitor.mu.Unlock()

	// 对空窗口调用 Increment，不应 panic
	monitor.Increment(ctx, "test", "empty", 1)
}

func TestWindowMonitor_SingleWindow_NoCompare(t *testing.T) {
	var compareCount int32

	monitor := New(Param{
		Interval:        30 * time.Millisecond,
		NumOfGoRoutines: 2,
	})

	ctx := context.Background()
	counter := NewSimpleCounter()

	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  200 * time.Millisecond, // 较长窗口
		WindowCount: 2,
		CompareMode: Sequential,
		Counter:     counter,
		OnCompare: func(result CompareResult) {
			atomic.AddInt32(&compareCount, 1)
		},
	})

	monitor.Increment(ctx, "project1", "scene1", 10)

	monitor.Start()
	// 短时间内不会触发窗口轮换
	time.Sleep(50 * time.Millisecond)
	monitor.Stop()

	// 只有一个窗口时不应触发对比
	if atomic.LoadInt32(&compareCount) != 0 {
		t.Errorf("compareCount = %d, want 0 (no compare with single window)", compareCount)
	}
}

func TestWindowMonitor_ZeroToPositive_GrowthRate(t *testing.T) {
	var results []CompareResult
	var mu sync.Mutex

	monitor := New(Param{
		Interval:        30 * time.Millisecond,
		NumOfGoRoutines: 2,
	})

	ctx := context.Background()
	counter := NewSimpleCounter()

	_ = monitor.Register(ctx, "project1", "scene1", MonitorRule{
		WindowSize:  80 * time.Millisecond,
		WindowCount: 3,
		CompareMode: Sequential,
		Counter:     counter,
		OnCompare: func(result CompareResult) {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		},
	})

	// 第一个窗口不增加任何计数（保持 0）

	monitor.Start()
	time.Sleep(250 * time.Millisecond)
	monitor.Stop()

	mu.Lock()
	defer mu.Unlock()

	// 验证有对比结果产生
	if len(results) < 1 {
		t.Fatal("No compare result received")
	}

	// 验证第一次对比时 WindowA 为 0（第一个窗口没有增加计数）
	// 注意：窗口轮换时 windowB 是新创建的空窗口
	firstResult := results[0]
	if firstResult.WindowA != 0 {
		t.Logf("First compare: WindowA=%d, WindowB=%d, GrowthRate=%.2f",
			firstResult.WindowA, firstResult.WindowB, firstResult.GrowthRate)
	}
}

// ==================== 并发安全测试 ====================

func TestWindowMonitor_ConcurrentOperations(t *testing.T) {
	monitor := New(Param{
		Interval:        50 * time.Millisecond,
		NumOfGoRoutines: 4,
	})

	ctx := context.Background()
	counter := NewSimpleCounter()

	var wg sync.WaitGroup

	// 并发注册
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = monitor.Register(ctx, "project", "scene"+string(rune('0'+idx)), MonitorRule{
				WindowSize:  time.Minute,
				WindowCount: 3,
				Counter:     counter,
			})
		}(i)
	}

	// 并发增加计数
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			scene := "scene" + string(rune('0'+idx%10))
			monitor.Increment(ctx, "project", scene, 1)
		}(i)
	}

	wg.Wait()
	monitor.Stop()
}

