package cache

import (
	"context"
	"time"
)

// Store 缓存存储接口
type Store interface {
	// Set 设置缓存
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	// Get 获取缓存
	Get(ctx context.Context, key string, dest interface{}) error
	// Delete 删除缓存
	Delete(ctx context.Context, key string) error
	// Exists 检查缓存是否存在
	Exists(ctx context.Context, key string) (bool, error)
	// Clear 清除所有缓存
	Clear(ctx context.Context) error
	// Close 关闭缓存连接
	Close() error
}
