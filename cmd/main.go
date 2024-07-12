package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	workerpool "github.com/SamuelJenkinsML/worker-pool/workerpool"
	"github.com/go-redis/redis/v8"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	pool, _ := workerpool.New(10, redisAddr)

	// ctx for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	go generateTasks(ctx, redisAddr)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// wait for interrupt signal
	<-signals
	cancel()
	pool.StopWait()

	log.Println("Worker pool stopped")
}

func generateTasks(ctx context.Context, redisAddr string) {
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			task := workerpool.Task{
				ID:   fmt.Sprintf("task-%d", i),
				Data: fmt.Sprintf("data for task %d", i),
			}
			taskJSON, _ := json.Marshal(task)
			err := rdb.RPush(ctx, "workerpool:tasks", taskJSON).Err()
			if err != nil {
				log.Printf("Error pushing task to Redis: %v", err)
			}
			time.Sleep(time.Second) // Add a new task every second
		}
	}
}
