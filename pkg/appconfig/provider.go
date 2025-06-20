package appconfig

import (
	"context"
	"fmt"
	"scaler/pkg/config"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
)

type StaticTokenCredential struct {
	Token azcore.AccessToken
}

type ScalerPoolConfig struct {
	PoolCapacity    int
	WarmPoolSize    int
	WarmPoolEnabled bool
}

// Provider defines operations for managing Azure App Configuration
type Provider interface {
	GetConfiguration(ctx context.Context, key string) (string, error)
	ParseConfiguration(ctx context.Context) (*ScalerPoolConfig, error)
}

type AzureAppConfigProvider struct {
	client *azappconfig.Client
}

func (s *StaticTokenCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return s.Token, nil
}

func NewAzureAppConfigProvider(cfg *config.AppConfigConfig) (*AzureAppConfigProvider, error) {
	// Use ManagedIdentityCredential specifically for ACI
	cred, err := azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
		ID: nil, // System-assigned identity
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create managed identity credential: %v", err)
	}

	// One-time token acquisition
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://azconfig.io"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to acquire token: %v", err)
	}
	fmt.Printf("Successfully acquired App Config token (expires: %v)\n", token.ExpiresOn)

	staticCred := &StaticTokenCredential{Token: token}

	// Construct endpoint URL
	endpoint := fmt.Sprintf("https://%s.azconfig.io", cfg.StoreName)

	// Create App Configuration client
	client, err := azappconfig.NewClient(endpoint, staticCred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create app configuration client: %v", err)
	}

	return &AzureAppConfigProvider{
		client: client,
	}, nil
}

func (p *AzureAppConfigProvider) GetConfiguration(ctx context.Context, key string) (string, error) {
	response, err := p.client.GetSetting(ctx, key, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get configuration for key %s: %v", key, err)
	}

	if response.Value == nil {
		return "", fmt.Errorf("configuration value not found for key %s", key)
	}

	return *response.Value, nil
}

func (p *AzureAppConfigProvider) ParseConfiguration(ctx context.Context) (*ScalerPoolConfig, error) {
	config := &ScalerPoolConfig{}
	var err error

	// Get pool capacity
	poolCapStr, err := p.GetConfiguration(ctx, "SCALER_POOL_CAPACITY")
	if err != nil {
		return nil, fmt.Errorf("failed to get pool capacity: %w", err)
	}
	if _, err := fmt.Sscanf(poolCapStr, "%d", &config.PoolCapacity); err != nil {
		return nil, fmt.Errorf("invalid pool capacity value: %s", poolCapStr)
	}

	// Get warm pool size
	warmPoolStr, err := p.GetConfiguration(ctx, "SCALER_WARMPOOL_SIZE")
	if err != nil {
		return nil, fmt.Errorf("failed to get warm pool size: %w", err)
	}
	if _, err := fmt.Sscanf(warmPoolStr, "%d", &config.WarmPoolSize); err != nil {
		return nil, fmt.Errorf("invalid warm pool size value: %s", warmPoolStr)
	}

	// Get warm pool enabled flag
	enabledStr, err := p.GetConfiguration(ctx, "SCALER_WARMPOOL_ENABLED")
	if err != nil {
		return nil, fmt.Errorf("failed to get warm pool enabled flag: %w", err)
	}
	if _, err := fmt.Sscanf(enabledStr, "%t", &config.WarmPoolEnabled); err != nil {
		return nil, fmt.Errorf("invalid warm pool enabled value: %s", enabledStr)
	}

	return config, nil
}
