package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"contract_review/app/internal/global"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

func TestReviewOrchestratorConcurrentCallbacksAreIsolated(t *testing.T) {
	previousLogger := global.Log
	global.Log = zap.NewNop()
	defer func() { global.Log = previousLogger }()

	var firstCalls atomic.Int32
	release := make(chan struct{})
	fakeLLM := func(_ context.Context, _ []*schema.Message) (*schema.Message, error) {
		call := firstCalls.Add(1)
		if call <= 2 {
			if call == 2 {
				close(release)
			}
			<-release
		}
		time.Sleep(time.Millisecond)
		return &schema.Message{
			Role:    schema.Assistant,
			Content: "{\"findings\":[],\"overall_score\":0.8,\"should_retry\":false}",
		}, nil
	}

	config := DefaultOrchestratorConfig()
	config.MaxConcurrentAgents = 1
	config.RiskReviewBatchSize = 1
	orchestrator := NewReviewOrchestrator(fakeLLM, nil, nil, config)
	lineBreak := string(rune(10))
	contractText := "第一条 服务范围" + lineBreak +
		"乙方提供服务。" + lineBreak +
		"第二条 付款方式" + lineBreak +
		"甲方按期付款。"

	type collector struct {
		mu       sync.Mutex
		progress int
		finished bool
	}
	collectors := []*collector{{}, {}}

	var wg sync.WaitGroup
	errs := make(chan error, len(collectors))
	for _, current := range collectors {
		current := current
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := orchestrator.ReviewContract(
				context.Background(),
				contractText,
				ContractMeta{ContractType: "服务合同"},
				ReviewCallbacks{
					OnProgress: func(event ProgressEvent) {
						current.mu.Lock()
						defer current.mu.Unlock()
						current.progress++
						if event.Progress == 1 {
							current.finished = true
						}
					},
				},
			)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent review failed: %v", err)
		}
	}
	for i, current := range collectors {
		current.mu.Lock()
		progress, finished := current.progress, current.finished
		current.mu.Unlock()
		if progress == 0 || !finished {
			t.Fatalf("collector %d did not receive its complete callback stream: progress=%d finished=%v",
				i, progress, finished)
		}
	}
}
