package monitoring

import (
	"context"
	"fmt"
	"log"
	"scaler/internal/vmss"
	"time"

	"github.com/google/uuid"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
)

type Monitor struct {
	client appinsights.TelemetryClient
}

func NewMonitor(instrumentationKey string) (*Monitor, error) {
	telemetryConfig := appinsights.NewTelemetryConfiguration(instrumentationKey)

	// Configure the client
	telemetryConfig.MaxBatchSize = 1024
	telemetryConfig.MaxBatchInterval = time.Second * 2

	client := appinsights.NewTelemetryClientFromConfig(telemetryConfig)

	// Send initial telemetry to verify connection
	startupEvent := appinsights.NewEventTelemetry("MonitorStartup")
	startupEvent.Properties["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	client.Track(startupEvent)

	log.Printf("Initialized Application Insights (data may take 2-5 minutes to appear)")

	return &Monitor{
		client: client,
	}, nil
}

func (m *Monitor) TrackVMSSOperation(ctx context.Context, metrics vmss.VMMetrics, geoName string) {
	operationID := uuid.New().String()

	// Custom event for Log Analytics querying
	event := appinsights.NewEventTelemetry("VMSSOperation")

	event.Properties["operationId"] = operationID
	event.Properties["operation"] = metrics.Operation
	event.Properties["resourceId"] = metrics.ResourceID
	event.Properties["region"] = geoName
	event.Properties["duration"] = fmt.Sprintf("%d", int(metrics.Duration.Seconds()))

	m.client.Track(event)
}
