package main

import (
	"os"

	workerpool "github.com/SamuelJenkinsML/worker-pool"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	pool := workerpool.New(10, redisAddr)
}
