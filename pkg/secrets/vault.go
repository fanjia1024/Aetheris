// Copyright 2026 fanjia1024
// HashiCorp Vault secret store

package secrets

import (
	"context"
	"fmt"
	"strings"
	"sync"

	vault "github.com/hashicorp/vault/api"
)

// VaultConfig Vault 配置
type VaultConfig struct {
	Address    string `yaml:"address"`     // Vault server address (e.g., http://vault:8200)
	Token      string `yaml:"token"`       // Vault token
	PathPrefix string `yaml:"path_prefix"` // Secret path prefix (e.g., "secret")
}

type vaultStore struct {
	client     *vault.Client
	pathPrefix string
	mu         sync.RWMutex
	transient  map[string]string // For Set operations that need caching
}

// NewVaultStore 创建 Vault secret store
func NewVaultStore(config VaultConfig) (Store, error) {
	if config.Address == "" {
		config.Address = "http://localhost:8200"
	}

	cfg := vault.DefaultConfig()
	cfg.Address = config.Address

	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	if config.Token != "" {
		client.SetToken(config.Token)
	}

	// Try to verify connection
	if _, err := client.Sys().Health(); err != nil {
		return nil, fmt.Errorf("failed to connect to vault: %w", err)
	}

	prefix := "secret"
	if config.PathPrefix != "" {
		prefix = config.PathPrefix
	}

	return &vaultStore{
		client:     client,
		pathPrefix: prefix,
		transient:  make(map[string]string),
	}, nil
}

func (v *vaultStore) Get(ctx context.Context, key string) (string, error) {
	// Try transient cache first (for recently set values)
	v.mu.RLock()
	if val, ok := v.transient[key]; ok {
		v.mu.RUnlock()
		return val, nil
	}
	v.mu.RUnlock()

	// Read from Vault
	secretPath := v.buildPath(key)
	secret, err := v.client.Logical().Read(secretPath)
	if err != nil {
		return "", fmt.Errorf("failed to read secret from vault: %w", err)
	}

	if secret == nil {
		return "", fmt.Errorf("secret not found: %s", key)
	}

	// Get the value from secret data
	if data, ok := secret.Data["value"].(string); ok {
		return data, nil
	}

	// If no "value" key, return first value found
	for _, val := range secret.Data {
		if str, ok := val.(string); ok {
			return str, nil
		}
	}

	return "", fmt.Errorf("secret value not found: %s", key)
}

func (v *vaultStore) Set(ctx context.Context, key string, value string) error {
	secretPath := v.buildPath(key)

	data := map[string]interface{}{
		"value": value,
	}

	_, err := v.client.Logical().Write(secretPath, data)
	if err != nil {
		return fmt.Errorf("failed to write secret to vault: %w", err)
	}

	// Cache in transient store
	v.mu.Lock()
	v.transient[key] = value
	v.mu.Unlock()

	return nil
}

func (v *vaultStore) Delete(ctx context.Context, key string) error {
	secretPath := v.buildPath(key)

	_, err := v.client.Logical().Delete(secretPath)
	if err != nil {
		return fmt.Errorf("failed to delete secret from vault: %w", err)
	}

	v.mu.Lock()
	delete(v.transient, key)
	v.mu.Unlock()

	return nil
}

func (v *vaultStore) List(ctx context.Context, prefix string) ([]string, error) {
	searchPath := v.pathPrefix
	if prefix != "" {
		searchPath = fmt.Sprintf("%s/metadata/%s", v.pathPrefix, prefix)
	}

	secret, err := v.client.Logical().List(searchPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets from vault: %w", err)
	}

	if secret == nil {
		return nil, nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return nil, nil
	}

	var result []string
	for _, k := range keys {
		if str, ok := k.(string); ok {
			fullKey := str
			if !strings.HasPrefix(str, prefix) {
				fullKey = fmt.Sprintf("%s/%s", prefix, str)
			}
			result = append(result, fullKey)
		}
	}

	return result, nil
}

func (v *vaultStore) buildPath(key string) string {
	return fmt.Sprintf("%s/%s", v.pathPrefix, key)
}
