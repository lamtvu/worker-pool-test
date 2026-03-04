package main

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBasicSubmitAndExecute(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    10,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(5 * time.Second)

	taskCount := atomic.Int64{}
	for i := 0; i < 5; i++ {
		task := &SimpleTask{ID: i, Duration: 10 * time.Millisecond}
		go func() {
			pool.Submit(task)
		}()
	}

	pool.Wait()
	stats := pool.GetStats()

	if stats.TasksProcessed < 5 {
		t.Errorf("expected at least 5 tasks processed, got %d", stats.TasksProcessed)
	}

	_ = taskCount
}

func TestBlockingOverflow(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   1,
		QueueSize:    3,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(5 * time.Second)

	for i := 0; i < 5; i++ {
		task := &SimpleTask{ID: i, Duration: 100 * time.Millisecond}
		err := pool.Submit(task)
		if err != nil {
			t.Errorf("submit should not fail: %v", err)
		}
	}

	pool.Wait()
	stats := pool.GetStats()

	if stats.TasksProcessed < 5 {
		t.Errorf("expected 5 tasks processed, got %d", stats.TasksProcessed)
	}
}

func TestRejectOverflow(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   1,
		QueueSize:    2,
		OverflowMode: Reject,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(5 * time.Second)

	rejectedCount := 0
	for i := 0; i < 10; i++ {
		task := &SimpleTask{ID: i, Duration: 100 * time.Millisecond}
		err := pool.Submit(task)
		if err != nil {
			rejectedCount++
		}
	}

	pool.Wait()

	if rejectedCount == 0 {
		t.Errorf("expected some tasks to be rejected")
	}

	stats := pool.GetStats()
	if stats.TasksProcessed+int64(rejectedCount) < 10 {
		t.Errorf("total of processed and rejected should be at least 10")
	}
}

func TestDiscardOldestOverflow(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   1,
		QueueSize:    2,
		OverflowMode: DiscardOldest,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(5 * time.Second)

	for i := 0; i < 10; i++ {
		task := &SimpleTask{ID: i, Duration: 10 * time.Millisecond}
		err := pool.Submit(task)
		if err != nil {
			t.Errorf("submit with DiscardOldest should not fail: %v", err)
		}
	}

	pool.Wait()
	stats := pool.GetStats()

	if stats.TasksProcessed == 0 {
		t.Errorf("expected some tasks to be processed")
	}
}

func TestErrorHandling(t *testing.T) {
	errorCount := atomic.Int64{}
	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    10,
		OverflowMode: Block,
		OnTaskError: func(task Task, err error) {
			errorCount.Add(1)
		},
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(5 * time.Second)

	for i := 0; i < 5; i++ {
		task := &SimpleTask{ID: i, Duration: 5 * time.Millisecond, ShouldErr: true}
		pool.Submit(task)
	}

	pool.Wait()
	stats := pool.GetStats()

	if int64(errorCount.Load()) != 5 {
		t.Errorf("expected 5 errors, got %d", errorCount.Load())
	}

	if stats.ErrorsEncountered != 5 {
		t.Errorf("expected 5 errors in stats, got %d", stats.ErrorsEncountered)
	}
}

func TestPanicRecovery(t *testing.T) {
	panicCount := atomic.Int64{}
	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    10,
		OverflowMode: Block,
		OnPanic: func(err interface{}) {
			panicCount.Add(1)
		},
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(5 * time.Second)

	for i := 0; i < 3; i++ {
		task := &SimpleTask{ID: i, Duration: 5 * time.Millisecond, ShouldPanic: true}
		pool.Submit(task)
	}

	for i := 3; i < 6; i++ {
		task := &SimpleTask{ID: i, Duration: 5 * time.Millisecond}
		pool.Submit(task)
	}

	pool.Wait()

	if panicCount.Load() != 3 {
		t.Errorf("expected 3 panics, got %d", panicCount.Load())
	}

	stats := pool.GetStats()
	if stats.TasksProcessed < 6 {
		t.Errorf("expected at least 6 tasks processed after panics, got %d", stats.TasksProcessed)
	}
}

func TestConcurrentSubmit(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   4,
		QueueSize:    50,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(10 * time.Second)

	wg := sync.WaitGroup{}
	totalTasks := 100

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				task := &SimpleTask{
					ID:       start + j,
					Duration: time.Duration(rand.Intn(50)) * time.Millisecond,
				}
				pool.Submit(task)
			}
		}(i * 10)
	}

	wg.Wait()
	pool.Wait()

	stats := pool.GetStats()
	if stats.TasksProcessed < int64(totalTasks-10) {
		t.Errorf("expected around 100 tasks processed, got %d", stats.TasksProcessed)
	}
}

func TestStats(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    10,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(5 * time.Second)

	for i := 0; i < 10; i++ {
		task := &SimpleTask{ID: i, Duration: 5 * time.Millisecond}
		pool.Submit(task)
	}

	stats1 := pool.GetStats()
	if stats1.ActiveWorkers < 0 {
		t.Errorf("active workers should be non-negative")
	}

	pool.Wait()

	stats2 := pool.GetStats()
	if stats2.TasksProcessed < 10 {
		t.Errorf("expected 10 tasks processed, got %d", stats2.TasksProcessed)
	}
	if stats2.ActiveWorkers != 0 {
		t.Errorf("expected 0 active workers after wait, got %d", stats2.ActiveWorkers)
	}
}

func TestSubmitAfterClose(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   1,
		QueueSize:    10,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)

	pool.Stop(2 * time.Second)

	task := &SimpleTask{ID: 0, Duration: 5 * time.Millisecond}
	err := pool.Submit(task)

	if err == nil {
		t.Errorf("expected error when submitting to closed pool")
	}
}

func TestMultipleWorkers(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   4,
		QueueSize:    100,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(10 * time.Second)

	for i := 0; i < 20; i++ {
		task := &SimpleTask{ID: i, Duration: 5 * time.Millisecond}
		pool.Submit(task)
	}

	pool.Wait()
	stats := pool.GetStats()

	if stats.TasksProcessed < 20 {
		t.Errorf("expected 20 tasks processed, got %d", stats.TasksProcessed)
	}
}

func TestWaitWithMultipleCalls(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    10,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(5 * time.Second)

	for i := 0; i < 5; i++ {
		task := &SimpleTask{ID: i, Duration: 10 * time.Millisecond}
		pool.Submit(task)
	}

	pool.Wait()
	stats1 := pool.GetStats()

	for i := 5; i < 10; i++ {
		task := &SimpleTask{ID: i, Duration: 10 * time.Millisecond}
		pool.Submit(task)
	}

	pool.Wait()
	stats2 := pool.GetStats()

	if stats2.TasksProcessed <= stats1.TasksProcessed {
		t.Errorf("second batch should have processed more tasks")
	}
}

func TestHighLoad(t *testing.T) {
	config := WorkerPoolConfig{
		NumWorkers:   8,
		QueueSize:    200,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)
	defer pool.Stop(30 * time.Second)

	taskCount := 500

	start := time.Now()
	for i := 0; i < taskCount; i++ {
		task := &SimpleTask{
			ID:       i,
			Duration: time.Duration(rand.Intn(10)) * time.Millisecond,
		}
		pool.Submit(task)
	}

	pool.Wait()
	duration := time.Since(start)

	stats := pool.GetStats()
	if stats.TasksProcessed < int64(taskCount-50) {
		t.Errorf("expected around %d tasks processed, got %d", taskCount, stats.TasksProcessed)
	}

	t.Logf("Processed %d tasks in %v (%.2f tasks/sec)", stats.TasksProcessed, duration,
		float64(stats.TasksProcessed)/duration.Seconds())
}
