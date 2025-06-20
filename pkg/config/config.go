package config

import (
	"os"
	"strconv"
)

func getEnvInt(key string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return value
	}
	return fallback
}

type RedisConfig struct {
	Host string
	Port string
	SSL  bool
}

func LoadRedisConfig() (*RedisConfig, error) {
	config := &RedisConfig{
		Host: os.Getenv("REDIS_HOST"),
		Port: os.Getenv("REDIS_PORT"),
		SSL:  os.Getenv("REDIS_SSL") == "true",
	}

	return config, nil
}

type VMSSConfig struct {
	SubscriptionID     string
	TenantID           string
	ResourceGroup      string
	ScaleSetName       string
	InstrumentationKey string
}

func LoadVMSSConfig() (*VMSSConfig, error) {
	config := &VMSSConfig{
		SubscriptionID:     os.Getenv("AZURE_SUBSCRIPTION_ID"),
		TenantID:           os.Getenv("AZURE_TENANT_ID"),
		ResourceGroup:      os.Getenv("AZURE_RESOURCE_GROUP"),
		ScaleSetName:       os.Getenv("AZURE_VMSS_NAME"),
		InstrumentationKey: os.Getenv("AZURE_APPI_INSTRUMENTATION_KEY"),
	}

	return config, nil
}

type ScalerConfig struct {
	PoolCapacity    int
	JobInterval     int
	JobTimeout      int
	VMRuntime       int
	JobDelay        int
	GeoName         string
	WarmPoolSize    int
	WarmPoolEnabled bool
}

func LoadScalerConfig() (*ScalerConfig, error) {
	config := &ScalerConfig{
		PoolCapacity:    getEnvInt("SCALER_POOL_CAPACITY", 4),
		JobInterval:     getEnvInt("SCALER_JOB_INTERVAL", 60),
		JobTimeout:      getEnvInt("SCALER_JOB_TIMEOUT", 180),
		VMRuntime:       getEnvInt("SCALER_VM_RUNTIME", 360),
		JobDelay:        getEnvInt("SCALER_JOB_DELAY", 10),
		GeoName:         os.Getenv("SCALER_GEO_NAME"),
		WarmPoolSize:    getEnvInt("SCALER_WARMPOOL_SIZE", 0),
		WarmPoolEnabled: os.Getenv("SCALER_WARMPOOL_ENABLED") == "true",
	}

	return config, nil
}

type AppGWConfig struct {
	SubscriptionID string
	ResourceGroup  string
	GWName         string
	PathMapName    string
}

func LoadAppGWConfig() (*AppGWConfig, error) {
	config := &AppGWConfig{
		SubscriptionID: os.Getenv("AZURE_SUBSCRIPTION_ID"),
		ResourceGroup:  os.Getenv("AZURE_RESOURCE_GROUP"),
		GWName:         os.Getenv("AZURE_APPGW_NAME"),
		PathMapName:    os.Getenv("AZURE_APPGW_PATH_MAP_NAME"),
	}

	return config, nil
}

type AppConfigConfig struct {
	SubscriptionID string
	ResourceGroup  string
	StoreName      string
}

func LoadAppConfigConfig() (*AppConfigConfig, error) {
	config := &AppConfigConfig{
		SubscriptionID: os.Getenv("AZURE_SUBSCRIPTION_ID"),
		ResourceGroup:  os.Getenv("AZURE_RESOURCE_GROUP"),
		StoreName:      os.Getenv("AZURE_CONFIG_NAME"),
	}

	return config, nil
}
