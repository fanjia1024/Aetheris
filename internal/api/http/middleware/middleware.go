package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// Middleware 中间件管理器
type Middleware struct {
	// 这里可以添加配置和依赖
}

// NewMiddleware 创建新的中间件管理器
func NewMiddleware() *Middleware {
	return &Middleware{}
}

// CORS CORS 中间件
func (m *Middleware) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		c.Header("Access-Control-Expose-Headers", "Content-Length")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Auth 认证中间件
func (m *Middleware) Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 这里可以实现认证逻辑
		// 暂时跳过认证
		c.Next()
	}
}

// RateLimit 速率限制中间件
func (m *Middleware) RateLimit(rps int) gin.HandlerFunc {
	// 简单的内存速率限制实现
	var (
		lastTime time.Time
		count    int
	)

	return func(c *gin.Context) {
		now := time.Now()
		if now.Sub(lastTime) > time.Second {
			lastTime = now
			count = 1
		} else {
			count++
			if count > rps {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": "请求过于频繁，请稍后再试",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// Logger 日志中间件
func (m *Middleware) Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开始时间
		startTime := time.Now()

		// 处理请求
		c.Next()

		// 结束时间
		endTime := time.Now()
		latency := endTime.Sub(startTime)

		// 请求信息
		method := c.Request.Method
		path := c.Request.URL.Path
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()

		// 日志格式
		gin.DefaultWriter.Write([]byte(
			time.Now().Format("2006-01-02 15:04:05") + " | " +
			method + " | " +
			path + " | " +
			clientIP + " | " +
			string(rune(statusCode)) + " | " +
			latency.String() + "\n",
		))
	}
}

// Recovery 恢复中间件
func (m *Middleware) Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
