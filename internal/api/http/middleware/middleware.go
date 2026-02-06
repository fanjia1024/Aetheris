package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// Middleware 中间件管理器
type Middleware struct{}

// NewMiddleware 创建新的中间件管理器
func NewMiddleware() *Middleware {
	return &Middleware{}
}

// CORS CORS 中间件
func (m *Middleware) CORS() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		c.Header("Access-Control-Expose-Headers", "Content-Length")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if string(c.Method()) == "OPTIONS" {
			c.AbortWithStatus(consts.StatusNoContent)
			return
		}

		c.Next(ctx)
	}
}

// Auth 认证中间件
func (m *Middleware) Auth() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// 暂时跳过认证
		c.Next(ctx)
	}
}

// RateLimit 速率限制中间件
func (m *Middleware) RateLimit(rps int) app.HandlerFunc {
	var (
		mu       sync.Mutex
		lastTime time.Time
		count    int
	)
	return func(ctx context.Context, c *app.RequestContext) {
		mu.Lock()
		now := time.Now()
		if now.Sub(lastTime) > time.Second {
			lastTime = now
			count = 1
		} else {
			count++
			if count > rps {
				mu.Unlock()
				c.JSON(consts.StatusTooManyRequests, map[string]string{
					"error": "请求过于频繁，请稍后再试",
				})
				c.Abort()
				return
			}
		}
		mu.Unlock()
		c.Next(ctx)
	}
}

// AccessLog 访问日志中间件（使用 hlog）
func (m *Middleware) AccessLog() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()
		c.Next(ctx)
		latency := time.Since(start)
		hlog.CtxInfof(ctx, "%s %s %s %d %s",
			c.Method(), c.Path(), c.ClientIP(), c.Response.StatusCode(), latency)
	}
}
