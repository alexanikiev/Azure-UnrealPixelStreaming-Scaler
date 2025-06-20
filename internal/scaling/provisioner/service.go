package provisioner

import (
	"context"
	"fmt"
	"log"
	"time"

	"scaler/internal/vmss"
	"scaler/pkg/appconfig"
	"scaler/pkg/appgw"
	"scaler/pkg/config"
	"scaler/pkg/monitoring"
)

type Service struct {
	ctx             context.Context
	cancel          context.CancelFunc
	vmss            vmss.Provider
	appgw           appgw.Provider
	telemetry       *monitoring.Monitor
	scalerConfig    *config.ScalerConfig
	appConfig       appconfig.Provider
	poolCapacity    int
	warmPoolSize    int
	warmPoolEnabled bool
}

func NewService(
	vmssProvider vmss.Provider,
	appGWProvider appgw.Provider,
	monitor *monitoring.Monitor,
	scalerConfig *config.ScalerConfig,
	appConfigProvider appconfig.Provider,
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

	// Initialize pools settings from App Config or fall back to default scaler config
	poolCapacity := scalerConfig.PoolCapacity
	warmPoolSize := scalerConfig.WarmPoolSize
	warmPoolEnabled := scalerConfig.WarmPoolEnabled

	if appConfigProvider != nil {
		if config, err := appConfigProvider.ParseConfiguration(ctx); err == nil {
			poolCapacity = config.PoolCapacity
			warmPoolSize = config.WarmPoolSize
			warmPoolEnabled = config.WarmPoolEnabled
			log.Printf("Using App Config settings - Pool Capacity: %d, Warm Pool Size: %d, Warm Pool Enabled: %t",
				poolCapacity, warmPoolSize, warmPoolEnabled)
		} else {
			log.Printf("Failed to parse App Config, using default scaler config: %v", err)
		}
	}

	return &Service{
		ctx:             ctx,
		cancel:          cancel,
		vmss:            vmssProvider,
		appgw:           appGWProvider,
		telemetry:       monitor,
		scalerConfig:    scalerConfig,
		appConfig:       appConfigProvider,
		poolCapacity:    poolCapacity,
		warmPoolSize:    warmPoolSize,
		warmPoolEnabled: warmPoolEnabled,
	}, nil
}

func (s *Service) Start() error {
	log.Printf("Provisioner service scheduled to start in %d seconds...", s.scalerConfig.JobDelay)

	// Start the service with delay
	go func() {
		// Initial delay
		time.Sleep(time.Duration(s.scalerConfig.JobDelay) * time.Second)

		log.Printf("Starting provisioner service...")
		s.run()
	}()

	return nil
}

func (s *Service) Stop() error {
	log.Printf("Stopping provisioner service...")
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
					if err := s.provision(); err != nil {
						log.Printf("Error during provisioning: %v", err)
					}
					done <- true // Signal completion
				}()
			default:
				log.Printf("Operation still running ...")
			}
		}
	}
}

func (s *Service) provision() error {
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(s.scalerConfig.JobTimeout)*time.Second)
	defer cancel()

	start := time.Now()
	metrics := vmss.VMMetrics{
		Operation: "provision",
		Success:   true,
	}
	defer func() {
		metrics.Duration = time.Since(start)
		s.telemetry.TrackVMSSOperation(ctx, metrics, s.scalerConfig.GeoName)
	}()

	// Get initial instances list
	oldInstances, err := s.vmss.ListInstances(ctx, vmss.ListInstancesOptions{})
	if err != nil {
		return fmt.Errorf("failed to list instances before scaling: %w", err)
	}
	oldInstanceMap := make(map[string]bool)
	for _, instance := range oldInstances {
		oldInstanceMap[instance.InstanceID] = true
	}

	// Create VMSS instances
	if err = s.vmss.CreateInstances(ctx, int64(s.poolCapacity)); err != nil {
		metrics.Success = false
		metrics.ErrorMessage = err.Error()
		log.Printf("Failed to provision instance(s): %v", err)
		return err
	}

	// Get updated instances list
	newInstances, err := s.vmss.ListInstances(ctx, vmss.ListInstancesOptions{})
	if err != nil {
		return fmt.Errorf("failed to list instances after scaling: %w", err)
	}

	// Manage path-based rules in Application Gateway
	err = s.appgw.UpdatePathBasedRules(ctx, newInstances)
	if err != nil {
		metrics.Success = false
		metrics.ErrorMessage = err.Error()
		log.Printf("Failed to update path-based rules: %v", err)
		return err
	}

	// Identify newly provisioned instances
	var provisionedInstances []string
	for _, instance := range newInstances {
		if !oldInstanceMap[instance.InstanceID] {
			provisionedInstances = append(provisionedInstances, instance.InstanceID)
		}
	}

	// Get warm instances list
	warmInstances, err := s.vmss.ListInstances(ctx, vmss.ListInstancesOptions{
		VMPowerStates: []vmss.VMPowerState{
			vmss.PowerStateStopped,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list of warm instances: %w", err)
	}

	currentWarmPoolSize := len(warmInstances)

	// Validate warm pool settings
	effectiveWarmPoolSize := 0
	if s.warmPoolEnabled && s.warmPoolSize > 0 && s.warmPoolSize <= s.poolCapacity {
		effectiveWarmPoolSize = s.warmPoolSize - currentWarmPoolSize
		log.Printf("Effective warm pool size: %d", effectiveWarmPoolSize)
	} else if s.warmPoolEnabled {
		log.Printf("Invalid warm pool configuration - size: %d, capacity: %d. Disabling warm pool.",
			s.warmPoolSize, s.poolCapacity)
	}

	// Handle warm and cold instances
	warmCount := 0
	for _, instanceID := range provisionedInstances {
		if warmCount < effectiveWarmPoolSize {
			// Let the VM script handle shutdown for warm pool instances
			log.Printf("Instance %s added to warm pool", instanceID)
			warmCount++
		} else {
			// Deallocate instances beyond warm pool size
			if err := s.vmss.StopInstance(ctx, instanceID); err != nil {
				log.Printf("Failed to deallocate instance %s: %v", instanceID, err)
				continue
			}
			log.Printf("Instance %s deallocated (cold pool)", instanceID)
		}
	}

	if len(provisionedInstances) > 0 {
		log.Printf("Provisioned %d instances - Warm: %d, Cold: %d",
			len(provisionedInstances), warmCount, len(provisionedInstances)-warmCount)
	}

	return nil
}
