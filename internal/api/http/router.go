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
}

// NewRouter 创建新的 HTTP 路由器
func NewRouter(handler *Handler, mw *middleware.Middleware) *Router {
	return &Router{
		handler:    handler,
		middleware: mw,
	}
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

	documents := api.Group("/documents")
	{
		documents.POST("/upload", r.middleware.Auth(), r.handler.UploadDocument)
		documents.GET("/", r.middleware.Auth(), r.handler.ListDocuments)
		documents.GET("/:id", r.middleware.Auth(), r.handler.GetDocument)
		documents.DELETE("/:id", r.middleware.Auth(), r.handler.DeleteDocument)
	}

	knowledge := api.Group("/knowledge")
	{
		knowledge.GET("/collections", r.middleware.Auth(), r.handler.ListCollections)
		knowledge.POST("/collections", r.middleware.Auth(), r.handler.CreateCollection)
		knowledge.DELETE("/collections/:id", r.middleware.Auth(), r.handler.DeleteCollection)
	}

	query := api.Group("/query")
	{
		query.POST("/", r.middleware.Auth(), r.handler.Query)
		query.POST("/batch", r.middleware.Auth(), r.handler.BatchQuery)
	}

	system := api.Group("/system")
	{
		system.GET("/status", r.middleware.Auth(), r.handler.SystemStatus)
		system.GET("/metrics", r.middleware.Auth(), r.handler.SystemMetrics)
	}

	return h
}
