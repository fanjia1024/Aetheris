// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/jwt"
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

// Auth 认证中间件（未启用 JWT 时跳过认证）
func (m *Middleware) Auth() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Next(ctx)
	}
}

// JWTAuth 持有 JWT 中间件，用于 Login 与保护路由
type JWTAuth struct {
	Middleware *jwt.HertzJWTMiddleware
}

// LoginHandler 返回登录接口 Handler
func (j *JWTAuth) LoginHandler() app.HandlerFunc {
	return j.Middleware.LoginHandler
}

// MiddlewareFunc 返回 JWT 校验中间件
func (j *JWTAuth) MiddlewareFunc() app.HandlerFunc {
	return j.Middleware.MiddlewareFunc()
}

// NewJWTAuth 创建 JWT 认证（key 签名密钥；Authenticator 示例：admin/admin、test/test 通过）
func NewJWTAuth(key []byte, timeout, maxRefresh time.Duration) (*JWTAuth, error) {
	identityKey := "id"
	authMiddleware, err := jwt.New(&jwt.HertzJWTMiddleware{
		Realm:       "rag-api",
		Key:         key,
		Timeout:     timeout,
		MaxRefresh:  maxRefresh,
		IdentityKey: identityKey,
		PayloadFunc: func(data interface{}) jwt.MapClaims {
			if u, ok := data.(*AuthUser); ok {
				return jwt.MapClaims{identityKey: u.Username}
			}
			return jwt.MapClaims{}
		},
		IdentityHandler: func(ctx context.Context, c *app.RequestContext) interface{} {
			claims := jwt.ExtractClaims(ctx, c)
			return &AuthUser{Username: claims[identityKey].(string)}
		},
		Authenticator: func(ctx context.Context, c *app.RequestContext) (interface{}, error) {
			var loginVals struct {
				Username string `form:"username" json:"username"`
				Password string `form:"password" json:"password"`
			}
			if err := c.Bind(&loginVals); err != nil {
				return nil, jwt.ErrMissingLoginValues
			}
			if (loginVals.Username == "admin" && loginVals.Password == "admin") ||
				(loginVals.Username == "test" && loginVals.Password == "test") {
				return &AuthUser{Username: loginVals.Username}, nil
			}
			return nil, jwt.ErrFailedAuthentication
		},
		Authorizator: func(data interface{}, ctx context.Context, c *app.RequestContext) bool {
			return data != nil
		},
		Unauthorized: func(ctx context.Context, c *app.RequestContext, code int, message string) {
			c.JSON(code, map[string]interface{}{"code": code, "message": message})
		},
	})
	if err != nil {
		return nil, err
	}
	if errInit := authMiddleware.MiddlewareInit(); errInit != nil {
		return nil, errInit
	}
	return &JWTAuth{Middleware: authMiddleware}, nil
}

// AuthUser 登录用户（示例）
type AuthUser struct {
	Username string
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
