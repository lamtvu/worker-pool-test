package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

func exampleBasicUsage() {
	fmt.Println("\n=== Example 1: Basic Usage (Block Strategy) ===")

	config := WorkerPoolConfig{
		NumWorkers:   3,
		QueueSize:    10,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)

	for i := 0; i < 10; i++ {
		task := &SimpleTask{
			ID:       i,
			Duration: 100 * time.Millisecond,
		}
		pool.Submit(task)
		fmt.Printf("Submitted task %d\n", i)
	}

	fmt.Print("Waiting for all tasks to complete...")
	start := time.Now()
	pool.Wait()
	duration := time.Since(start)

	stats := pool.GetStats()
	fmt.Printf("\nCompleted in %v\n", duration)
	fmt.Printf("Tasks processed: %d\n", stats.TasksProcessed)
	fmt.Printf("Active workers: %d\n", stats.ActiveWorkers)

	pool.Stop(2 * time.Second)
}

func exampleRejectStrategy() {
	fmt.Println("\n=== Example 2: Reject Strategy ===")

	rejectedCount := atomic.Int64{}
	config := WorkerPoolConfig{
		NumWorkers:   1,
		QueueSize:    3,
		OverflowMode: Reject,
	}
	pool := NewWorkerPool(config)

	for i := 0; i < 15; i++ {
		task := &SimpleTask{
			ID:       i,
			Duration: 500 * time.Millisecond,
		}
		err := pool.Submit(task)
		if err != nil {
			rejectedCount.Add(1)
			fmt.Printf("Task %d REJECTED: %v\n", i, err)
		} else {
			fmt.Printf("Task %d queued\n", i)
		}
	}

	pool.Wait()

	stats := pool.GetStats()
	fmt.Printf("\nTasks processed: %d\n", stats.TasksProcessed)
	fmt.Printf("Tasks rejected: %d\n", rejectedCount.Load())

	pool.Stop(2 * time.Second)
}

func exampleDiscardOldestStrategy() {
	fmt.Println("\n=== Example 3: DiscardOldest Strategy ===")

	config := WorkerPoolConfig{
		NumWorkers:   1,
		QueueSize:    2,
		OverflowMode: DiscardOldest,
	}
	pool := NewWorkerPool(config)

	for i := 0; i < 8; i++ {
		task := &SimpleTask{
			ID:       i,
			Duration: 300 * time.Millisecond,
		}
		err := pool.Submit(task)
		if err != nil {
			fmt.Printf("Task %d error: %v\n", i, err)
		} else {
			fmt.Printf("Task %d submitted\n", i)
		}
		time.Sleep(50 * time.Millisecond)
	}

	pool.Wait()

	stats := pool.GetStats()
	fmt.Printf("\nTasks processed: %d\n", stats.TasksProcessed)
	fmt.Printf("Note: Some early tasks may have been discarded to make room\n")

	pool.Stop(2 * time.Second)
}

func exampleErrorHandling() {
	fmt.Println("\n=== Example 4: Error Handling ===")

	errorCount := atomic.Int64{}
	panicCount := atomic.Int64{}

	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    10,
		OverflowMode: Block,
		OnTaskError: func(task Task, err error) {
			errorCount.Add(1)
			fmt.Printf("ERROR: Task failed with: %v\n", err)
		},
		OnPanic: func(err interface{}) {
			panicCount.Add(1)
			fmt.Printf("PANIC: %v\n", err)
		},
	}
	pool := NewWorkerPool(config)

	for i := 0; i < 9; i++ {
		var shouldErr bool
		var shouldPanic bool

		if i%3 == 1 {
			shouldErr = true
		} else if i%3 == 2 {
			shouldPanic = true
		}

		task := &SimpleTask{
			ID:          i,
			Duration:    50 * time.Millisecond,
			ShouldErr:   shouldErr,
			ShouldPanic: shouldPanic,
		}
		pool.Submit(task)
	}

	pool.Wait()

	stats := pool.GetStats()
	fmt.Printf("\nSummary:\n")
	fmt.Printf("Tasks processed: %d\n", stats.TasksProcessed)
	fmt.Printf("Errors encountered: %d\n", stats.ErrorsEncountered)
	fmt.Printf("Panics recovered: %d\n", panicCount.Load())

	pool.Stop(2 * time.Second)
}

func exampleConcurrentSubmissions() {
	fmt.Println("\n=== Example 5: Concurrent Submissions ===")

	config := WorkerPoolConfig{
		NumWorkers:   4,
		QueueSize:    50,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)

	wg := sync.WaitGroup{}
	numGoroutines := 5
	tasksPerGoroutine := 20

	fmt.Printf("Submitting %d tasks from %d goroutines\n",
		numGoroutines*tasksPerGoroutine, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < tasksPerGoroutine; i++ {
				task := &SimpleTask{
					ID:       goroutineID*1000 + i,
					Duration: time.Duration(rand.Intn(100)) * time.Millisecond,
				}
				pool.Submit(task)
			}
		}(g)
	}

	wg.Wait()

	start := time.Now()
	pool.Wait()
	duration := time.Since(start)

	stats := pool.GetStats()
	fmt.Printf("\nCompleted in %v\n", duration)
	fmt.Printf("Tasks processed: %d\n", stats.TasksProcessed)
	fmt.Printf("Throughput: %.2f tasks/sec\n", float64(stats.TasksProcessed)/duration.Seconds())

	pool.Stop(2 * time.Second)
}

func exampleMonitoring() {
	fmt.Println("\n=== Example 6: Monitoring Pool Statistics ===")

	config := WorkerPoolConfig{
		NumWorkers:   3,
		QueueSize:    20,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)

	go func() {
		for i := 0; i < 30; i++ {
			task := &SimpleTask{
				ID:       i,
				Duration: 200 * time.Millisecond,
			}
			pool.Submit(task)
			time.Sleep(100 * time.Millisecond)
		}
	}()

	fmt.Println("Monitoring pool statistics:")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	done := make(chan bool)
	go func() {
		for {
			select {
			case <-ticker.C:
				stats := pool.GetStats()
				fmt.Printf("Processed: %d | Active: %d | Queue: %d\n",
					stats.TasksProcessed, stats.ActiveWorkers, stats.QueueSize)
			case <-done:
				return
			}
		}
	}()

	pool.Wait()
	done <- true

	stats := pool.GetStats()
	fmt.Printf("\nFinal stats - Processed: %d | Errors: %d\n",
		stats.TasksProcessed, stats.ErrorsEncountered)

	pool.Stop(2 * time.Second)
}

type DatabaseQueryTask struct {
	QueryID   int
	QueryText string
	Duration  time.Duration
}

func (t *DatabaseQueryTask) Execute(ctx context.Context) error {
	fmt.Printf("Executing query %d: %s\n", t.QueryID, t.QueryText)
	time.Sleep(t.Duration)
	fmt.Printf("Query %d completed\n", t.QueryID)
	return nil
}

func exampleCustomTask() {
	fmt.Println("\n=== Example 7: Custom Task Implementation ===")

	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    10,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)

	queries := []DatabaseQueryTask{
		{QueryID: 1, QueryText: "SELECT * FROM users", Duration: 100 * time.Millisecond},
		{QueryID: 2, QueryText: "INSERT INTO logs", Duration: 50 * time.Millisecond},
		{QueryID: 3, QueryText: "UPDATE products", Duration: 150 * time.Millisecond},
		{QueryID: 4, QueryText: "DELETE FROM cache", Duration: 75 * time.Millisecond},
	}

	for _, q := range queries {
		queryTask := &DatabaseQueryTask{
			QueryID:   q.QueryID,
			QueryText: q.QueryText,
			Duration:  q.Duration,
		}
		pool.Submit(queryTask)
	}

	pool.Wait()

	stats := pool.GetStats()
	fmt.Printf("\nCompleted %d database operations\n", stats.TasksProcessed)

	pool.Stop(2 * time.Second)
}

func exampleLoadBalancing() {
	fmt.Println("\n=== Example 8: Load Balancing with Multiple Workers ===")

	taskCounter := atomic.Int64{}

	config := WorkerPoolConfig{
		NumWorkers:   4,
		QueueSize:    100,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)

	for i := 0; i < 100; i++ {
		task := &SimpleTask{
			ID:       i,
			Duration: 10 * time.Millisecond,
		}
		pool.Submit(task)
		if i%25 == 0 {
			taskCounter.Add(1)
		}
	}

	start := time.Now()
	pool.Wait()
	duration := time.Since(start)

	stats := pool.GetStats()
	fmt.Printf("Load balancing results:\n")
	fmt.Printf("Workers: %d | Total tasks: %d | Duration: %v\n",
		config.NumWorkers, stats.TasksProcessed, duration)
	fmt.Printf("Throughput: %.2f tasks/sec\n", float64(stats.TasksProcessed)/duration.Seconds())

	pool.Stop(2 * time.Second)
}

func exampleGracefulShutdown() {
	fmt.Println("\n=== Example 9: Graceful Shutdown ===")

	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    10,
		OverflowMode: Block,
	}
	pool := NewWorkerPool(config)

	for i := 0; i < 5; i++ {
		task := &SimpleTask{
			ID:       i,
			Duration: 200 * time.Millisecond,
		}
		pool.Submit(task)
	}

	fmt.Println("Initiating graceful shutdown...")
	err := pool.Stop(10 * time.Second)
	if err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	} else {
		fmt.Println("Gracefully shut down completed")
	}

	stats := pool.GetStats()
	fmt.Printf("Final stats - Tasks processed: %d\n", stats.TasksProcessed)
}

func exampleQueueManagement() {
	fmt.Println("\n=== Example 10: Queue Size Management ===")

	config := WorkerPoolConfig{
		NumWorkers:   2,
		QueueSize:    5,
		OverflowMode: Reject,
	}
	pool := NewWorkerPool(config)

	successCount := atomic.Int64{}
	rejectCount := atomic.Int64{}

	for i := 0; i < 20; i++ {
		task := &SimpleTask{
			ID:       i,
			Duration: 100 * time.Millisecond,
		}
		err := pool.Submit(task)
		if err != nil {
			rejectCount.Add(1)
			fmt.Printf("Task %d rejected\n", i)
		} else {
			successCount.Add(1)
			fmt.Printf("Task %d accepted\n", i)
		}

		if i%5 == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	pool.Wait()

	fmt.Printf("\nQueue management results:\n")
	fmt.Printf("Accepted: %d | Rejected: %d\n", successCount.Load(), rejectCount.Load())

	stats := pool.GetStats()
	fmt.Printf("Tasks processed: %d\n", stats.TasksProcessed)

	pool.Stop(2 * time.Second)
}

func main() {
	fmt.Println("╔════════════════════════════════════════════════╗")
	fmt.Println("║  Worker Pool - Thread-Safe Implementation      ║")
	fmt.Println("║  Comprehensive Examples & Demonstrations       ║")
	fmt.Println("╚════════════════════════════════════════════════╝")

	exampleBasicUsage()
	exampleRejectStrategy()
	exampleDiscardOldestStrategy()
	exampleErrorHandling()
	exampleConcurrentSubmissions()
	exampleMonitoring()
	exampleCustomTask()
	exampleLoadBalancing()
	exampleGracefulShutdown()
	exampleQueueManagement()

	fmt.Println("\n╔════════════════════════════════════════════════╗")
	fmt.Println("║  All Examples Completed Successfully           ║")
	fmt.Println("╚════════════════════════════════════════════════╝")
}
