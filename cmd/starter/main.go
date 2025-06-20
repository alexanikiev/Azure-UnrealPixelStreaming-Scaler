package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"scaler/internal/scaling/starter"
	"scaler/internal/vmss"
	"scaler/pkg/config"
	"scaler/pkg/monitoring"
	"scaler/pkg/redis"
)

func main() {
	// Load configs
	redisConfig, err := config.LoadRedisConfig()
	if err != nil {
		log.Fatalf("Failed to load Redis config: %v", err)
	}

	vmssConfig, err := config.LoadVMSSConfig()
	if err != nil {
		log.Fatalf("Failed to load VMSS config: %v", err)
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

	vmssProvider, err := vmss.NewAzureVMSSProvider(vmssConfig)
	if err != nil {
		log.Fatalf("Failed to create VMSS provider: %v", err)
	}

	monitor, err := monitoring.NewMonitor(vmssConfig.InstrumentationKey)
	if err != nil {
		log.Fatalf("Failed to create monitoring client: %v", err)
	}

	// Create and start service
	svc, err := starter.NewService(
		vmssProvider,
		redisClient,
		monitor,
		scalerConfig,
	)
	if err != nil {
		log.Fatalf("Failed to create simulator service: %v", err)
	}

	if err := svc.Start(); err != nil {
		log.Fatalf("Failed to start starter service: %v", err)
	}

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	if err := svc.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
