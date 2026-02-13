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

package http

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"
	"rag-platform/internal/api/http/middleware"
	"rag-platform/pkg/auth"
)

// Router HTTP 路由器（Hertz）
type Router struct {
	handler    *Handler
	middleware *middleware.Middleware
	jwtAuth    *middleware.JWTAuth
	authz      *middleware.AuthZMiddleware
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

// SetAuthZ 设置 RBAC 授权中间件（可选；启用后需在 Build 前调用）
func (r *Router) SetAuthZ(authz *middleware.AuthZMiddleware) {
	r.authz = authz
}

// authChain 返回认证链：authHandler + InjectAuthContext；若启用 RBAC 则追加 RequirePermission
func (r *Router) authChain(permission auth.Permission) []app.HandlerFunc {
	chain := []app.HandlerFunc{r.middleware.Auth(), r.middleware.InjectAuthContext()}
	if r.jwtAuth != nil {
		chain[0] = r.jwtAuth.MiddlewareFunc()
	}
	if r.authz != nil {
		chain = append(chain, r.authz.RequirePermission(permission))
	}
	return chain
}

// authChainWith 认证链 + 最终 handler，用于注册路由
func (r *Router) authChainWith(permission auth.Permission, handler app.HandlerFunc) []app.HandlerFunc {
	return append(r.authChain(permission), handler)
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
	if r.jwtAuth != nil {
		api.POST("/login", r.jwtAuth.LoginHandler())
	}

	documents := api.Group("/documents")
	{
		documents.POST("/upload", r.authChainWith(auth.PermissionJobView, r.handler.UploadDocument)...)
		documents.POST("/upload/async", r.authChainWith(auth.PermissionJobView, r.handler.UploadDocumentAsync)...)
		documents.GET("/upload/status/:task_id", r.authChainWith(auth.PermissionJobView, r.handler.UploadStatus)...)
		documents.GET("/", r.authChainWith(auth.PermissionJobView, r.handler.ListDocuments)...)
		documents.GET("/:id", r.authChainWith(auth.PermissionJobView, r.handler.GetDocument)...)
		documents.DELETE("/:id", r.authChainWith(auth.PermissionJobView, r.handler.DeleteDocument)...)
	}

	knowledge := api.Group("/knowledge")
	{
		knowledge.GET("/collections", r.authChainWith(auth.PermissionJobView, r.handler.ListCollections)...)
		knowledge.POST("/collections", r.authChainWith(auth.PermissionJobView, r.handler.CreateCollection)...)
		knowledge.DELETE("/collections/:id", r.authChainWith(auth.PermissionJobView, r.handler.DeleteCollection)...)
	}

	// Deprecated: 请使用 POST /api/agents/{id}/message
	query := api.Group("/query")
	{
		query.POST("/", r.authChainWith(auth.PermissionJobCreate, r.handler.Query)...)
		query.POST("/batch", r.authChainWith(auth.PermissionJobCreate, r.handler.BatchQuery)...)
	}

	agentGroup := api.Group("/agent")
	{
		agentGroup.POST("/run", r.authChainWith(auth.PermissionJobCreate, r.handler.AgentRun)...)
		agentGroup.POST("/resume", r.authChainWith(auth.PermissionJobCreate, r.handler.AgentResumeCheckpoint)...)
		agentGroup.POST("/stream", r.authChainWith(auth.PermissionJobCreate, r.handler.AgentStream)...)
	}

	// v1 Agent 中心 API（POST/GET 同时注册 "" 与 "/" 以兼容带/不带尾部斜杠的请求）
	agents := api.Group("/agents")
	{
		agents.POST("", r.authChainWith(auth.PermissionAgentManage, r.handler.CreateAgent)...)
		agents.POST("/", r.authChainWith(auth.PermissionAgentManage, r.handler.CreateAgent)...)
		agents.GET("", r.authChainWith(auth.PermissionJobView, r.handler.ListAgents)...)
		agents.GET("/", r.authChainWith(auth.PermissionJobView, r.handler.ListAgents)...)
		agents.POST("/:id/message", r.authChainWith(auth.PermissionJobCreate, r.handler.AgentMessage)...)
		agents.GET("/:id/state", r.authChainWith(auth.PermissionJobView, r.handler.AgentState)...)
		agents.POST("/:id/resume", r.authChainWith(auth.PermissionJobCreate, r.handler.AgentResume)...)
		agents.POST("/:id/stop", r.authChainWith(auth.PermissionJobStop, r.handler.AgentStop)...)
		agents.GET("/:id/jobs/:job_id", r.authChainWith(auth.PermissionJobView, r.handler.GetAgentJob)...)
		agents.GET("/:id/jobs", r.authChainWith(auth.PermissionJobView, r.handler.ListAgentJobs)...)
	}

	// Execution Trace：Job 时间线与节点详情（可观测）
	jobs := api.Group("/jobs")
	{
		jobs.GET("/:id", r.authChainWith(auth.PermissionJobView, r.handler.GetJob)...)
		jobs.POST("/:id/stop", r.authChainWith(auth.PermissionJobStop, r.handler.JobStop)...)
		jobs.POST("/:id/signal", r.authChainWith(auth.PermissionJobCreate, r.handler.JobSignal)...)
		jobs.POST("/:id/message", r.authChainWith(auth.PermissionJobCreate, r.handler.JobMessage)...)
		jobs.GET("/:id/events", r.authChainWith(auth.PermissionJobView, r.handler.GetJobEvents)...)
		jobs.GET("/:id/replay", r.authChainWith(auth.PermissionTraceView, r.handler.GetJobReplay)...)
		jobs.GET("/:id/verify", r.authChainWith(auth.PermissionTraceView, r.handler.GetJobVerify)...)
		jobs.GET("/:id/trace", r.authChainWith(auth.PermissionTraceView, r.handler.GetJobTrace)...)
		jobs.GET("/:id/trace/cognition", r.authChainWith(auth.PermissionTraceView, r.handler.GetJobCognitionTrace)...)
		jobs.GET("/:id/nodes/:node_id", r.authChainWith(auth.PermissionTraceView, r.handler.GetJobNode)...)
		jobs.GET("/:id/trace/page", r.authChainWith(auth.PermissionTraceView, r.handler.GetJobTracePage)...)
		jobs.POST("/:id/export", r.authChainWith(auth.PermissionJobExport, r.handler.ExportJobForensics)...)
		jobs.GET("/:id/evidence-graph", r.authChainWith(auth.PermissionAuditView, r.handler.GetJobEvidenceGraph)...)
		jobs.GET("/:id/audit-log", r.authChainWith(auth.PermissionAuditView, r.handler.GetJobAuditLog)...)
	}

	// 2.0-M3: Forensics API
	forensics := api.Group("/forensics")
	{
		forensics.POST("/query", r.authChainWith(auth.PermissionJobExport, r.handler.ForensicsQuery)...)
		forensics.POST("/batch-export", r.authChainWith(auth.PermissionJobExport, r.handler.ForensicsBatchExport)...)
		forensics.GET("/export-status/:task_id", r.authChainWith(auth.PermissionJobExport, r.handler.ForensicsExportStatus)...)
		forensics.GET("/consistency/:job_id", r.authChainWith(auth.PermissionJobView, r.handler.ForensicsConsistencyCheck)...)
	}

	toolsGroup := api.Group("/tools")
	{
		toolsGroup.GET("/", r.authChainWith(auth.PermissionToolExecute, r.handler.ListTools)...)
		toolsGroup.GET("/:name", r.authChainWith(auth.PermissionToolExecute, r.handler.GetTool)...)
	}

	system := api.Group("/system")
	{
		system.GET("/status", r.authChainWith(auth.PermissionJobView, r.handler.SystemStatus)...)
		system.GET("/metrics", r.authChainWith(auth.PermissionJobView, r.handler.SystemMetrics)...)
		system.GET("/workers", r.authChainWith(auth.PermissionJobView, r.handler.SystemWorkers)...)
	}
	api.GET("/observability/summary", r.authChainWith(auth.PermissionJobView, r.handler.GetObservabilitySummary)...)
	api.GET("/observability/stuck", r.authChainWith(auth.PermissionJobView, r.handler.GetObservabilityStuck)...)

	return h
}
