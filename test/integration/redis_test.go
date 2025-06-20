package integration

import (
	"context"
	"testing"

	"scaler/pkg/config"
	"scaler/pkg/redis"
)

func TestRedis(t *testing.T) {
	// Load Redis configuration
	cfg, err := config.LoadRedisConfig()
	if err != nil {
		t.Fatalf("Failed to load Redis config: %v", err)
	}

	// Create Redis client
	client, err := redis.NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Redis client: %v", err)
	}
	defer client.Close()

	// Test basic set/get operations
	ctx := context.Background()
	testKey := "test:connection"
	testValue := "working"

	// Test SET
	err = client.Set(ctx, testKey, testValue)
	if err != nil {
		t.Fatalf("Failed to set test key: %v", err)
	}

	// Test GET
	val, err := client.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("Failed to get test key: %v", err)
	}

	if val != testValue {
		t.Errorf("Expected value %q, got %q", testValue, val)
	}
}
