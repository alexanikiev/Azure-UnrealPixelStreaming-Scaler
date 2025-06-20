package starter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"scaler/internal/vmss"
	"scaler/pkg/config"
	"scaler/pkg/monitoring"
	"scaler/pkg/redis"
)

type Service struct {
	ctx          context.Context
	cancel       context.CancelFunc
	redis        redis.Client
	vmss         vmss.Provider
	telemetry    *monitoring.Monitor
	scalerConfig *config.ScalerConfig
}

func NewService(
	vmssProvider vmss.Provider,
	redisClient redis.Client,
	monitor *monitoring.Monitor,
	scalerConfig *config.ScalerConfig,
) (*Service, error) {
	// Validate mandatory parameters
	if scalerConfig.JobInterval <= 0 {
		return nil, fmt.Errorf("invalid job interval: %d, must be positive", scalerConfig.JobInterval)
	}
	if scalerConfig.JobTimeout <= 0 {
		return nil, fmt.Errorf("invalid job timeout: %d, must be positive", scalerConfig.JobTimeout)
	}
	if scalerConfig.JobDelay <= 0 {
		return nil, fmt.Errorf("invalid job delay: %d, must be positive", scalerConfig.JobDelay)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		ctx:          ctx,
		cancel:       cancel,
		redis:        redisClient,
		vmss:         vmssProvider,
		telemetry:    monitor,
		scalerConfig: scalerConfig,
	}, nil
}

func (s *Service) Start() error {
	log.Printf("Starter service scheduled to start in %d seconds...", s.scalerConfig.JobDelay)

	// Start the service with delay
	go func() {
		// Initial delay
		time.Sleep(time.Duration(s.scalerConfig.JobDelay) * time.Second)

		log.Printf("Starting starter service...")
		s.run()
	}()

	return nil
}

func (s *Service) Stop() error {
	log.Printf("Stopping starter service...")
	s.cancel()
	return s.redis.Close()
}

func (s *Service) run() {
	ticker := time.NewTicker(time.Duration(s.scalerConfig.JobInterval) * time.Second)
	defer ticker.Stop()

	// Channel to coordinate operations
	done := make(chan bool, 1)
	done <- true // Initial token

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Wait for previous operation to complete
			select {
			case <-done:
				// Start new operation
				go func() {
					if err := s.start(); err != nil {
						log.Printf("Error during starting: %v", err)
					}
					done <- true // Signal completion
				}()
			default:
				log.Printf("Operation still running ...")
			}
		}
	}
}

func (s *Service) start() error {
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(s.scalerConfig.JobTimeout)*time.Second)
	defer cancel()

	// Get all Reserved instances
	selectedInstances, err := s.redis.SPop(ctx, redis.VMStatusReservedSet, 100) // Arbitrary large limit
	if err != nil {
		return fmt.Errorf("failed to get reserved instances: %w", err)
	}

	if len(selectedInstances) == 0 {
		log.Printf("No reserved instances found to process")
		return nil
	}

	log.Printf("Found %d reserved instances to process", len(selectedInstances))

	// Process each instance
	for _, instance := range selectedInstances {
		// Get current instance data
		instanceData, err := s.redis.Get(ctx, instance)
		if err != nil {
			log.Printf("Error getting instance data for %s: %v", instance, err)
			continue
		}

		// Create pipeline for atomic operations
		pipe := s.redis.Pipeline()

		// Parse and update instance data
		var record vmss.VMRedisRecord
		if err := json.Unmarshal([]byte(instanceData), &record); err != nil {
			log.Printf("Error parsing instance data for %s: %v", instance, err)
			continue
		}

		record.Status = string(vmss.VMStatusUnavailable)
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

		// Convert to JSON
		updatedData, err := json.Marshal(record)
		if err != nil {
			log.Printf("Error marshaling updated data for %s: %v", instance, err)
			continue
		}

		// Queue instance record update
		if err := pipe.Set(ctx, instance, string(updatedData)); err != nil {
			log.Printf("Error queueing instance update for %s: %v", instance, err)
			continue
		}

		// Add to unavailable set
		if err := pipe.SAdd(ctx, redis.VMStatusUnavailableSet, instance); err != nil {
			log.Printf("Error queueing status set update for %s: %v", instance, err)
			continue
		}

		// Execute pipeline
		if err := pipe.Exec(ctx); err != nil {
			log.Printf("Error executing Redis pipeline for %s: %v", instance, err)
			continue
		}

		// Start the VM instance
		if err := s.vmss.StartInstance(ctx, record.InstanceID); err != nil {
			log.Printf("Error starting VM %s: %v", record.InstanceID, err)
			continue
		}

		// Submit telemetry
		metrics := vmss.VMMetrics{
			Operation:  "start",
			Success:    true,
			ResourceID: record.InstanceID,
		}

		s.telemetry.TrackVMSSOperation(ctx, metrics, s.scalerConfig.GeoName)

		log.Printf("Started VM %s and updated status to Unavailable", record.InstanceID)
	}

	return nil
}
