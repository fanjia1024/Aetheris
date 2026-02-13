// Copyright 2026 fanjia1024
// Environment variable based secret store

package secrets

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type envStore struct{}

// NewEnvStore 创建环境变量 secret store
func NewEnvStore() Store {
	return &envStore{}
}

func (e *envStore) Get(ctx context.Context, key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("environment variable not set: %s", key)
	}
	return value, nil
}

func (e *envStore) Set(ctx context.Context, key string, value string) error {
	return os.Setenv(key, value)
}

func (e *envStore) Delete(ctx context.Context, key string) error {
	return os.Unsetenv(key)
}

func (e *envStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) > 0 && strings.HasPrefix(parts[0], prefix) {
			keys = append(keys, parts[0])
		}
	}
	return keys, nil
}
