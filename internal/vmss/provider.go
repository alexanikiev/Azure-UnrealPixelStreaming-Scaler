package vmss

import (
	"context"
	"fmt"
	"log"
	"strings"

	"scaler/pkg/config"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
)

type Provider interface {
	CreateInstances(ctx context.Context, desiredCount int64) error
	StartInstance(ctx context.Context, instanceID string) error
	StopInstance(ctx context.Context, instanceID string) error
	DeleteInstance(ctx context.Context, instanceID string) error
	GetInstance(ctx context.Context, instanceID string) (*VMInstance, error)
	ListInstances(ctx context.Context, opts ListInstancesOptions) ([]*VMInstance, error)
}

type AzureVMSSProvider struct {
	vmssClient *armcompute.VirtualMachineScaleSetsClient
	vmsClient  *armcompute.VirtualMachineScaleSetVMsClient
	nicClient  *armnetwork.InterfacesClient
	config     *config.VMSSConfig
}

func NewAzureVMSSProvider(cfg *config.VMSSConfig) (*AzureVMSSProvider, error) {
	options := &azidentity.DefaultAzureCredentialOptions{
		TenantID: cfg.TenantID,
	}

	cred, err := azidentity.NewDefaultAzureCredential(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %v", err)
	}

	vmssClient, err := armcompute.NewVirtualMachineScaleSetsClient(cfg.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create VMSS client: %v", err)
	}

	vmsClient, err := armcompute.NewVirtualMachineScaleSetVMsClient(cfg.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create VMSS VMs client: %v", err)
	}

	nicClient, err := armnetwork.NewInterfacesClient(cfg.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create network interfaces client: %v", err)
	}

	return &AzureVMSSProvider{
		vmssClient: vmssClient,
		vmsClient:  vmsClient,
		nicClient:  nicClient,
		config:     cfg,
	}, nil
}

func (p *AzureVMSSProvider) CreateInstances(ctx context.Context, desiredCount int64) error {
	// Get the current VMSS to retrieve existing instance count
	vmss, err := p.vmssClient.Get(ctx, p.config.ResourceGroup, p.config.ScaleSetName, nil)
	if err != nil {
		return fmt.Errorf("failed to get current VMSS: %w", err)
	}

	// Extract current capacity
	var currentCount int64 = 0
	if vmss.SKU != nil && vmss.SKU.Capacity != nil {
		currentCount = *vmss.SKU.Capacity
	}

	// Check if the desired count is greater than the current count
	if currentCount >= desiredCount {
		log.Printf("scale set already has %d or more instances", currentCount)
		return nil
	}

	log.Printf("Provisioning %d new instance(s)", desiredCount-currentCount)

	// Prepare update object with existing SKU details
	update := armcompute.VirtualMachineScaleSetUpdate{
		SKU: &armcompute.SKU{
			Name:     vmss.SKU.Name,
			Tier:     vmss.SKU.Tier,
			Capacity: &desiredCount,
		},
	}

	// Call BeginUpdate to change instance count
	poller, err := p.vmssClient.BeginUpdate(ctx, p.config.ResourceGroup, p.config.ScaleSetName, update, nil)
	if err != nil {
		return fmt.Errorf("failed to begin update: %w", err)
	}

	// Wait for operation to complete
	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to scale VMSS: %w", err)
	}

	return nil
}

func (p *AzureVMSSProvider) StartInstance(ctx context.Context, instanceID string) error {
	_, err := p.vmsClient.BeginStart(ctx, p.config.ResourceGroup, p.config.ScaleSetName, instanceID, nil)
	if err != nil {
		return fmt.Errorf("failed to start instance %s: %v", instanceID, err)
	}
	log.Printf("Started instance %s in scale set %s", instanceID, p.config.ScaleSetName)
	return nil
}

func (p *AzureVMSSProvider) StopInstance(ctx context.Context, instanceID string) error {
	_, err := p.vmsClient.BeginDeallocate(ctx, p.config.ResourceGroup, p.config.ScaleSetName, instanceID, nil)
	if err != nil {
		return fmt.Errorf("failed to stop instance %s: %v", instanceID, err)
	}
	log.Printf("Stopped instance %s in scale set %s", instanceID, p.config.ScaleSetName)
	return nil
}

func (p *AzureVMSSProvider) DeleteInstance(ctx context.Context, instanceID string) error {
	_, err := p.vmsClient.BeginDelete(ctx, p.config.ResourceGroup, p.config.ScaleSetName, instanceID, nil)
	if err != nil {
		return fmt.Errorf("failed to delete instance %s: %v", instanceID, err)
	}
	log.Printf("Deleted instance %s from scale set %s", instanceID, p.config.ScaleSetName)
	return nil
}

func (p *AzureVMSSProvider) GetInstance(ctx context.Context, instanceID string) (*VMInstance, error) {
	// Include instanceView in the get request
	options := &armcompute.VirtualMachineScaleSetVMsClientGetOptions{
		Expand: to.Ptr(armcompute.InstanceViewTypesInstanceView),
	}

	instance, err := p.vmsClient.Get(ctx, p.config.ResourceGroup, p.config.ScaleSetName, instanceID, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s: %v", instanceID, err)
	}

	// Get power state from instance view
	var powerState VMPowerState
	if instance.Properties.InstanceView != nil && len(instance.Properties.InstanceView.Statuses) > 0 {
		for _, status := range instance.Properties.InstanceView.Statuses {
			if status.Code != nil && strings.HasPrefix(*status.Code, "PowerState/") {
				powerState = VMPowerState(*status.Code)
				break
			}
		}
	}

	return &VMInstance{
		VMID:       *instance.ID,
		Status:     VMStatusAvailable,
		State:      powerState,
		InstanceID: *instance.InstanceID,
		PublicIP:   "0.0.0.0",
	}, nil
}

func (p *AzureVMSSProvider) ListInstances(ctx context.Context, opts ListInstancesOptions) ([]*VMInstance, error) {
	// Set expand parameter to include instance view
	options := &armcompute.VirtualMachineScaleSetVMsClientListOptions{
		Expand: to.Ptr("instanceView"),
	}
	pager := p.vmsClient.NewListPager(p.config.ResourceGroup, p.config.ScaleSetName, options)

	instances := make([]*VMInstance, 0)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get instances page: %v", err)
		}

		for _, instance := range page.Value {
			// Get power state from instance view
			var powerState VMPowerState
			if instance.Properties.InstanceView != nil && len(instance.Properties.InstanceView.Statuses) > 0 {
				for _, status := range instance.Properties.InstanceView.Statuses {
					if status.Code != nil && strings.HasPrefix(*status.Code, "PowerState/") {
						powerState = VMPowerState(*status.Code)
						break
					}
				}
			}

			// Filter by power states if specified
			if len(opts.VMPowerStates) > 0 {
				matches := false
				for _, state := range opts.VMPowerStates {
					if powerState == state {
						matches = true
						break
					}
				}
				if !matches {
					continue
				}
			}

			// Get private IP using network interfaces client
			privateIP, err := p.getInstancePrivateIP(ctx, instance)
			if err != nil {
				log.Printf("Warning: %v for instance %s", err, *instance.InstanceID)
			}

			instances = append(instances, &VMInstance{
				VMID:       *instance.Properties.VMID,
				Status:     VMStatusAvailable,
				State:      powerState,
				InstanceID: *instance.InstanceID,
				PrivateIP:  privateIP,
				PublicIP:   "0.0.0.0",
			})
		}
	}

	log.Printf("Listed %d instances in scale set %s (filtered by power states: %v)", len(instances), p.config.ScaleSetName, opts.VMPowerStates)
	return instances, nil
}

func (p *AzureVMSSProvider) getInstancePrivateIP(ctx context.Context, instance *armcompute.VirtualMachineScaleSetVM) (string, error) {
	if instance.InstanceID == nil {
		return "", fmt.Errorf("instance has no ID")
	}

	// Use the correct method to list VMSS network interfaces
	pager := p.nicClient.NewListVirtualMachineScaleSetNetworkInterfacesPager(
		p.config.ResourceGroup,
		p.config.ScaleSetName,
		nil,
	)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list NICs: %w", err)
		}

		for _, nic := range page.Value {
			// Match the instance ID
			if nic.Properties != nil && nic.Properties.VirtualMachine != nil {
				vmID := *nic.Properties.VirtualMachine.ID
				if strings.HasSuffix(vmID, *instance.InstanceID) {
					// Get private IP from configurations
					for _, ipConf := range nic.Properties.IPConfigurations {
						if ipConf.Properties != nil && ipConf.Properties.PrivateIPAddress != nil {
							return *ipConf.Properties.PrivateIPAddress, nil
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no private IP found for instance %s", *instance.InstanceID)
}
