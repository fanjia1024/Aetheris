// Copyright 2026 fanjia1024
// In-memory secret store (for development only)

package secrets

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type memoryStore struct {
	mu      sync.RWMutex
	secrets map[string]string
}

// NewMemoryStore 创建内存 secret store
func NewMemoryStore() Store {
	return &memoryStore{
		secrets: make(map[string]string),
	}
}

func (m *memoryStore) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.secrets[key]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", key)
	}
	return value, nil
}

func (m *memoryStore) Set(ctx context.Context, key string, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.secrets[key] = value
	return nil
}

func (m *memoryStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.secrets, key)
	return nil
}

func (m *memoryStore) List(ctx context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string
	for key := range m.secrets {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}
