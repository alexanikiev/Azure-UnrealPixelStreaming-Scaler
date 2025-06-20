package redis

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"scaler/pkg/config"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/redis/go-redis/v9"
)

const (
	VMStatusAvailableSet   = "vmss:status:available"
	VMStatusReservedSet    = "vmss:status:reserved"
	VMStatusUnavailableSet = "vmss:status:unavailable"
)

type Pipeline interface {
	Set(ctx context.Context, key, value string) error
	SAdd(ctx context.Context, key string, member ...string) error
	SRem(ctx context.Context, key string, members ...string) error
	Delete(ctx context.Context, key string) error
	Exec(ctx context.Context) error
}

type Client interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}) error
	Delete(ctx context.Context, key string) error
	Keys(ctx context.Context, pattern string) ([]string, error)
	SPop(ctx context.Context, key string, count int64) ([]string, error)
	SMembers(ctx context.Context, key string) ([]string, error)
	Pipeline() Pipeline
	Ping(ctx context.Context) error
	Close() error
}

type redisClient struct {
	client *redis.Client
}

func redisCredentialProvider(credential azcore.TokenCredential) func(context.Context) (string, string, error) {
	return func(ctx context.Context) (string, string, error) {
		// Get an access token for Azure Cache for Redis
		tk, err := credential.GetToken(ctx, policy.TokenRequestOptions{
			// Azure Cache for Redis uses the same scope in all clouds
			Scopes: []string{"https://redis.azure.com/.default"},
		})
		if err != nil {
			return "", "", err
		}

		// The token is a JWT: get the principal's object ID from its payload
		parts := strings.Split(tk.Token, ".")
		if len(parts) != 3 {
			return "", "", errors.New("token must have 3 parts")
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", "", fmt.Errorf("couldn't decode payload: %s", err)
		}
		claims := struct {
			OID string `json:"oid"`
		}{}
		err = json.Unmarshal(payload, &claims)
		if err != nil {
			return "", "", fmt.Errorf("couldn't unmarshal payload: %s", err)
		}
		if claims.OID == "" {
			return "", "", errors.New("missing object ID claim")
		}
		return claims.OID, tk.Token, nil
	}
}

func NewClient(cfg *config.RedisConfig) (Client, error) {
	// Get token credential from managed identity
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %v", err)
	}

	opts := &redis.Options{
		Addr:                       fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		CredentialsProviderContext: redisCredentialProvider(cred),
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	client := redis.NewClient(opts)
	ctx := context.Background()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %v", err)
	}

	return &redisClient{
		client: client,
	}, nil
}

func (r *redisClient) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *redisClient) Set(ctx context.Context, key string, value interface{}) error {
	return r.client.Set(ctx, key, value, 0).Err()
}

func (r *redisClient) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *redisClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	return r.client.Keys(ctx, pattern).Result()
}

func (c *redisClient) SPop(ctx context.Context, key string, count int64) ([]string, error) {
	result, err := c.client.SPopN(ctx, key, count).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to pop members from set %s: %w", key, err)
	}
	return result, nil
}

func (c *redisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	members, err := c.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get set members for %s: %w", key, err)
	}
	return members, nil
}

type redisPipeline struct {
	pipeline redis.Pipeliner
}

func (c *redisClient) Pipeline() Pipeline {
	pipe := c.client.Pipeline()
	return &redisPipeline{pipeline: pipe}
}

func (p *redisPipeline) Set(ctx context.Context, key, value string) error {
	p.pipeline.Set(ctx, key, value, 0)
	return nil
}

func (p *redisPipeline) SAdd(ctx context.Context, key string, members ...string) error {
	p.pipeline.SAdd(ctx, key, members)
	return nil
}

func (p *redisPipeline) SRem(ctx context.Context, key string, members ...string) error {
	p.pipeline.SRem(ctx, key, members)
	return nil
}

func (p *redisPipeline) Delete(ctx context.Context, key string) error {
	p.pipeline.Del(ctx, key)
	return nil
}

func (p *redisPipeline) Exec(ctx context.Context) error {
	_, err := p.pipeline.Exec(ctx)
	return err
}

func (c *redisClient) Ping(ctx context.Context) error {
	result := c.client.Ping(ctx)
	if result.Err() != nil {
		return fmt.Errorf("redis ping failed: %w", result.Err())
	}

	// Check for specific response
	if result.Val() != "PONG" {
		return fmt.Errorf("unexpected ping response: %s", result.Val())
	}

	return nil
}

func (r *redisClient) Close() error {
	return r.client.Close()
}
