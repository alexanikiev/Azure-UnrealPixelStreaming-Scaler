package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"scaler/internal/scaling/provisioner"
	"scaler/internal/vmss"
	"scaler/pkg/appconfig"
	"scaler/pkg/appgw"
	"scaler/pkg/config"
	"scaler/pkg/monitoring"
)

func main() {
	// Load configs
	vmssConfig, err := config.LoadVMSSConfig()
	if err != nil {
		log.Fatalf("Failed to load VMSS config: %v", err)
	}

	appgwConfig, err := config.LoadAppGWConfig()
	if err != nil {
		log.Fatalf("Failed to load AppGW config: %v", err)
	}

	scalerConfig, err := config.LoadScalerConfig()
	if err != nil {
		log.Fatalf("Failed to load Scaler config: %v", err)
	}

	appConfigConfig, err := config.LoadAppConfigConfig()
	if err != nil {
		log.Fatalf("Failed to load App Configuration: %v", err)
	}

	// Create clients
	vmssProvider, err := vmss.NewAzureVMSSProvider(vmssConfig)
	if err != nil {
		log.Fatalf("Failed to create VMSS provider: %v", err)
	}

	appGWProvider, err := appgw.NewAzureAppGWProvider(appgwConfig)
	if err != nil {
		log.Fatalf("Failed to create AppGW provider: %v", err)
	}

	monitor, err := monitoring.NewMonitor(vmssConfig.InstrumentationKey)
	if err != nil {
		log.Fatalf("Failed to create monitoring client: %v", err)
	}

	appConfigProvider, err := appconfig.NewAzureAppConfigProvider(appConfigConfig)
	if err != nil {
		log.Fatalf("Failed to create App Configuration provider: %v", err)
	}

	// Create and start service
	svc, err := provisioner.NewService(
		vmssProvider,
		appGWProvider,
		monitor,
		scalerConfig,
		appConfigProvider,
	)
	if err != nil {
		log.Fatalf("Failed to create provisioner service: %v", err)
	}

	if err := svc.Start(); err != nil {
		log.Fatalf("Failed to start provisioner service: %v", err)
	}

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	if err := svc.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
