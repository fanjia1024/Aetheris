package http

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"
	"rag-platform/internal/api/http/middleware"
)

// Router HTTP 路由器（Hertz）
type Router struct {
	handler    *Handler
	middleware *middleware.Middleware
	jwtAuth    *middleware.JWTAuth
}

// NewRouter 创建新的 HTTP 路由器
func NewRouter(handler *Handler, mw *middleware.Middleware) *Router {
	return &Router{
		handler:    handler,
		middleware: mw,
	}
}

// SetJWT 设置 JWT 认证（启用后需在 Build 前调用）
func (r *Router) SetJWT(jwtAuth *middleware.JWTAuth) {
	r.jwtAuth = jwtAuth
}

// Build 创建 Hertz 引擎并注册路由与中间件，供 app 层 Run(addr) 使用；opts 可传入 server.WithTracer 等
func (r *Router) Build(addr string, opts ...config.Option) *server.Hertz {
	allOpts := append([]config.Option{server.WithHostPorts(addr)}, opts...)
	h := server.Default(allOpts...)

	// 全局中间件：访问日志、CORS
	h.Use(r.middleware.AccessLog())
	h.Use(r.middleware.CORS())

	api := h.Group("/api")
	api.GET("/health", r.handler.HealthCheck)

	authHandler := r.middleware.Auth()
	if r.jwtAuth != nil {
		api.POST("/login", r.jwtAuth.LoginHandler())
		authHandler = r.jwtAuth.MiddlewareFunc()
	}

	documents := api.Group("/documents")
	{
		documents.POST("/upload", authHandler, r.handler.UploadDocument)
		documents.GET("/", authHandler, r.handler.ListDocuments)
		documents.GET("/:id", authHandler, r.handler.GetDocument)
		documents.DELETE("/:id", authHandler, r.handler.DeleteDocument)
	}

	knowledge := api.Group("/knowledge")
	{
		knowledge.GET("/collections", authHandler, r.handler.ListCollections)
		knowledge.POST("/collections", authHandler, r.handler.CreateCollection)
		knowledge.DELETE("/collections/:id", authHandler, r.handler.DeleteCollection)
	}

	query := api.Group("/query")
	{
		query.POST("/", authHandler, r.handler.Query)
		query.POST("/batch", authHandler, r.handler.BatchQuery)
	}

	agentGroup := api.Group("/agent")
	{
		agentGroup.POST("/run", authHandler, r.handler.AgentRun)
	}

	system := api.Group("/system")
	{
		system.GET("/status", authHandler, r.handler.SystemStatus)
		system.GET("/metrics", authHandler, r.handler.SystemMetrics)
	}

	return h
}
