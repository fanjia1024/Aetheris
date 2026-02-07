package http

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"rag-platform/internal/agent"
	appcore "rag-platform/internal/app"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/runtime/eino"
)

// AgentRunner 可选的 Agent 入口（供 POST /api/agent/run 使用）
type AgentRunner interface {
	Run(ctx context.Context, sessionID string, userQuery string, history []llm.Message) (*agent.RunResult, error)
}

// Handler HTTP 处理器（仅依赖 Engine + DocumentService，不直接调用 storage）
type Handler struct {
	engine     *eino.Engine
	docService appcore.DocumentService
	agent      AgentRunner
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

// AgentRun Agent 入口：解析 query、可选 session_id，调用 Agent.Run，返回 JSON
func (h *Handler) AgentRun(ctx context.Context, c *app.RequestContext) {
	if h.agent == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent 未配置",
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
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	history := req.History
	if history == nil {
		history = []llm.Message{}
	}
	result, err := h.agent.Run(ctx, sessionID, req.Query, history)
	if err != nil {
		hlog.CtxErrorf(ctx, "Agent Run 失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "Agent 执行失败",
			"details": err.Error(),
		})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":      "success",
		"session_id":  sessionID,
		"answer":      result.Answer,
		"steps":       result.Steps,
		"duration_ms": result.Duration.Milliseconds(),
	})
}
