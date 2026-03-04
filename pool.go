package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Task interface {
	Execute(ctx context.Context) error
}

type OverflowStrategy int

const (
	Block OverflowStrategy = iota
	Reject
	DiscardOldest
)

type WorkerPoolConfig struct {
	NumWorkers   int
	QueueSize    int
	OverflowMode OverflowStrategy
	OnPanic      func(err interface{})
	OnTaskError  func(task Task, err error)
}

type WorkerPool struct {
	config        WorkerPoolConfig
	taskQueue     chan Task
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	stats         PoolStats
	taskCount     int64
	errorCount    int64
	activeWorkers int64
}

type PoolStats struct {
	TasksProcessed    int64
	TasksRejected     int64
	ErrorsEncountered int64
	ActiveWorkers     int64
	QueueSize         int
}

func NewWorkerPool(config WorkerPoolConfig) *WorkerPool {
	if config.NumWorkers <= 0 {
		config.NumWorkers = 1
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	wp := &WorkerPool{
		config:    config,
		taskQueue: make(chan Task, config.QueueSize),
		ctx:       ctx,
		cancel:    cancel,
	}

	for i := 0; i < config.NumWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	return wp
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case task, ok := <-wp.taskQueue:
			if !ok {
				return
			}

			atomic.AddInt64(&wp.activeWorkers, 1)

			func() {
				defer func() {
					if r := recover(); r != nil {
						if wp.config.OnPanic != nil {
							wp.config.OnPanic(r)
						}
					}
					atomic.AddInt64(&wp.activeWorkers, -1)
				}()

				if err := task.Execute(wp.ctx); err != nil {
					atomic.AddInt64(&wp.errorCount, 1)
					if wp.config.OnTaskError != nil {
						wp.config.OnTaskError(task, err)
					}
				}
			}()

			atomic.AddInt64(&wp.taskCount, 1)
		}
	}
}

func (wp *WorkerPool) Submit(task Task) error {
	select {
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool is closed")
	default:
	}

	switch wp.config.OverflowMode {
	case Block:
		return wp.submitBlock(task)
	case Reject:
		return wp.submitReject(task)
	case DiscardOldest:
		return wp.submitDiscardOldest(task)
	default:
		return wp.submitBlock(task)
	}
}

func (wp *WorkerPool) submitBlock(task Task) error {
	select {
	case wp.taskQueue <- task:
		return nil
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool is closed")
	}
}

func (wp *WorkerPool) submitReject(task Task) error {
	select {
	case wp.taskQueue <- task:
		return nil
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool is closed")
	default:
		atomic.AddInt64(&wp.stats.TasksRejected, 1)
		return fmt.Errorf("task queue is full, task rejected")
	}
}

func (wp *WorkerPool) submitDiscardOldest(task Task) error {
	select {
	case wp.taskQueue <- task:
		return nil
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool is closed")
	default:
		select {
		case <-wp.taskQueue:
			select {
			case wp.taskQueue <- task:
				return nil
			case <-wp.ctx.Done():
				return fmt.Errorf("worker pool is closed")
			}
		default:
			select {
			case wp.taskQueue <- task:
				return nil
			default:
				return fmt.Errorf("failed to add task after discarding oldest")
			}
		}
	}
}

func (wp *WorkerPool) Wait() {
	for {
		if len(wp.taskQueue) == 0 && atomic.LoadInt64(&wp.activeWorkers) == 0 {
			time.Sleep(10 * time.Millisecond)
			if len(wp.taskQueue) == 0 && atomic.LoadInt64(&wp.activeWorkers) == 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (wp *WorkerPool) Stop(timeout time.Duration) error {
	wp.cancel()

	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("worker pool shutdown timeout exceeded")
	}
}

func (wp *WorkerPool) GetStats() PoolStats {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	return PoolStats{
		TasksProcessed:    atomic.LoadInt64(&wp.taskCount),
		ErrorsEncountered: atomic.LoadInt64(&wp.errorCount),
		ActiveWorkers:     atomic.LoadInt64(&wp.activeWorkers),
		QueueSize:         len(wp.taskQueue),
	}
}

type SimpleTask struct {
	ID        int
	Duration  time.Duration
	ShouldErr bool
	ShouldPanic bool
}

func (t *SimpleTask) Execute(ctx context.Context) error {
	if t.ShouldPanic {
		panic("intentional panic in task")
	}
	time.Sleep(t.Duration)
	if t.ShouldErr {
		return fmt.Errorf("task %d failed", t.ID)
	}
	return nil
}
