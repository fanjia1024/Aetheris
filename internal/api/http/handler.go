package http

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"rag-platform/internal/agent"
	agentruntime "rag-platform/internal/agent/runtime"
	appcore "rag-platform/internal/app"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/runtime/session"
)

// AgentRunner 可选的 Agent 入口（供 POST /api/agent/run 使用）；优先使用 RunWithSession
type AgentRunner interface {
	RunWithSession(ctx context.Context, sess *session.Session, userQuery string) (*agent.RunResult, error)
}

// SessionManager 用于 Agent 请求的 session 查找/创建/保存
type SessionManager interface {
	GetOrCreate(ctx context.Context, id string) (*session.Session, error)
	Save(ctx context.Context, s *session.Session) error
}

// AgentCreator 创建 v1 Agent（由应用层注入 session/memory/planner/tools）
type AgentCreator interface {
	Create(ctx context.Context, name string) (*agentruntime.Agent, error)
}

// Handler HTTP 处理器（仅依赖 Engine + DocumentService，不直接调用 storage）
type Handler struct {
	engine         *eino.Engine
	docService     appcore.DocumentService
	agent          AgentRunner
	sessionManager SessionManager

	// v1 Agent Runtime
	agentManager  *agentruntime.Manager
	agentScheduler *agentruntime.Scheduler
	agentCreator  AgentCreator
}

// NewHandler 创建新的 HTTP 处理器
func NewHandler(engine *eino.Engine, docService appcore.DocumentService) *Handler {
	return &Handler{
		engine:     engine,
		docService: docService,
	}
}

// SetAgent 设置 Agent 入口（可选，用于 /api/agent/run）
func (h *Handler) SetAgent(agent AgentRunner) {
	h.agent = agent
}

// SetSessionManager 设置 Session 管理器（用于 /api/agent/run 的 session 生命周期）
func (h *Handler) SetSessionManager(m SessionManager) {
	h.sessionManager = m
}

// SetAgentRuntime 设置 v1 Agent Manager、Scheduler 与可选 Creator（用于 /api/agents 系列）
func (h *Handler) SetAgentRuntime(manager *agentruntime.Manager, scheduler *agentruntime.Scheduler, creator AgentCreator) {
	h.agentManager = manager
	h.agentScheduler = scheduler
	h.agentCreator = creator
}

// HealthCheck 健康检查
func (h *Handler) HealthCheck(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"service":   "api-service",
	})
}

// UploadDocument 上传文档
func (h *Handler) UploadDocument(ctx context.Context, c *app.RequestContext) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请上传文件",
		})
		return
	}

	result, err := h.engine.ExecuteWorkflow(ctx, "ingest_pipeline", map[string]interface{}{
		"file": file,
		"metadata": map[string]interface{}{
			"filename":    file.Filename,
			"size":        file.Size,
			"uploaded_at": time.Now(),
		},
	})

	if err != nil {
		hlog.CtxErrorf(ctx, "上传文档失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "上传文档失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":  "success",
		"result":  result,
		"message": "文档上传成功",
	})
}

// ListDocuments 列出文档
func (h *Handler) ListDocuments(ctx context.Context, c *app.RequestContext) {
	documents, err := h.docService.ListDocuments(ctx)
	if err != nil {
		hlog.CtxErrorf(ctx, "获取文档列表失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "获取文档列表失败",
		})
		return
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"documents": documents,
		"total":     len(documents),
	})
}

// GetDocument 获取文档
func (h *Handler) GetDocument(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	document, err := h.docService.GetDocument(ctx, id)
	if err != nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "文档不存在",
		})
		return
	}

	c.JSON(consts.StatusOK, document)
}

// DeleteDocument 删除文档
func (h *Handler) DeleteDocument(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	if err := h.docService.DeleteDocument(ctx, id); err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "删除文档失败",
		})
		return
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "文档删除成功",
	})
}

// ListCollections 列出集合
func (h *Handler) ListCollections(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, map[string]interface{}{
		"collections": []map[string]interface{}{
			{
				"id":             "default",
				"name":           "默认集合",
				"description":    "默认文档集合",
				"document_count": 100,
				"created_at":     time.Now(),
			},
		},
	})
}

// CreateCollection 创建集合
func (h *Handler) CreateCollection(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.BindJSON(&request); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数错误",
		})
		return
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"status": "success",
		"collection": map[string]interface{}{
			"id":          "new-collection",
			"name":        request.Name,
			"description": request.Description,
			"created_at":  time.Now(),
		},
	})
}

// DeleteCollection 删除集合
func (h *Handler) DeleteCollection(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("集合 %s 删除成功", id),
	})
}

// Query 查询
// Deprecated: 请使用 POST /api/agents/{id}/message 以 Agent 为中心与系统交互。
func (h *Handler) Query(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Query    string                 `json:"query" binding:"required"`
		Metadata map[string]interface{} `json:"metadata"`
		TopK     int                    `json:"top_k"`
	}

	if err := c.BindJSON(&request); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数错误",
		})
		return
	}

	query := &common.Query{
		ID:        fmt.Sprintf("query-%d", time.Now().UnixNano()),
		Text:      request.Query,
		Metadata:  request.Metadata,
		CreatedAt: time.Now(),
	}

	result, err := h.engine.ExecuteWorkflow(ctx, "query_pipeline", map[string]interface{}{
		"query": query,
		"top_k": request.TopK,
	})

	if err != nil {
		hlog.CtxErrorf(ctx, "查询失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "查询失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"status": "success",
		"result": result,
	})
}

// BatchQuery 批量查询
func (h *Handler) BatchQuery(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Queries []struct {
			Query    string                 `json:"query" binding:"required"`
			Metadata map[string]interface{} `json:"metadata"`
		} `json:"queries" binding:"required"`
	}

	if err := c.BindJSON(&request); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数错误",
		})
		return
	}

	results := make([]interface{}, len(request.Queries))

	for i, q := range request.Queries {
		query := &common.Query{
			ID:        fmt.Sprintf("query-%d-%d", time.Now().UnixNano(), i),
			Text:      q.Query,
			Metadata:  q.Metadata,
			CreatedAt: time.Now(),
		}

		result, err := h.engine.ExecuteWorkflow(ctx, "query_pipeline", map[string]interface{}{
			"query": query,
		})

		if err != nil {
			results[i] = map[string]interface{}{
				"error": err.Error(),
				"query": q.Query,
			}
		} else {
			results[i] = result
		}
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"status": "success",
		"results": results,
		"total":   len(results),
	})
}

// SystemStatus 系统状态
func (h *Handler) SystemStatus(ctx context.Context, c *app.RequestContext) {
	status := map[string]interface{}{
		"api_service":    "running",
		"agent_service":  "running",
		"index_service":  "running",
		"workflows":      h.engine.GetWorkflows(),
		"agents":         h.engine.GetAgents(),
		"timestamp":      time.Now(),
	}

	c.JSON(consts.StatusOK, status)
}

// SystemMetrics 系统指标
func (h *Handler) SystemMetrics(ctx context.Context, c *app.RequestContext) {
	metrics := map[string]interface{}{
		"requests_total":  1000,
		"errors_total":    10,
		"latency_avg":     50,
		"documents_count": 1000,
		"index_size":      "100MB",
		"timestamp":       time.Now(),
	}

	c.JSON(consts.StatusOK, metrics)
}

// AgentRunRequest POST /api/agent/run 请求体
type AgentRunRequest struct {
	Query     string         `json:"query" binding:"required"`
	SessionID string         `json:"session_id"`
	History   []llm.Message `json:"history"`
}

// AgentRun Agent 入口：找到或创建 session，在 session 上执行 Agent，保存后返回
func (h *Handler) AgentRun(ctx context.Context, c *app.RequestContext) {
	if h.agent == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent 未配置",
		})
		return
	}
	if h.sessionManager == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "SessionManager 未配置",
		})
		return
	}
	var req AgentRunRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数错误",
		})
		return
	}
	sess, err := h.sessionManager.GetOrCreate(ctx, req.SessionID)
	if err != nil {
		hlog.CtxErrorf(ctx, "Session GetOrCreate 失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "获取或创建 Session 失败",
			"details": err.Error(),
		})
		return
	}
	result, err := h.agent.RunWithSession(ctx, sess, req.Query)
	if err != nil {
		hlog.CtxErrorf(ctx, "Agent Run 失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "Agent 执行失败",
			"details": err.Error(),
		})
		return
	}
	if err := h.sessionManager.Save(ctx, sess); err != nil {
		hlog.CtxErrorf(ctx, "Session Save 失败: %v", err)
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":      "success",
		"session_id":  sess.ID,
		"answer":      result.Answer,
		"steps":       result.Steps,
		"duration_ms": result.Duration.Milliseconds(),
	})
}

// --- v1 Agent API ---

// CreateAgentRequest POST /api/agents 请求体
type CreateAgentRequest struct {
	Name string `json:"name"`
}

// CreateAgent 创建 Agent，返回 Agent ID
func (h *Handler) CreateAgent(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil || h.agentCreator == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent Runtime 未配置",
		})
		return
	}
	var req CreateAgentRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数错误",
		})
		return
	}
	agent, err := h.agentCreator.Create(ctx, req.Name)
	if err != nil {
		hlog.CtxErrorf(ctx, "创建 Agent 失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "创建 Agent 失败",
			"details": err.Error(),
		})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"id":   agent.ID,
		"name": agent.Name,
	})
}

// AgentMessageRequest POST /api/agents/:id/message 请求体
type AgentMessageRequest struct {
	Message string `json:"message" binding:"required"`
}

// AgentMessage 向 Agent 发送消息（写入 Session 并唤醒）
func (h *Handler) AgentMessage(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil || h.agentScheduler == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent Runtime 未配置",
		})
		return
	}
	id := c.Param("id")
	var req AgentMessageRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数错误",
		})
		return
	}
	agent, err := h.agentManager.Get(ctx, id)
	if err != nil || agent == nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "Agent 不存在",
		})
		return
	}
	agent.Session.AddMessage("user", req.Message)
	_ = h.agentScheduler.WakeAgent(ctx, id)
	c.JSON(consts.StatusAccepted, map[string]interface{}{
		"status": "accepted",
		"agent_id": id,
	})
}

// AgentState 返回 Agent 状态（Status, CurrentTask, LastCheckpoint）
func (h *Handler) AgentState(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent Runtime 未配置",
		})
		return
	}
	id := c.Param("id")
	agent, err := h.agentManager.Get(ctx, id)
	if err != nil || agent == nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "Agent 不存在",
		})
		return
	}
	sess := agent.Session
	c.JSON(consts.StatusOK, map[string]interface{}{
		"agent_id":         agent.ID,
		"status":           agent.GetStatus().String(),
		"current_task":    sess.GetCurrentTask(),
		"last_checkpoint": sess.GetLastCheckpoint(),
		"updated_at":      sess.GetUpdatedAt(),
	})
}

// AgentResume 恢复 Agent 执行
func (h *Handler) AgentResume(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil || h.agentScheduler == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent Runtime 未配置",
		})
		return
	}
	id := c.Param("id")
	agent, _ := h.agentManager.Get(ctx, id)
	if agent == nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "Agent 不存在",
		})
		return
	}
	_ = h.agentScheduler.Resume(ctx, id)
	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":   "ok",
		"agent_id": id,
	})
}

// AgentStop 停止 Agent
func (h *Handler) AgentStop(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil || h.agentScheduler == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent Runtime 未配置",
		})
		return
	}
	id := c.Param("id")
	agent, _ := h.agentManager.Get(ctx, id)
	if agent == nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "Agent 不存在",
		})
		return
	}
	_ = h.agentScheduler.Stop(ctx, id)
	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":   "ok",
		"agent_id": id,
	})
}

// ListAgents 列出所有 Agent
func (h *Handler) ListAgents(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent Runtime 未配置",
		})
		return
	}
	list, err := h.agentManager.List(ctx)
	if err != nil {
		hlog.CtxErrorf(ctx, "列出 Agent 失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "列出 Agent 失败",
		})
		return
	}
	agents := make([]map[string]interface{}, 0, len(list))
	for _, a := range list {
		agents = append(agents, map[string]interface{}{
			"id":         a.ID,
			"name":       a.Name,
			"status":     a.GetStatus().String(),
			"created_at": a.CreatedAt,
		})
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"agents": agents,
		"total":  len(agents),
	})
}
