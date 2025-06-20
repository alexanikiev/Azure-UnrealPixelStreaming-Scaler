package appgw

import (
	"context"
	"fmt"
	"log"
	"scaler/internal/vmss"
	"scaler/pkg/config"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
)

// Provider defines operations for managing Application Gateway path-based rules
type Provider interface {
	UpdatePathBasedRules(ctx context.Context, instances []*vmss.VMInstance) error
}

type AzureAppGWProvider struct {
	client *armnetwork.ApplicationGatewaysClient
	config *config.AppGWConfig
}

func NewAzureAppGWProvider(cfg *config.AppGWConfig) (*AzureAppGWProvider, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %v", err)
	}

	client, err := armnetwork.NewApplicationGatewaysClient(cfg.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create app gateway client: %v", err)
	}

	return &AzureAppGWProvider{
		client: client,
		config: cfg,
	}, nil
}

func (p *AzureAppGWProvider) UpdatePathBasedRules(ctx context.Context, instances []*vmss.VMInstance) error {
	gateway, err := p.client.Get(ctx, p.config.ResourceGroup, p.config.GWName, nil)
	if err != nil {
		return fmt.Errorf("failed to get app gateway: %w", err)
	}

	// Find the URL path map
	var pathMap *armnetwork.ApplicationGatewayURLPathMap
	for _, pm := range gateway.Properties.URLPathMaps {
		if *pm.Name == p.config.PathMapName {
			pathMap = pm
			break
		}
	}

	if pathMap == nil {
		return fmt.Errorf("URL path map %s not found", p.config.PathMapName)
	}

	// Create map of current instances by VMID for quick lookup
	activeInstances := make(map[string]*vmss.VMInstance)
	for _, instance := range instances {
		if instance.PrivateIP != "" {
			activeInstances[instance.VMID] = instance
		}
	}

	updateFlag := false

	// Track changes
	changes := struct {
		added   int
		removed int
	}{0, 0}

	// Filter out old rules
	validRules := make([]*armnetwork.ApplicationGatewayPathRule, 0)
	for _, rule := range pathMap.Properties.PathRules {
		if len(rule.Properties.Paths) == 0 {
			continue
		}

		// Extract VMID from path pattern
		vmid := strings.TrimPrefix(*rule.Properties.Paths[0], "/")

		if vmid == "default" {
			// Keep default rule
			validRules = append(validRules, rule)
			continue
		}

		// Check if instance still exists
		if _, exists := activeInstances[vmid]; exists {
			validRules = append(validRules, rule)
			delete(activeInstances, vmid) // Remove from map to track new instances
		} else {
			updateFlag = true
			changes.removed++
		}
	}

	// Add rules for new instances
	if len(activeInstances) > 0 {
		updateFlag = true
		for vmid, instance := range activeInstances {
			targetName := fmt.Sprintf("instance%s", instance.InstanceID)
			pathPattern := fmt.Sprintf("/%s", vmid)
			poolName := strings.ReplaceAll(instance.PrivateIP, ".", "-")

			changes.added++

			newRule := &armnetwork.ApplicationGatewayPathRule{
				Name: to.Ptr(targetName),
				Properties: &armnetwork.ApplicationGatewayPathRulePropertiesFormat{
					Paths: []*string{to.Ptr(pathPattern)},
					BackendAddressPool: &armnetwork.SubResource{
						ID: to.Ptr(fmt.Sprintf(
							"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/applicationGateways/%s/backendAddressPools/%s",
							p.config.SubscriptionID,
							p.config.ResourceGroup,
							p.config.GWName,
							poolName,
						)),
					},
					BackendHTTPSettings: &armnetwork.SubResource{
						ID: to.Ptr(fmt.Sprintf(
							"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/applicationGateways/%s/backendHttpSettingsCollection/wss",
							p.config.SubscriptionID,
							p.config.ResourceGroup,
							p.config.GWName,
						)),
					},
				},
			}
			validRules = append(validRules, newRule)
		}
	}

	if updateFlag {
		// Update path map with valid rules
		pathMap.Properties.PathRules = validRules

		// Update the gateway
		_, err = p.client.BeginCreateOrUpdate(ctx, p.config.ResourceGroup, p.config.GWName, gateway.ApplicationGateway, nil)
		if err != nil {
			return fmt.Errorf("failed to update application gateway: %w", err)
		}

		log.Printf("Started path rules update: added %d, removed %d, total %d rules",
			changes.added,
			changes.removed,
			len(validRules),
		)
	} else {
		log.Printf("No changes needed for path rules (total: %d)", len(validRules))
	}

	return nil
}
