// Copyright 2026 fanjia1024
// Kubernetes secret store

package secrets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// K8sConfig Kubernetes 配置
type K8sConfig struct {
	// ServiceAccountPath 是 Kubernetes service account token 路径
	// 默认: /var/run/secrets/kubernetes.io/serviceaccount
	ServiceAccountPath string `yaml:"service_account_path"`

	// Namespace 是 pod 所在 namespace
	Namespace string `yaml:"namespace"`

	// SecretsPath 是额外 secret 挂载路径
	SecretsPath string `yaml:"secrets_path"`
}

type k8sStore struct {
	serviceAccountPath string
	secretsPath        string
	namespace          string
	mu                 sync.RWMutex
	cache              map[string]string
}

// NewK8sStore 创建 Kubernetes secret store
// 从 pod 的 service account secret 和额外挂载的 secret 读取
func NewK8sStore(config K8sConfig) (Store, error) {
	saPath := "/var/run/secrets/kubernetes.io/serviceaccount"
	if config.ServiceAccountPath != "" {
		saPath = config.ServiceAccountPath
	}

	// Verify the service account directory exists
	if _, err := os.Stat(saPath); os.IsNotExist(err) {
		// In non-K8s environment, return an error
		return nil, fmt.Errorf("kubernetes service account path not found: %s (not running in Kubernetes?)", saPath)
	}

	secretsPath := "/etc/secrets"
	if config.SecretsPath != "" {
		secretsPath = config.SecretsPath
	}

	namespace := "default"
	if config.Namespace != "" {
		namespace = config.Namespace
	}

	return &k8sStore{
		serviceAccountPath: saPath,
		secretsPath:        secretsPath,
		namespace:          namespace,
		cache:              make(map[string]string),
	}, nil
}

func (k *k8sStore) Get(ctx context.Context, key string) (string, error) {
	// Check cache first
	k.mu.RLock()
	if val, ok := k.cache[key]; ok {
		k.mu.RUnlock()
		return val, nil
	}
	k.mu.RUnlock()

	// Try service account token first
	tokenPath := filepath.Join(k.serviceAccountPath, "token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		k.mu.Lock()
		k.cache[key] = string(data)
		k.mu.Unlock()
		return string(data), nil
	}

	// Try secrets path
	secretPath := filepath.Join(k.secretsPath, key)
	if data, err := os.ReadFile(secretPath); err == nil {
		k.mu.Lock()
		k.cache[key] = string(data)
		k.mu.Unlock()
		return string(data), nil
	}

	// Try reading from /run/secrets/kubernetes.io/... (standard K8s secret mount)
	k8sSecretPath := fmt.Sprintf("/run/secrets/kubernetes.io/%s/%s", k.namespace, key)
	if data, err := os.ReadFile(k8sSecretPath); err == nil {
		k.mu.Lock()
		k.cache[key] = string(data)
		k.mu.Unlock()
		return string(data), nil
	}

	return "", fmt.Errorf("secret not found: %s", key)
}

func (k *k8sStore) Set(ctx context.Context, key string, value string) error {
	// Kubernetes secrets are typically read-only from within the pod
	// We only support caching for in-pod access
	k.mu.Lock()
	defer k.mu.Unlock()
	k.cache[key] = value
	return nil
}

func (k *k8sStore) Delete(ctx context.Context, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.cache, key)
	return nil
}

func (k *k8sStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string

	// List from service account directory
	if entries, err := os.ReadDir(k.serviceAccountPath); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				key := e.Name()
				if prefix == "" || strings.HasPrefix(key, prefix) {
					keys = append(keys, key)
				}
			}
		}
	}

	// List from secrets path (if exists)
	if entries, err := os.ReadDir(k.secretsPath); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				key := e.Name()
				if prefix == "" || strings.HasPrefix(key, prefix) {
					// Avoid duplicates
					found := false
					for _, k := range keys {
						if k == key {
							found = true
							break
						}
					}
					if !found {
						keys = append(keys, key)
					}
				}
			}
		}
	}

	return keys, nil
}
