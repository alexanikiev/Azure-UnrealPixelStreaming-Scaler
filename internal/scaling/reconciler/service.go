package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"scaler/internal/vmss"
	"scaler/pkg/config"
	"scaler/pkg/redis"
)

type Service struct {
	ctx          context.Context
	cancel       context.CancelFunc
	vmss         vmss.Provider
	redis        redis.Client
	scalerConfig *config.ScalerConfig
}

var statusSets = []string{
	redis.VMStatusAvailableSet,
	redis.VMStatusReservedSet,
	redis.VMStatusUnavailableSet,
}

func NewService(
	vmssProvider vmss.Provider,
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
		ctx:          ctx,
		cancel:       cancel,
		vmss:         vmssProvider,
		redis:        redisClient,
		scalerConfig: scalerConfig,
	}, nil
}

func (s *Service) Start() error {
	log.Printf("Reconciler service scheduled to start in %d seconds...", s.scalerConfig.JobDelay)

	// Start the service with delay
	go func() {
		// Initial delay
		time.Sleep(time.Duration(s.scalerConfig.JobDelay) * time.Second)

		log.Printf("Starting reconciler service...")
		s.run()
	}()

	return nil
}

func (s *Service) Stop() error {
	log.Printf("Stopping reconciler service...")
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
					if err := s.reconcile(); err != nil {
						log.Printf("Error during reconciliation: %v", err)
					}
					done <- true // Signal completion
				}()
			default:
				log.Printf("Operation still running ...")
			}
		}
	}
}

func (s *Service) reconcile() error {
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(s.scalerConfig.JobTimeout)*time.Second)
	defer cancel()

	// Get all VMSS instances for orphan detection
	allInstances, err := s.vmss.ListInstances(ctx, vmss.ListInstancesOptions{})
	if err != nil {
		return fmt.Errorf("failed to list all VMSS instances: %v", err)
	}

	// Create map of all VMSS instances for lookup during orphan detection
	allInstancesMap := make(map[string]bool)
	for _, instance := range allInstances {
		allInstancesMap[instance.VMID] = true
	}

	// Get Redis records
	redisKeys := "vmss:instance:*"
	redisRecords, err := s.redis.Keys(ctx, redisKeys)
	if err != nil {
		return fmt.Errorf("failed to get Redis records: %v", err)
	}

	// Handle orphaned records using allInstancesMap
	pipe := s.redis.Pipeline()
	orphanedKeys := make([]string, 0)

	// Queue all deletions
	for _, key := range redisRecords {
		var vmID string
		if _, err := fmt.Sscanf(key, "vmss:instance:%s", &vmID); err != nil {
			log.Printf("Failed to parse VMID from Redis key %s: %v", key, err)
			continue
		}

		// Check if VM exists in VMSS
		if !allInstancesMap[vmID] {
			// Queue operations for orphaned record
			if err := pipe.Delete(ctx, key); err != nil {
				log.Printf("Failed to queue delete for Redis record %s: %v", key, err)
				continue
			}

			// Remove from all possible status sets to ensure cleanup
			for _, set := range statusSets {
				if err := pipe.SRem(ctx, set, key); err != nil {
					log.Printf("Failed to queue status set removal for %s from %s: %v", key, set, err)
					// Continue with other sets even if one fails
				}
			}

			orphanedKeys = append(orphanedKeys, key)
		}
	}

	// Execute all deletions in single pipeline if there are any orphaned records
	if len(orphanedKeys) > 0 {
		if err := pipe.Exec(ctx); err != nil {
			log.Printf("Failed to execute Redis pipeline for orphaned records: %v", err)
			return fmt.Errorf("failed to remove orphaned records: %w", err)
		}
		log.Printf("Removed %d orphaned Redis records: %s",
			len(orphanedKeys),
			strings.Join(orphanedKeys, ", "))

		// Refetch Redis records only after successful deletion
		redisRecords, err = s.redis.Keys(ctx, redisKeys)
		if err != nil {
			return fmt.Errorf("failed to refresh Redis records after orphan deletion: %v", err)
		}
	}

	// Get only Stopped/Deallocated instances for new record creation
	stoppedInstances, err := s.vmss.ListInstances(ctx, vmss.ListInstancesOptions{
		VMPowerStates: []vmss.VMPowerState{
			vmss.PowerStateStopped,
			vmss.PowerStateDeallocated,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list inactive VMSS instances: %v", err)
	}

	// Create new records only for stopped/deallocated instances
	redisMap := make(map[string]bool)
	for _, key := range redisRecords {
		redisMap[key] = true
	}

	pipe = s.redis.Pipeline()
	newRecords := make([]string, 0)

	for _, instance := range stoppedInstances {
		redisKey := fmt.Sprintf("vmss:instance:%s", instance.VMID)
		if !redisMap[redisKey] {
			// Set warm flag based on power state
			isWarm := instance.State == vmss.PowerStateStopped

			// Create record with required fields
			record := &vmss.VMRedisRecord{
				VMID:       instance.VMID,
				InstanceID: instance.InstanceID,
				PublicIP:   instance.PublicIP,
				// ClientIP:   instance.ClientIP,
				// SessionID: instance.SessionID,
				Status:    string(vmss.VMStatusAvailable),
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
				Region:    s.scalerConfig.GeoName,
				Used:      false,
				Warm:      isWarm,
			}

			// Convert to JSON before storing
			recordJSON, err := json.Marshal(record)
			if err != nil {
				log.Printf("Failed to marshal record for instance %s: %v", instance.VMID, err)
				continue
			}

			// Queue record creation operations
			if err := pipe.Set(ctx, redisKey, string(recordJSON)); err != nil {
				log.Printf("Failed to queue Redis record for instance %s: %v", instance.VMID, err)
				continue
			}
			if err := pipe.SAdd(ctx, redis.VMStatusAvailableSet, redisKey); err != nil {
				log.Printf("Failed to queue status set update for instance %s: %v", instance.VMID, err)
				continue
			}
			newRecords = append(newRecords, instance.VMID)

			suffix := "cold"
			if isWarm {
				suffix = "warm"
			}
			log.Printf("Queued new %s instance record: %s", suffix, instance.InstanceID)
		}
	}

	// Execute all creations in single pipeline if there are any new records
	if len(newRecords) > 0 {
		if err := pipe.Exec(ctx); err != nil {
			log.Printf("Failed to execute Redis pipeline for new records: %v", err)
			return fmt.Errorf("failed to create new records: %w", err)
		}
		log.Printf("Created %d new Redis records: %s",
			len(newRecords),
			strings.Join(newRecords, ", "))
	}

	return nil
}
