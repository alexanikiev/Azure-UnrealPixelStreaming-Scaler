package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"scaler/internal/scaling/simulator"
	"scaler/pkg/config"
	"scaler/pkg/redis"
)

func main() {
	// Load configs
	redisConfig, err := config.LoadRedisConfig()
	if err != nil {
		log.Fatalf("Failed to load Redis config: %v", err)
	}

	scalerConfig, err := config.LoadScalerConfig()
	if err != nil {
		log.Fatalf("Failed to load Scaler config: %v", err)
	}

	// Create clients
	redisClient, err := redis.NewClient(redisConfig)
	if err != nil {
		log.Fatalf("Failed to create Redis client: %v", err)
	}
	defer redisClient.Close()

	// Create and start service
	svc, err := simulator.NewService(
		redisClient,
		scalerConfig,
	)
	if err != nil {
		log.Fatalf("Failed to create simulator service: %v", err)
	}

	if err := svc.Start(); err != nil {
		log.Fatalf("Failed to start simulator service: %v", err)
	}

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	if err := svc.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
