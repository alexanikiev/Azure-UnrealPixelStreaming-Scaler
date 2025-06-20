package integration

import (
	"context"
	"testing"

	"scaler/internal/vmss"
	"scaler/pkg/config"
)

func TestVMSS(t *testing.T) {
	// Load VMSS config
	cfg, err := config.LoadVMSSConfig()
	if err != nil {
		t.Fatalf("Failed to load VMSS config: %v", err)
	}

	// Create VMSS provider
	provider, err := vmss.NewAzureVMSSProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create VMSS provider: %v", err)
	}

	// Test ListInstances
	ctx := context.Background()
	options := vmss.ListInstancesOptions{}
	instances, err := provider.ListInstances(ctx, options)
	if err != nil {
		t.Fatalf("Failed to list instances: %v", err)
	}

	// Verify we got some instances
	if len(instances) == 0 {
		t.Log("No instances found in scale set")
	} else {
		for _, instance := range instances {
			t.Logf("Found instance: ID=%s, Status=%s", instance.VMID, instance.Status)
		}
	}
}
