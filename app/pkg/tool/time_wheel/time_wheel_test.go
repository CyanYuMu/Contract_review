package timewheel

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestTimeWheel(t *testing.T) {
	// Create a time wheel with configuration
	config := Config{
		Interval: time.Second,
		MaxDelay: time.Minute,
		SlotSize: 60,
	}
	tw := New(config)
	defer tw.Stop()

	var wg sync.WaitGroup
	executed := make(map[string]bool)
	var mu sync.Mutex

	// Test adding tasks with different priorities
	tasks := []struct {
		id       string
		delay    time.Duration
		priority int
	}{
		{"task1", 1000 * time.Millisecond, 1},
		{"task2", 4000 * time.Millisecond, 2},
		{"task3", 8000 * time.Millisecond, 3},
	}

	for _, task := range tasks {
		wg.Add(1)
		taskID := task.id
		tw.AddTask(&Task{
			ID:       taskID,
			Priority: task.priority,
			Delay:    task.delay,
			Execute: func() {
				defer wg.Done()
				mu.Lock()
				executed[taskID] = true
				fmt.Println("Task", taskID, "executed", time.Now().String())
				mu.Unlock()
			},
		})
	}

	// Wait for all tasks to complete
	wg.Wait()

	// Verify all tasks were executed
	//mu.Lock()
	//for _, task := range tasks {
	//	if !executed[task.id] {
	//		t.Errorf("Task %s was not executed", task.id)
	//	}
	//}
	//mu.Unlock()

	// Test removing a task
	//task := &Task{
	//	ID:       "task4",
	//	Priority: 1,
	//	Delay:    50 * time.Millisecond,
	//	Execute: func() {
	//		t.Error("Removed task should not execute")
	//	},
	//}
	//
	//tw.AddTask(task)
	//if !tw.RemoveTask("task4") {
	//	t.Error("Failed to remove task")
	//}
	//
	//// Wait to ensure removed task doesn't execute
	//time.Sleep(100 * time.Millisecond)
}
