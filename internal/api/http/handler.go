package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/prometheus/common/expfmt"

	"rag-platform/internal/agent"
	"rag-platform/internal/agent/job"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/tools"
	agentruntime "rag-platform/internal/agent/runtime"
	appcore "rag-platform/internal/app"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/runtime/jobstore"
	"rag-platform/internal/runtime/session"
	"rag-platform/pkg/metrics"
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
	agentManager   *agentruntime.Manager
	agentScheduler *agentruntime.Scheduler
	agentCreator   AgentCreator
	jobStore        job.JobStore
	jobEventStore   jobstore.JobStore
	toolsRegistry   *tools.Registry
	agentStateStore agentruntime.AgentStateStore
	// planAtJobCreation 在 Job 创建时生成并持久化 Plan（1.0：执行阶段只读，禁止再调 Planner）
	planAtJobCreation func(ctx context.Context, agentID, goal string) (*planner.TaskGraph, error)
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

// SetJobStore 设置 Job 存储；设置后 POST /api/agents/:id/message 将创建 Job 并由 JobRunner 拉取执行，不再通过 WakeAgent 直接触发
func (h *Handler) SetJobStore(store job.JobStore) {
	h.jobStore = store
}

// SetJobEventStore 设置任务事件存储；设置后 message 创建任务时会先追加 JobCreated 事件（与 SetJobStore 双写）
func (h *Handler) SetJobEventStore(store jobstore.JobStore) {
	h.jobEventStore = store
}

// SetToolsRegistry 设置工具注册表；设置后提供 GET /api/tools 与 GET /api/tools/:name
func (h *Handler) SetToolsRegistry(reg *tools.Registry) {
	h.toolsRegistry = reg
}

// SetAgentStateStore 设置 Agent 状态存储；设置后 message 与 runJob 会持久化/加载会话，供 Worker 恢复
func (h *Handler) SetAgentStateStore(store agentruntime.AgentStateStore) {
	h.agentStateStore = store
}

// SetPlanAtJobCreation 设置 Job 创建时规划函数；传入后 POST message 将先 Append PlanGenerated 再返回 202（1.0 确定性执行）
func (h *Handler) SetPlanAtJobCreation(fn func(ctx context.Context, agentID, goal string) (*planner.TaskGraph, error)) {
	h.planAtJobCreation = fn
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

// SystemMetrics 系统指标（Prometheus 文本格式，供 /metrics 抓取）
func (h *Handler) SystemMetrics(ctx context.Context, c *app.RequestContext) {
	var buf bytes.Buffer
	if err := metrics.WritePrometheus(&buf); err != nil {
		hlog.CtxErrorf(ctx, "WritePrometheus: %v", err)
		c.AbortWithStatus(consts.StatusInternalServerError)
		return
	}
	c.Header("Content-Type", string(expfmt.FmtText))
	c.Write(buf.Bytes())
}

// workersLister 可选接口：事件存储为 Postgres 时支持列出活跃 Worker
type workersLister interface {
	ListActiveWorkerIDs(ctx context.Context) ([]string, error)
}

// SystemWorkers 返回当前有未过期租约的 Worker 列表（GET /api/system/workers，供 CLI corag workers）
func (h *Handler) SystemWorkers(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusOK, map[string]interface{}{"workers": []string{}, "total": 0})
		return
	}
	wl, ok := h.jobEventStore.(workersLister)
	if !ok {
		c.JSON(consts.StatusOK, map[string]interface{}{"workers": []string{}, "total": 0, "message": "事件存储不支持列出 Worker"})
		return
	}
	ids, err := wl.ListActiveWorkerIDs(ctx)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListActiveWorkerIDs: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取 Worker 列表失败"})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{"workers": ids, "total": len(ids)})
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

// AgentMessage 向 Agent 发送消息：写入 Session；若已设置 JobStore 则创建 Job 由 JobRunner 拉取执行，否则通过 WakeAgent 触发（兼容旧行为）
func (h *Handler) AgentMessage(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil {
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
	if h.agentStateStore != nil {
		state := agentruntime.SessionToAgentState(agent.Session)
		_ = h.agentStateStore.SaveAgentState(ctx, id, agent.Session.ID, state)
	}
	if h.jobStore != nil {
		// 先创建 Job 得到稳定 jobID，再双写事件流，避免 Create 失败时留下孤立事件
		j := &job.Job{AgentID: id, Goal: req.Message, Status: job.StatusPending, SessionID: agent.Session.ID}
		jobIDOut, errCreate := h.jobStore.Create(ctx, j)
		if errCreate != nil {
			hlog.CtxErrorf(ctx, "创建 Job 失败: %v", errCreate)
			c.JSON(consts.StatusInternalServerError, map[string]string{
				"error": "创建任务失败",
			})
			return
		}
		if h.jobEventStore != nil {
			payload, _ := json.Marshal(map[string]string{"agent_id": id, "goal": req.Message})
			ver, errAppend := h.jobEventStore.Append(ctx, jobIDOut, 0, jobstore.JobEvent{
				JobID: jobIDOut, Type: jobstore.JobCreated, Payload: payload,
			})
			if errAppend != nil {
				hlog.CtxErrorf(ctx, "追加 JobCreated 事件失败（Job 已创建，可继续执行）: %v", errAppend)
			} else if h.planAtJobCreation != nil {
				// 1.0 Plan 事件化：Job 创建时即生成并持久化 TaskGraph，执行阶段只读
				taskGraph, planErr := h.planAtJobCreation(ctx, id, req.Message)
				if planErr != nil {
					hlog.CtxErrorf(ctx, "Job 创建时 Plan 失败: %v", planErr)
					c.JSON(consts.StatusInternalServerError, map[string]string{
						"error": "规划失败，请重试",
					})
					return
				}
				if taskGraph != nil {
					graphBytes, _ := taskGraph.Marshal()
					payloadPlan, _ := json.Marshal(map[string]interface{}{
						"task_graph": json.RawMessage(graphBytes),
						"goal":       req.Message,
					})
					_, _ = h.jobEventStore.Append(ctx, jobIDOut, ver, jobstore.JobEvent{
						JobID: jobIDOut, Type: jobstore.PlanGenerated, Payload: payloadPlan,
					})
				}
			}
		}
		c.JSON(consts.StatusAccepted, map[string]interface{}{
			"status":   "accepted",
			"agent_id": id,
			"job_id":   jobIDOut,
		})
		return
	}
	if h.agentScheduler != nil {
		_ = h.agentScheduler.WakeAgent(ctx, id)
	}
	c.JSON(consts.StatusAccepted, map[string]interface{}{
		"status":   "accepted",
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

// ListAgentJobs 列出该 Agent 的 Job 列表（可选 status、limit 查询参数）
func (h *Handler) ListAgentJobs(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent Runtime 未配置",
		})
		return
	}
	if h.jobStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Job 未启用",
		})
		return
	}
	id := c.Param("id")
	if _, err := h.agentManager.Get(ctx, id); err != nil {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "Agent 不存在"})
		return
	}
	list, err := h.jobStore.ListByAgent(ctx, id)
	if err != nil {
		hlog.CtxErrorf(ctx, "列出 Job 失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "列出任务失败"})
		return
	}
	statusFilter := c.Query("status")
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var jobs []*job.Job
	for _, j := range list {
		if statusFilter != "" && j.Status.String() != statusFilter {
			continue
		}
		jobs = append(jobs, j)
		if len(jobs) >= limit {
			break
		}
	}
	out := make([]map[string]interface{}, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, map[string]interface{}{
			"id":          j.ID,
			"agent_id":    j.AgentID,
			"goal":        j.Goal,
			"status":      j.Status.String(),
			"cursor":      j.Cursor,
			"retry_count": j.RetryCount,
			"created_at":  j.CreatedAt,
			"updated_at":  j.UpdatedAt,
		})
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"jobs":  out,
		"total": len(out),
	})
}

// GetAgentJob 返回单条 Job 详情（需属于该 Agent）
func (h *Handler) GetAgentJob(ctx context.Context, c *app.RequestContext) {
	if h.jobStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Job 未启用",
		})
		return
	}
	id := c.Param("id")
	jobID := c.Param("job_id")
	j, err := h.jobStore.Get(ctx, jobID)
	if err != nil || j == nil {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务不存在"})
		return
	}
	if j.AgentID != id {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务不存在"})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"id":          j.ID,
		"agent_id":    j.AgentID,
		"goal":        j.Goal,
		"status":      j.Status.String(),
		"cursor":      j.Cursor,
		"retry_count": j.RetryCount,
		"created_at":  j.CreatedAt,
		"updated_at":  j.UpdatedAt,
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

// GetJob 按 job_id 返回 Job 元数据（供 Trace 等使用）
func (h *Handler) GetJob(ctx context.Context, c *app.RequestContext) {
	if h.jobStore == nil || h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Job 未启用"})
		return
	}
	jobID := c.Param("id")
	j, err := h.jobStore.Get(ctx, jobID)
	if err != nil || j == nil {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务不存在"})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"id":          j.ID,
		"agent_id":    j.AgentID,
		"goal":        j.Goal,
		"status":      j.Status.String(),
		"cursor":      j.Cursor,
		"retry_count": j.RetryCount,
		"created_at":  j.CreatedAt,
		"updated_at":  j.UpdatedAt,
	})
}

// JobStop 请求取消执行中的 Job（POST /api/jobs/:id/stop）；Worker 轮询到后取消 runCtx，Job 进入 CANCELLED
func (h *Handler) JobStop(ctx context.Context, c *app.RequestContext) {
	if h.jobStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Job 未启用"})
		return
	}
	jobID := c.Param("id")
	j, err := h.jobStore.Get(ctx, jobID)
	if err != nil || j == nil {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务不存在"})
		return
	}
	if j.Status == job.StatusCompleted || j.Status == job.StatusFailed || j.Status == job.StatusCancelled {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "任务已结束，无法取消"})
		return
	}
	if err := h.jobStore.RequestCancel(ctx, jobID); err != nil {
		hlog.CtxErrorf(ctx, "RequestCancel 失败: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "取消失败"})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id":  jobID,
		"status":  "cancelling",
		"message": "已请求取消，Worker 将中断执行",
	})
}

// GetJobReplay 返回只读的 Replay 视图（从事件流推导，不触发任何执行）；供 1.0 认证「Replay 一致性」校验
func (h *Handler) GetJobReplay(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取 Replay 失败: " + err.Error()})
		return
	}
	timeline := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		payload := json.RawMessage(e.Payload)
		if len(e.Payload) == 0 {
			payload = []byte("null")
		}
		entry := map[string]interface{}{
			"type":       string(e.Type),
			"created_at": e.CreatedAt,
			"payload":    payload,
		}
		var pl map[string]interface{}
		if _ = json.Unmarshal(e.Payload, &pl); pl != nil {
			if n, ok := pl["node_id"]; ok {
				entry["node_id"] = n
			}
		}
		timeline = append(timeline, entry)
	}
	goal := ""
	if h.jobStore != nil {
		if j, _ := h.jobStore.Get(ctx, jobID); j != nil {
			goal = j.Goal
		}
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id":    jobID,
		"goal":      goal,
		"read_only": true,
		"timeline":  timeline,
	})
}

// GetJobEvents 返回该 Job 的原始事件列表
func (h *Handler) GetJobEvents(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取事件失败"})
		return
	}
	out := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		payload := json.RawMessage(e.Payload)
		if len(e.Payload) == 0 {
			payload = []byte("null")
		}
		out = append(out, map[string]interface{}{
			"id":         e.ID,
			"job_id":     e.JobID,
			"type":       string(e.Type),
			"payload":    payload,
			"created_at": e.CreatedAt,
		})
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id": jobID,
		"events": out,
	})
}

// GetJobTrace 返回执行时间线（由事件流派生）
func (h *Handler) GetJobTrace(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取时间线失败: " + err.Error()})
		return
	}
	timeline := make([]map[string]interface{}, 0, len(events))
	nodeStarted := make(map[string]time.Time)
	nodeDurations := make([]map[string]interface{}, 0)
	for _, e := range events {
		payload := json.RawMessage(e.Payload)
		if len(e.Payload) == 0 {
			payload = []byte("null")
		}
		entry := map[string]interface{}{
			"type":       string(e.Type),
			"created_at": e.CreatedAt,
			"payload":    payload,
		}
		var pl map[string]interface{}
		if _ = json.Unmarshal(e.Payload, &pl); pl != nil {
			if n, ok := pl["node_id"]; ok {
				nodeID, _ := n.(string)
				entry["node_id"] = n
				if e.Type == jobstore.NodeStarted && nodeID != "" {
					nodeStarted[nodeID] = e.CreatedAt
				}
				if e.Type == jobstore.NodeFinished && nodeID != "" {
					if startAt, ok := nodeStarted[nodeID]; ok {
						durMs := e.CreatedAt.Sub(startAt).Milliseconds()
						nodeDurations = append(nodeDurations, map[string]interface{}{
							"node_id":      nodeID,
							"started_at":  startAt,
							"finished_at": e.CreatedAt,
							"duration_ms": durMs,
						})
					}
				}
			}
		}
		timeline = append(timeline, entry)
	}
	executionTree := BuildExecutionTree(events)
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id":          jobID,
		"timeline":        timeline,
		"node_durations":  nodeDurations,
		"execution_tree":  executionTree,
	})
}

// GetJobNode 返回某节点的相关事件与 payload（输入/输出等）
func (h *Handler) GetJobNode(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	nodeID := c.Param("node_id")
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取节点详情失败"})
		return
	}
	var nodeEvents []map[string]interface{}
	for _, e := range events {
		var pl map[string]interface{}
		if len(e.Payload) > 0 {
			_ = json.Unmarshal(e.Payload, &pl)
		}
		evNodeID, _ := pl["node_id"].(string)
		if evNodeID != nodeID {
			continue
		}
		payload := json.RawMessage(e.Payload)
		if len(e.Payload) == 0 {
			payload = []byte("null")
		}
		nodeEvents = append(nodeEvents, map[string]interface{}{
			"type":       string(e.Type),
			"created_at": e.CreatedAt,
			"payload":    payload,
		})
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id": jobID,
		"node_id": nodeID,
		"events":  nodeEvents,
	})
}

// GetJobTracePage 返回简单 Trace 回放页（HTML）
func (h *Handler) GetJobTracePage(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil || h.jobStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Trace 未启用"})
		return
	}
	jobID := c.Param("id")
	j, _ := h.jobStore.Get(ctx, jobID)
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取事件失败"})
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.WriteString(buildTraceHTML(jobID, j, events))
}

func buildTraceHTML(jobID string, j *job.Job, events []jobstore.JobEvent) string {
	status := "unknown"
	goal := ""
	if j != nil {
		status = j.Status.String()
		goal = j.Goal
	}
	s := "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Trace " + jobID + "</title></head><body>"
	s += "<h1>Job: " + jobID + "</h1>"
	s += "<p><b>Goal:</b> " + goal + "</p>"
	s += "<p><b>Status:</b> " + status + "</p>"
	tree := BuildExecutionTree(events)
	s += "<h2>Execution Tree (User → Plan → Node → Tool)</h2><ul>"
	s += ExecutionTreeToHTML(tree)
	s += "</ul>"
	s += "<h2>Timeline</h2><ul>"
	for _, e := range events {
		s += "<li><code>" + string(e.Type) + "</code> " + e.CreatedAt.Format("15:04:05") + " <pre>" + string(e.Payload) + "</pre></li>"
	}
	s += "</ul></body></html>"
	return s
}

// ListTools 返回所有工具的 Manifest 列表（GET /api/tools）
func (h *Handler) ListTools(ctx context.Context, c *app.RequestContext) {
	if h.toolsRegistry == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "工具注册表未配置"})
		return
	}
	manifests := h.toolsRegistry.Manifests()
	c.JSON(consts.StatusOK, map[string]interface{}{
		"tools": manifests,
		"total": len(manifests),
	})
}

// GetTool 返回指定名称工具的 Manifest（GET /api/tools/:name）
func (h *Handler) GetTool(ctx context.Context, c *app.RequestContext) {
	if h.toolsRegistry == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "工具注册表未配置"})
		return
	}
	name := c.Param("name")
	m := h.toolsRegistry.Manifest(name)
	if m == nil {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "工具不存在"})
		return
	}
	c.JSON(consts.StatusOK, m)
}
