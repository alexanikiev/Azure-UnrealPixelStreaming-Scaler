package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"scaler/internal/vmss"
	"scaler/pkg/config"
	"scaler/pkg/redis"
)

type simulationStep struct {
	recordsToUpdate int
}

type Service struct {
	ctx          context.Context
	cancel       context.CancelFunc
	redis        redis.Client
	currentStep  int
	schedule     []simulationStep
	scalerConfig *config.ScalerConfig
}

func NewService(
	redisClient redis.Client,
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
		ctx:         ctx,
		cancel:      cancel,
		redis:       redisClient,
		currentStep: 0,
		schedule: []simulationStep{
			{recordsToUpdate: 1},
			{recordsToUpdate: 0},
			{recordsToUpdate: 2},
			{recordsToUpdate: 1},
			{recordsToUpdate: 0},
			{recordsToUpdate: 2},
		},
		scalerConfig: scalerConfig,
	}, nil
}

func (s *Service) Start() error {
	log.Printf("Simulator service scheduled to start in %d seconds...", s.scalerConfig.JobDelay)

	// Start the service with delay
	go func() {
		// Initial delay
		time.Sleep(time.Duration(s.scalerConfig.JobDelay) * time.Second)

		log.Printf("Starting simulator service...")
		s.run()
	}()

	return nil
}

func (s *Service) Stop() error {
	log.Printf("Stopping simulator service...")
	s.cancel()
	return nil
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
					if err := s.simulate(); err != nil {
						log.Printf("Error during sumulation: %v", err)
					}
					done <- true // Signal completion
				}()
			default:
				log.Printf("Operation still running ...")
			}
		}
	}
}

func (s *Service) simulate() error {
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(s.scalerConfig.JobTimeout)*time.Second)
	defer cancel()

	// Get current step
	step := s.schedule[s.currentStep]
	log.Printf("Executing simulation step %d: %d records to update", s.currentStep+1, step.recordsToUpdate)

	// Try to get requested number of instances
	selectedInstances, err := s.redis.SPop(ctx, redis.VMStatusAvailableSet, int64(step.recordsToUpdate))
	if err != nil {
		return fmt.Errorf("failed to pop available instances: %w", err)
	}

	// Check if we got enough instances
	if len(selectedInstances) < step.recordsToUpdate {
		log.Printf("Warning: Requested %d instances but only got %d from Available set",
			step.recordsToUpdate, len(selectedInstances))
	}

	if len(selectedInstances) == 0 {
		log.Printf("No available instances found, skipping this simulation step")
		s.currentStep = (s.currentStep + 1) % len(s.schedule)
		return nil
	}

	log.Printf("Popped %d instances from available set", len(selectedInstances))

	// Update each popped instance
	for _, instance := range selectedInstances {
		// Create pipeline for atomic updates
		pipe := s.redis.Pipeline()

		// Get current instance data
		instanceData, err := s.redis.Get(ctx, instance)
		if err != nil {
			log.Printf("Error getting instance data for %s: %v", instance, err)
			continue
		}

		// Update instance status
		var record vmss.VMRedisRecord
		if err := json.Unmarshal([]byte(instanceData), &record); err != nil {
			log.Printf("Error parsing instance data for %s: %v", instance, err)
			continue
		}

		record.Status = string(vmss.VMStatusReserved)
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

		updatedData, err := json.Marshal(record)
		if err != nil {
			log.Printf("Error marshaling updated data for %s: %v", instance, err)
			continue
		}

		// Queue updates in pipeline
		if err := pipe.Set(ctx, instance, string(updatedData)); err != nil {
			log.Printf("Error queueing instance update for %s: %v", instance, err)
			continue
		}

		// Add to reserved set (no need to remove from available - SPOP did that)
		if err := pipe.SAdd(ctx, redis.VMStatusReservedSet, instance); err != nil {
			log.Printf("Error queueing addition to reserved set for %s: %v", instance, err)
			continue
		}

		// Execute pipeline
		if err := pipe.Exec(ctx); err != nil {
			log.Printf("Error executing Redis pipeline for %s: %v", instance, err)
			continue
		}

		log.Printf("Updated instance %s to Reserved status", instance)
	}

	s.currentStep = (s.currentStep + 1) % len(s.schedule)
	return nil
}
