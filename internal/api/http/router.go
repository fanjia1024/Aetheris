package http

import (
	"github.com/gin-gonic/gin"
	"rag-platform/internal/api/http/middleware"
)

// Router HTTP 路由器
type Router struct {
	engine      *gin.Engine
	handler     *Handler
	middleware  *middleware.Middleware
}

// NewRouter 创建新的 HTTP 路由器
func NewRouter(handler *Handler, middleware *middleware.Middleware) *Router {
	// 设置 Gin 模式
	gin.SetMode(gin.ReleaseMode)

	// 创建引擎
	engine := gin.New()

	// 使用中间件
	engine.Use(gin.Logger())
	engine.Use(gin.Recovery())

	return &Router{
		engine:     engine,
		handler:    handler,
		middleware: middleware,
	}
}

// SetupRoutes 设置路由
func (r *Router) SetupRoutes() {
	// API 路由组
	api := r.engine.Group("/api")

	// 健康检查
	api.GET("/health", r.handler.HealthCheck)

	// 文档管理
	documents := api.Group("/documents")
	{
		documents.POST("/upload", r.middleware.CORS(), r.handler.UploadDocument)
		documents.GET("/", r.middleware.CORS(), r.handler.ListDocuments)
		documents.GET("/:id", r.middleware.CORS(), r.handler.GetDocument)
		documents.DELETE("/:id", r.middleware.CORS(), r.handler.DeleteDocument)
	}

	// 知识库管理
	knowledge := api.Group("/knowledge")
	{
		knowledge.GET("/collections", r.middleware.CORS(), r.handler.ListCollections)
		knowledge.POST("/collections", r.middleware.CORS(), r.handler.CreateCollection)
		knowledge.DELETE("/collections/:id", r.middleware.CORS(), r.handler.DeleteCollection)
	}

	// 查询 API
	query := api.Group("/query")
	{
		query.POST("/", r.middleware.CORS(), r.handler.Query)
		query.POST("/batch", r.middleware.CORS(), r.handler.BatchQuery)
	}

	// 系统管理
	system := api.Group("/system")
	{
		system.GET("/status", r.middleware.CORS(), r.handler.SystemStatus)
		system.GET("/metrics", r.middleware.CORS(), r.handler.SystemMetrics)
	}
}

// Engine 获取 Gin 引擎
func (r *Router) Engine() *gin.Engine {
	return r.engine
}

// Run 启动 HTTP 服务
func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}
