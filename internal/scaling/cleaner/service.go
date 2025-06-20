package cleaner

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
	if scalerConfig.VMRuntime <= 0 {
		return nil, fmt.Errorf("invalid VM runtime: %d, must be positive", scalerConfig.VMRuntime)
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
	log.Printf("Cleaner service scheduled to start in %d seconds...", s.scalerConfig.JobDelay)

	// Start the service with delay
	go func() {
		// Initial delay
		time.Sleep(time.Duration(s.scalerConfig.JobDelay) * time.Second)

		log.Printf("Starting cleaner service...")
		s.run()
	}()

	return nil
}

func (s *Service) Stop() error {
	log.Printf("Stopping cleaner service...")
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
					if err := s.clean(); err != nil {
						log.Printf("Error during cleaning: %v", err)
					}
					done <- true // Signal completion
				}()
			default:
				log.Printf("Operation still running ...")
			}
		}
	}
}

func (s *Service) clean() error {
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(s.scalerConfig.JobTimeout)*time.Second)
	defer cancel()

	// Get Unavailable instances directly from the set
	selectedInstances, err := s.redis.SMembers(ctx, redis.VMStatusUnavailableSet)
	if err != nil {
		return fmt.Errorf("failed to get unavailable instances: %w", err)
	}

	if len(selectedInstances) == 0 {
		log.Printf("No unavailable instances found to clean")
		return nil
	}

	log.Printf("Found %d unavailable instances to clean", len(selectedInstances))

	// Process each unavailable instance
	for _, instance := range selectedInstances {
		// Get current instance data
		instanceData, err := s.redis.Get(ctx, instance)
		if err != nil {
			log.Printf("Error getting instance data for %s: %v", instance, err)
			continue
		}

		// Parse instance data
		var record vmss.VMRedisRecord
		if err := json.Unmarshal([]byte(instanceData), &record); err != nil {
			log.Printf("Error parsing instance data for %s: %v", instance, err)
			continue
		}

		shouldCleanup := false
		cleanupReason := ""

		if record.Used {
			shouldCleanup = true
			cleanupReason = "marked as used"
		} else {
			// Check runtime for unused instances
			updatedAt, err := time.Parse(time.RFC3339, record.UpdatedAt)
			if err != nil {
				log.Printf("Error parsing UpdatedAt time for %s: %v", instance, err)
				continue
			}

			runtime := time.Since(updatedAt)
			if runtime >= time.Duration(s.scalerConfig.VMRuntime)*time.Second {
				shouldCleanup = true
				cleanupReason = fmt.Sprintf("runtime %v exceeded threshold %v",
					runtime.Round(time.Second), s.scalerConfig.VMRuntime)
			} else {
				log.Printf("Instance %s running time %v is below threshold %v, skipping",
					instance, runtime.Round(time.Second), s.scalerConfig.VMRuntime)
				continue
			}
		}

		if !shouldCleanup {
			continue
		}

		log.Printf("Cleaning up instance %s: %s", instance, cleanupReason)

		// Create pipeline for atomic operations
		pipe := s.redis.Pipeline()

		// Queue removal from unavailable set
		if err := pipe.SRem(ctx, redis.VMStatusUnavailableSet, instance); err != nil {
			log.Printf("Error queueing set removal for %s: %v", instance, err)
			continue
		}

		// Queue instance data deletion
		if err := pipe.Delete(ctx, instance); err != nil {
			log.Printf("Error queueing instance deletion for %s: %v", instance, err)
			continue
		}

		// Execute pipeline
		if err := pipe.Exec(ctx); err != nil {
			log.Printf("Error executing Redis pipeline for %s: %v", instance, err)
			continue
		}

		// Delete the VM instance from VMSS
		if err := s.vmss.DeleteInstance(ctx, record.InstanceID); err != nil {
			log.Printf("Error deleting VM %s: %v", record.InstanceID, err)
			continue
		}

		// Submit telemetry
		metrics := vmss.VMMetrics{
			Operation:  "clean",
			Success:    true,
			ResourceID: record.InstanceID,
		}

		s.telemetry.TrackVMSSOperation(ctx, metrics, s.scalerConfig.GeoName)

		log.Printf("Cleaned up instance %s (ID: %s)", instance, record.InstanceID)
	}

	return nil
}
