package vmss

import "time"

// VMStatus represents possible VM states
type VMStatus string

const (
	VMStatusAvailable   VMStatus = "Available"
	VMStatusReserved    VMStatus = "Reserved"
	VMStatusUnavailable VMStatus = "Unavailable"
)

type VMProvisioningState string

const (
	ProvisioningStateCreating  VMProvisioningState = "ProvisioningState/creating"
	ProvisioningStateSucceeded VMProvisioningState = "ProvisioningState/succeeded"
)

// VMPowerState represents the power state of a VM
type VMPowerState string

const (
	PowerStateRunning     VMPowerState = "PowerState/running"
	PowerStateStopped     VMPowerState = "PowerState/stopped"
	PowerStateDeallocated VMPowerState = "PowerState/deallocated"
)

type ListInstancesOptions struct {
	VMPowerStates       []VMPowerState
	VMProvisioningState []VMProvisioningState
}

// VMInstance represents a VM instance in the scale set
type VMInstance struct {
	VMID       string
	InstanceID string
	PublicIP   string
	PrivateIP  string
	Status     VMStatus
	State      VMPowerState
}

// VMRedisRecord represents a record in Redis for a VMSS instance
type VMRedisRecord struct {
	VMID       string `json:"vmId"`
	InstanceID string `json:"instanceId"`
	PublicIP   string `json:"publicIp"`
	ClientIP   string `json:"clientIp"`
	SessionID  string `json:"sessionId"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	Region     string `json:"region"`
	Used       bool   `json:"used"`
	Warm       bool   `json:"warm"`
}

// Metric types for monitoring
type VMMetrics struct {
	Operation    string
	Duration     time.Duration
	Success      bool
	ErrorMessage string
	ResourceID   string
	Region       string
}
