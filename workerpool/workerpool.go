package workerpool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gammazero/deque"
	"github.com/go-redis/redis/v8"
)

const (
	idleTimeout = 2 * time.Second
	redisKey    = "workerpool:tasks"
)

type Task struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type WorkerPool struct {
	maxWorkers   int
	taskQueue    chan Task
	workerQueue  chan func()
	stoppedChan  chan struct{}
	stopSignal   chan struct{}
	waitingQueue deque.Deque[Task]
	stopLock     sync.Mutex
	stopOnce     sync.Once
	stopped      bool
	waiting      int32
	wait         bool
	redisClient  *redis.Client
}

func New(maxWorkers int, redisAddr string) (*WorkerPool, error) {
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	pool := &WorkerPool{
		maxWorkers:  maxWorkers,
		taskQueue:   make(chan Task),
		workerQueue: make(chan func()),
		stopSignal:  make(chan struct{}),
		stoppedChan: make(chan struct{}),
		redisClient: redisClient,
	}

	go pool.dispatch()

	return pool, nil
}

func (p *WorkerPool) readTaskFromRedis(ctx context.Context) (*Task, error) {
	result, err := p.redisClient.BLPop(ctx, 0, redisKey).Result()
	if err != nil {
		return nil, err
	}

	var task Task
	err = json.Unmarshal([]byte(result[1]), &task)
	if err != nil {
		return nil, err
	}

	return &task, nil
}

func (p *WorkerPool) Size() int {
	return p.maxWorkers
}

func (p *WorkerPool) Stop() {
	p.stop(false)
}

func (p *WorkerPool) StopWait() {
	p.stop(true)
}

func (p *WorkerPool) Submit(task Task) {
	if task.ID != "" {
		p.taskQueue <- task
	}
}

func (p *WorkerPool) SubmitWait(task Task) {
	if task.ID == "" {
		return
	}
	doneChan := make(chan struct{})
	p.taskQueue <- task
	<-doneChan
}

func (p *WorkerPool) WaitingQueueSize() int {
	return int(atomic.LoadInt32(&p.waiting))
}

func (p *WorkerPool) dispatch() {
	defer close(p.stoppedChan)
	timeout := time.NewTimer(idleTimeout)
	var workerCount int
	var idle bool
	var wg sync.WaitGroup

	ctx := context.Background()

Loop:
	for {
		if p.waitingQueue.Len() != 0 {
			if !p.processWaitingQueue() {
				break Loop
			}
			continue
		}

		select {
		case task, ok := <-p.taskQueue:
			if !ok {
				break Loop
			}
			p.processTask(task, &workerCount, &wg)
			idle = false
		case <-timeout.C:
			if idle && workerCount > 0 {
				if p.killIdleWorker() {
					workerCount--
				}
			}
			idle = true
			timeout.Reset(idleTimeout)
		default:
			// Check Redis queue
			result, err := p.redisClient.BLPop(ctx, 0, redisKey).Result()
			if err != nil {
				log.Printf("Error reading from Redis: %v", err)
				continue
			}
			if len(result) == 2 {
				var task Task
				err = json.Unmarshal([]byte(result[1]), &task)
				if err != nil {
					log.Printf("Error unmarshaling task: %v", err)
					continue
				}
				p.processTask(task, &workerCount, &wg)
				idle = false
			}
		}
	}

	if p.wait {
		p.runQueuedTasks()
	}

	for workerCount > 0 {
		p.workerQueue <- nil
		workerCount--
	}
	wg.Wait()

	timeout.Stop()
}

func (p *WorkerPool) processTask(task Task, workerCount *int, wg *sync.WaitGroup) {
	select {
	case p.workerQueue <- func() { p.executeTask(task) }:
	default:
		if *workerCount < p.maxWorkers {
			wg.Add(1)
			go worker(func() { p.executeTask(task) }, p.workerQueue, wg)
			*workerCount++
		} else {
			p.waitingQueue.PushBack(task)
			atomic.StoreInt32(&p.waiting, int32(p.waitingQueue.Len()))
		}
	}
}

func (p *WorkerPool) stop(wait bool) {
	p.stopOnce.Do(func() {
		close(p.stopSignal)
		p.stopLock.Lock()
		p.stopped = true
		p.stopLock.Unlock()
		p.wait = wait
	})
}

func worker(task func(), workerQueue chan func(), wg *sync.WaitGroup) {
	defer wg.Done()
	for task != nil {
		task()
		task = <-workerQueue
	}
}

func (p *WorkerPool) executeTask(task Task) {
	fmt.Printf("Executing task: %s with data: %s\n", task.ID, task.Data)
	// Simulating work
	time.Sleep(time.Second)
}

func (p *WorkerPool) Stopped() bool {
	p.stopLock.Lock()
	defer p.stopLock.Unlock()
	return p.stopped
}

func (p *WorkerPool) Close() error {
	p.Stop()
	return p.redisClient.Close()
}

func (p *WorkerPool) processWaitingQueue() bool {
	select {
	case task, ok := <-p.taskQueue:
		if !ok {
			return false
		}
		p.waitingQueue.PushBack(task)
	case p.workerQueue <- func() { p.executeTask(p.waitingQueue.Front()) }:
		// A worker was ready, so gave task to worker.
		p.waitingQueue.PopFront()
	}
	atomic.StoreInt32(&p.waiting, int32(p.waitingQueue.Len()))
	return true
}

func (p *WorkerPool) runQueuedTasks() {
	for p.waitingQueue.Len() != 0 {
		// worker is ready so give task
		p.workerQueue <- func() { p.executeTask(p.waitingQueue.PopFront()) }
		atomic.StoreInt32(&p.waiting, int32(p.waitingQueue.Len()))
	}
}

func (p *WorkerPool) killIdleWorker() bool {
	select {
	case p.workerQueue <- nil:
		// Sent kill signal to worker.
		return true
	default:
		// No ready workers. All, if any, workers are busy.
		return false
	}
}
