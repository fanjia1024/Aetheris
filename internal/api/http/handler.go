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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/prometheus/common/expfmt"

	"rag-platform/internal/agent"
	"rag-platform/internal/agent/instance"
	"rag-platform/internal/agent/job"
	"rag-platform/internal/agent/messaging"
	"rag-platform/internal/agent/planner"
	"rag-platform/internal/agent/replay"
	agentruntime "rag-platform/internal/agent/runtime"
	"rag-platform/internal/agent/signal"
	"rag-platform/internal/agent/tools"
	"rag-platform/internal/agent/verify"
	appcore "rag-platform/internal/app"
	"rag-platform/internal/model/llm"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/runtime/jobstore"
	"rag-platform/internal/runtime/session"
	"rag-platform/pkg/auth"
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

// IngestQueueForAPI 入库队列的 API 侧接口（入队与状态查询）；由 app 在 postgres 时注入
type IngestQueueForAPI interface {
	Enqueue(ctx context.Context, payload map[string]interface{}) (taskID string, err error)
	GetStatus(ctx context.Context, taskID string) (status string, result interface{}, errMsg string, completedAt interface{}, err error)
}

// Handler HTTP 处理器（仅依赖 Engine + DocumentService，不直接调用 storage）
type Handler struct {
	engine         *eino.Engine
	docService     appcore.DocumentService
	agent          AgentRunner
	sessionManager SessionManager

	// adkRunner 主 ADK Runner；非空时 POST /api/agent/run 与 resume/stream 使用 ADK
	adkRunner *adk.Runner

	// ingestQueue 可选；postgres 时用于异步入库入队与状态查询
	ingestQueue IngestQueueForAPI

	// v1 Agent Runtime
	agentManager       *agentruntime.Manager
	agentScheduler     *agentruntime.Scheduler
	agentCreator       AgentCreator
	jobStore           job.JobStore
	jobEventStore      jobstore.JobStore
	toolsRegistry      *tools.Registry
	agentStateStore    agentruntime.AgentStateStore
	agentInstanceStore instance.AgentInstanceStore
	agentMessagingBus  messaging.AgentMessagingBus
	// planAtJobCreation 在 Job 创建时生成并持久化 Plan（1.0：执行阶段只读，forbidden再调 Planner）
	planAtJobCreation func(ctx context.Context, agentID, goal string) (*planner.TaskGraph, error)
	// wakeupQueue 可选；非 nil 时 JobSignal/JobMessage 在 UpdateStatus(Pending) 后调用 NotifyReady，供 Worker 事件驱动唤醒（design/wakeup-index）
	wakeupQueue job.WakeupQueue
	// signalInbox 可选；非 nil 时 JobSignal 先写 inbox 再 Append wait_completed，保证 at-least-once（design/runtime-contract.md）
	signalInbox signal.SignalInbox
	// observabilityReader 可选；非 nil 时提供 GET /api/observability/summary（队列积压、卡住 Job）
	observabilityReader job.ObservabilityReader
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

// SetIngestQueue 设置入库队列（postgres 时由 app 注入，用于 POST /documents/upload/async 与 GET /documents/upload/status/:id）
func (h *Handler) SetIngestQueue(q IngestQueueForAPI) {
	h.ingestQueue = q
}

// SetSessionManager 设置 Session 管理器（用于 /api/agent/run 的 session 生命周期）
func (h *Handler) SetSessionManager(m SessionManager) {
	h.sessionManager = m
}

// SetADKRunner 设置主 ADK Runner；设置后 /api/agent/run、/api/agent/resume、/api/agent/stream 使用 ADK 执行
func (h *Handler) SetADKRunner(runner *adk.Runner) {
	h.adkRunner = runner
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

// SetAgentInstanceStore 设置 Agent Instance 存储；设置后 POST message 时若 Instance not found则 Create（design/agent-instance-model.md）
func (h *Handler) SetAgentInstanceStore(store instance.AgentInstanceStore) {
	h.agentInstanceStore = store
}

// SetAgentMessagingBus 设置 Agent 级消息总线；设置后 POST message 双写 agent_messages（design/agent-messaging-bus.md）
func (h *Handler) SetAgentMessagingBus(bus messaging.AgentMessagingBus) {
	h.agentMessagingBus = bus
}

// SetPlanAtJobCreation 设置 Job 创建时规划函数；传入后 POST message 将先 Append PlanGenerated 再返回 202（1.0 确定性执行）
func (h *Handler) SetPlanAtJobCreation(fn func(ctx context.Context, agentID, goal string) (*planner.TaskGraph, error)) {
	h.planAtJobCreation = fn
}

// SetWakeupQueue 设置唤醒队列；非 nil 时 JobSignal/JobMessage 在将 Job 置为 Pending 后调用 NotifyReady，Worker 可据此立即 Claim 而非仅轮询（design/wakeup-index）
func (h *Handler) SetWakeupQueue(q job.WakeupQueue) {
	h.wakeupQueue = q
}

// SetSignalInbox 设置 signal 收件箱；非 nil 时 JobSignal 先写 inbox 再 Append wait_completed，保证 at-least-once 送达
func (h *Handler) SetSignalInbox(inbox signal.SignalInbox) {
	h.signalInbox = inbox
}

// SetObservabilityReader 设置可观测性数据源；非 nil 时提供 GET /api/observability/summary
func (h *Handler) SetObservabilityReader(r job.ObservabilityReader) {
	h.observabilityReader = r
}

// getJobAndCheckTenant 按 jobID 取 Job 并校验当前请求租户；不通过时写 404 并返回 (nil, false)
func (h *Handler) getJobAndCheckTenant(ctx context.Context, c *app.RequestContext, jobID string) (*job.Job, bool) {
	if h.jobStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Job 未启用"})
		return nil, false
	}
	j, err := h.jobStore.Get(ctx, jobID)
	if err != nil || j == nil {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务not found"})
		return nil, false
	}
	tid := auth.GetTenantID(ctx)
	if tid == "" {
		tid = "default"
	}
	if j.TenantID != tid {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务not found"})
		return nil, false
	}
	return j, true
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
		hlog.CtxErrorf(ctx, "上传文档failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "上传文档failed",
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

// UploadDocumentAsync 异步入库：将文件入队后立即返回 202，由 Worker 消费执行 ingest_pipeline；需配置 postgres
func (h *Handler) UploadDocumentAsync(ctx context.Context, c *app.RequestContext) {
	if h.ingestQueue == nil {
		c.JSON(consts.StatusNotImplemented, map[string]string{
			"error": "异步入库requires配置 jobstore.type=postgres",
		})
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请上传文件",
		})
		return
	}
	opened, err := file.Open()
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "打开上传文件failed",
		})
		return
	}
	defer opened.Close()
	data, err := io.ReadAll(opened)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "读取上传文件failed",
		})
		return
	}
	payload := map[string]interface{}{
		"content_base64": base64.StdEncoding.EncodeToString(data),
		"filename":       file.Filename,
		"metadata": map[string]interface{}{
			"filename":    file.Filename,
			"size":        file.Size,
			"uploaded_at": time.Now(),
		},
	}
	taskID, err := h.ingestQueue.Enqueue(ctx, payload)
	if err != nil {
		hlog.CtxErrorf(ctx, "入库任务入队failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "入库任务入队failed",
			"details": err.Error(),
		})
		return
	}
	c.JSON(consts.StatusAccepted, map[string]interface{}{
		"task_id": taskID,
		"message": "已入队",
	})
}

// UploadStatus 查询异步入库任务状态（GET /documents/upload/status/:task_id）
func (h *Handler) UploadStatus(ctx context.Context, c *app.RequestContext) {
	if h.ingestQueue == nil {
		c.JSON(consts.StatusNotImplemented, map[string]string{
			"error": "任务状态查询requires配置 jobstore.type=postgres",
		})
		return
	}
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "缺少 task_id"})
		return
	}
	status, result, errMsg, completedAt, err := h.ingestQueue.GetStatus(ctx, taskID)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if status == "" {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务not found"})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"task_id":      taskID,
		"status":       status,
		"result":       result,
		"error":        errMsg,
		"completed_at": completedAt,
	})
}

// ListDocuments 列出文档
func (h *Handler) ListDocuments(ctx context.Context, c *app.RequestContext) {
	documents, err := h.docService.ListDocuments(ctx)
	if err != nil {
		hlog.CtxErrorf(ctx, "获取文档列表failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "获取文档列表failed",
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
			"error": "文档not found",
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
			"error": "删除文档failed",
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
			"error": "请求参数error",
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
			"error": "请求参数error",
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
		hlog.CtxErrorf(ctx, "查询failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "查询failed",
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
			"error": "请求参数error",
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
		"status":  "success",
		"results": results,
		"total":   len(results),
	})
}

// SystemStatus 系统状态
func (h *Handler) SystemStatus(ctx context.Context, c *app.RequestContext) {
	status := map[string]interface{}{
		"api_service":   "running",
		"agent_service": "running",
		"index_service": "running",
		"workflows":     h.engine.GetWorkflows(),
		"agents":        h.engine.GetAgents(),
		"timestamp":     time.Now(),
	}

	c.JSON(consts.StatusOK, status)
}

// jobStateGaugeStates 与 JobStateGauge 的 state 标签一致，用于 /metrics 时更新瞬时值
var jobStateGaugeStates = []string{"pending", "running", "waiting", "parked", "completed", "failed", "cancelled", "retrying"}

// SystemMetrics 系统指标（Prometheus 文本格式，供 /metrics 抓取）
func (h *Handler) SystemMetrics(ctx context.Context, c *app.RequestContext) {
	if h.observabilityReader != nil {
		counts, err := h.observabilityReader.CountByStatus(ctx)
		if err == nil {
			for _, state := range jobStateGaugeStates {
				n := int64(0)
				if c, ok := counts[state]; ok {
					n = c
				}
				metrics.JobStateGauge.WithLabelValues(state).Set(float64(n))
			}
		}
	}
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

// SystemWorkers 返回当前有未过期租约的 Worker 列表（GET /api/system/workers，供 CLI aetheris workers）
func (h *Handler) SystemWorkers(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusOK, map[string]interface{}{"workers": []string{}, "total": 0})
		return
	}
	wl, ok := h.jobEventStore.(workersLister)
	if !ok {
		c.JSON(consts.StatusOK, map[string]interface{}{"workers": []string{}, "total": 0, "message": "事件存储unsupported列出 Worker"})
		return
	}
	ids, err := wl.ListActiveWorkerIDs(ctx)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListActiveWorkerIDs: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取 Worker 列表failed"})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{"workers": ids, "total": len(ids)})
}

// AgentRunRequest POST /api/agent/run 请求体
type AgentRunRequest struct {
	Query     string        `json:"query" binding:"required"`
	SessionID string        `json:"session_id"`
	History   []llm.Message `json:"history"`
}

// sessionToADKMessages 将 session 历史转为 adk.Message 列表（最近 maxRounds 轮；0 表示不限制）
func sessionToADKMessages(sess *session.Session, maxRounds int) []adk.Message {
	if sess == nil {
		return nil
	}
	msgs := sess.CopyMessages()
	if len(msgs) == 0 {
		return nil
	}
	if maxRounds > 0 {
		rounds := 0
		for i := len(msgs) - 1; i >= 0 && rounds < maxRounds; i-- {
			if msgs[i].Role == "user" || msgs[i].Role == "assistant" {
				rounds++
			}
		}
		start := 0
		for i, m := range msgs {
			if m.Role == "user" || m.Role == "assistant" {
				rounds--
				if rounds < 0 {
					start = i
					break
				}
			}
		}
		msgs = msgs[start:]
	}
	out := make([]adk.Message, 0, len(msgs))
	for _, m := range msgs {
		var role schema.RoleType
		switch m.Role {
		case "user":
			role = schema.User
		case "assistant":
			role = schema.Assistant
		case "system":
			role = schema.System
		default:
			role = schema.RoleType(m.Role)
		}
		out = append(out, &schema.Message{Role: role, Content: m.Content})
	}
	return out
}

// runADK 使用 ADK Runner 执行一次对话；stream 为 false 时收集最终回复并写 JSON，为 true 时以 SSE 流式写出
func runADK(ctx context.Context, c *app.RequestContext, runner *adk.Runner, sess *session.Session, query string, sessionManager SessionManager, stream bool) {
	history := sessionToADKMessages(sess, 20)
	messages := make([]adk.Message, 0, len(history)+1)
	messages = append(messages, history...)
	messages = append(messages, schema.UserMessage(query))
	iter := runner.Run(ctx, messages)
	var lastContent string
	var steps int
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			hlog.CtxErrorf(ctx, "ADK Run 事件error: %v", event.Err)
			c.JSON(consts.StatusInternalServerError, map[string]interface{}{
				"error":   "Agent execution failed",
				"details": event.Err.Error(),
			})
			return
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			// 中断：可返回 checkpoint 等，此处简化为返回已生成内容
			break
		}
		msg, _, err := adk.GetMessage(event)
		if err == nil && msg != nil && msg.Content != "" {
			lastContent = msg.Content
			steps++
		}
	}
	sess.AddMessage("user", query)
	sess.AddMessage("assistant", lastContent)
	if sessionManager != nil {
		_ = sessionManager.Save(ctx, sess)
	}
	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.SetStatusCode(consts.StatusOK)
		c.WriteString("data: " + jsonString(map[string]interface{}{"answer": lastContent, "session_id": sess.ID}) + "\n\n")
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"status":      "success",
		"session_id":  sess.ID,
		"answer":      lastContent,
		"steps":       steps,
		"duration_ms": 0,
	})
}

// AgentResumeCheckpointRequest POST /api/agent/resume 请求体（ADK checkpoint 恢复）
type AgentResumeCheckpointRequest struct {
	CheckPointID string `json:"checkpoint_id" binding:"required"`
	SessionID    string `json:"session_id"`
}

// AgentResumeCheckpoint 从 checkpoint 恢复 ADK 执行
func (h *Handler) AgentResumeCheckpoint(ctx context.Context, c *app.RequestContext) {
	if h.adkRunner == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "ADK Runner not configured，无法 Resume",
		})
		return
	}
	var req AgentResumeCheckpointRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数error，requires checkpoint_id",
		})
		return
	}
	iter, err := h.adkRunner.Resume(ctx, req.CheckPointID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ADK Resume failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "Resume failed",
			"details": err.Error(),
		})
		return
	}
	var lastContent string
	var steps int
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			hlog.CtxErrorf(ctx, "ADK Resume 事件error: %v", event.Err)
			c.JSON(consts.StatusInternalServerError, map[string]interface{}{
				"error":   "Agent execution failed",
				"details": event.Err.Error(),
			})
			return
		}
		msg, _, getErr := adk.GetMessage(event)
		if getErr == nil && msg != nil && msg.Content != "" {
			lastContent = msg.Content
			steps++
		}
	}
	resp := map[string]interface{}{
		"status":        "success",
		"checkpoint_id": req.CheckPointID,
		"answer":        lastContent,
		"steps":         steps,
	}
	if req.SessionID != "" && h.sessionManager != nil {
		if sess, err := h.sessionManager.GetOrCreate(ctx, req.SessionID); err == nil {
			sess.AddMessage("assistant", lastContent)
			_ = h.sessionManager.Save(ctx, sess)
			resp["session_id"] = sess.ID
		}
	}
	c.JSON(consts.StatusOK, resp)
}

// AgentStream 流式执行（与 AgentRun 相同请求体，响应为 SSE）
func (h *Handler) AgentStream(ctx context.Context, c *app.RequestContext) {
	if h.adkRunner == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "ADK Runner not configured",
		})
		return
	}
	if h.sessionManager == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "SessionManager not configured",
		})
		return
	}
	var req AgentRunRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数error",
		})
		return
	}
	sess, err := h.sessionManager.GetOrCreate(ctx, req.SessionID)
	if err != nil {
		hlog.CtxErrorf(ctx, "Session GetOrCreate failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "获取或创建 Session failed",
			"details": err.Error(),
		})
		return
	}
	runADK(ctx, c, h.adkRunner, sess, req.Query, h.sessionManager, true)
}

func jsonString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func marshalJSON(ctx context.Context, v interface{}, scene string) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		hlog.CtxErrorf(ctx, "JSON serialize failed (%s): %v", scene, err)
		return nil, err
	}
	return b, nil
}

// AgentRun Agent 入口：找到或创建 session，在 session 上执行 Agent，保存后返回；优先使用 ADK Runner
func (h *Handler) AgentRun(ctx context.Context, c *app.RequestContext) {
	if h.sessionManager == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "SessionManager not configured",
		})
		return
	}
	var req AgentRunRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数error",
		})
		return
	}
	sess, err := h.sessionManager.GetOrCreate(ctx, req.SessionID)
	if err != nil {
		hlog.CtxErrorf(ctx, "Session GetOrCreate failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "获取或创建 Session failed",
			"details": err.Error(),
		})
		return
	}
	if h.adkRunner != nil {
		runADK(ctx, c, h.adkRunner, sess, req.Query, h.sessionManager, false)
		return
	}
	if h.agent == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent not configured",
		})
		return
	}
	result, err := h.agent.RunWithSession(ctx, sess, req.Query)
	if err != nil {
		hlog.CtxErrorf(ctx, "Agent Run failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "Agent execution failed",
			"details": err.Error(),
		})
		return
	}
	if err := h.sessionManager.Save(ctx, sess); err != nil {
		hlog.CtxErrorf(ctx, "Session Save failed: %v", err)
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
			"error": "Agent Runtime not configured",
		})
		return
	}
	var req CreateAgentRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数error",
		})
		return
	}
	agent, err := h.agentCreator.Create(ctx, req.Name)
	if err != nil {
		hlog.CtxErrorf(ctx, "创建 Agent failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error":   "创建 Agent failed",
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
			"error": "Agent Runtime not configured",
		})
		return
	}
	id := c.Param("id")
	var req AgentMessageRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{
			"error": "请求参数error",
		})
		return
	}
	agent, err := h.agentManager.Get(ctx, id)
	if err != nil || agent == nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "Agent not found",
		})
		return
	}
	if h.agentInstanceStore != nil {
		inst, _ := h.agentInstanceStore.Get(ctx, id)
		if inst == nil {
			_ = h.agentInstanceStore.Create(ctx, &instance.AgentInstance{
				ID: id, Status: instance.StatusIdle, Name: id,
			})
		}
	}
	tenantID := auth.GetTenantID(ctx)
	if tenantID == "" {
		tenantID = "default"
	}
	// 幂等：若带 Idempotency-Key 且该 Agent 下已有同 key 且同租户的 Job，直接返回已有 job_id（202）
	idempotencyKey := strings.TrimSpace(string(c.GetHeader("Idempotency-Key")))
	if idempotencyKey != "" && h.jobStore != nil {
		existing, _ := h.jobStore.GetByAgentAndIdempotencyKey(ctx, id, idempotencyKey)
		if existing != nil && existing.TenantID == tenantID {
			c.JSON(consts.StatusAccepted, map[string]interface{}{
				"status":   "accepted",
				"agent_id": id,
				"job_id":   existing.ID,
			})
			return
		}
	}
	agent.Session.AddMessage("user", req.Message)
	if h.agentStateStore != nil {
		state := agentruntime.SessionToAgentState(agent.Session)
		_ = h.agentStateStore.SaveAgentState(ctx, id, agent.Session.ID, state)
	}
	// JobStore 模式下由 AgentMessage 直接创建 Job，不再额外投递 inbox，避免同一消息重复建 Job。
	if h.agentMessagingBus != nil && h.jobStore == nil {
		_, _ = h.agentMessagingBus.Send(ctx, "", id, map[string]any{"message": req.Message}, &messaging.SendOptions{Kind: messaging.KindUser})
	}
	if h.jobStore != nil {
		// 先创建 Job 得到稳定 jobID，再双写事件流，避免 Create failed时留下孤立事件；多租户写入 TenantID
		j := &job.Job{AgentID: id, TenantID: tenantID, Goal: req.Message, Status: job.StatusPending, SessionID: agent.Session.ID, IdempotencyKey: idempotencyKey}
		jobIDOut, errCreate := h.jobStore.Create(ctx, j)
		if errCreate != nil {
			hlog.CtxErrorf(ctx, "创建 Job failed: %v", errCreate)
			c.JSON(consts.StatusInternalServerError, map[string]string{
				"error": "创建任务failed",
			})
			return
		}
		metrics.JobsTotal.WithLabelValues(tenantID, "pending").Inc()
		if h.jobEventStore != nil {
			payload, errMarshal := marshalJSON(ctx, map[string]string{"agent_id": id, "goal": req.Message}, "job_created_payload")
			if errMarshal != nil {
				c.JSON(consts.StatusInternalServerError, map[string]string{
					"error": "创建任务事件failed",
				})
				return
			}
			ver, errAppend := h.jobEventStore.Append(ctx, jobIDOut, 0, jobstore.JobEvent{
				JobID: jobIDOut, Type: jobstore.JobCreated, Payload: payload,
			})
			if errAppend != nil {
				hlog.CtxErrorf(ctx, "追加 JobCreated 事件failed（Job 已创建，可继续执行）: %v", errAppend)
			} else if h.planAtJobCreation != nil {
				// 1.0 Plan 事件化：Job 创建时即生成并持久化 TaskGraph，执行阶段只读
				taskGraph, planErr := h.planAtJobCreation(ctx, id, req.Message)
				if planErr != nil {
					hlog.CtxErrorf(ctx, "Job 创建时 Plan failed: %v", planErr)
					c.JSON(consts.StatusInternalServerError, map[string]string{
						"error": "规划failed，请重试",
					})
					return
				}
				if taskGraph != nil {
					graphBytes, _ := taskGraph.Marshal()
					planHash := ""
					if len(graphBytes) > 0 {
						h := sha256.Sum256(graphBytes)
						planHash = hex.EncodeToString(h[:])
					}
					payloadPlan, errMarshal := marshalJSON(ctx, map[string]interface{}{
						"task_graph": json.RawMessage(graphBytes),
						"goal":       req.Message,
						"plan_hash":  planHash,
					}, "plan_generated_payload")
					if errMarshal != nil {
						c.JSON(consts.StatusInternalServerError, map[string]string{
							"error": "计划事件serialize failed",
						})
						return
					}
					verPlan, errPlanAppend := h.jobEventStore.Append(ctx, jobIDOut, ver, jobstore.JobEvent{
						JobID: jobIDOut, Type: jobstore.PlanGenerated, Payload: payloadPlan,
					})
					if errPlanAppend != nil {
						hlog.CtxErrorf(ctx, "追加 PlanGenerated 事件failed: %v", errPlanAppend)
						c.JSON(consts.StatusInternalServerError, map[string]string{
							"error": "写入计划事件failed",
						})
						return
					}
					taskGraphSummary := string(graphBytes)
					if len(graphBytes) > 512 {
						taskGraphSummary = string(graphBytes[:512]) + "..."
					}
					dsPayload, errMarshal := marshalJSON(ctx, map[string]interface{}{
						"goal":               req.Message,
						"task_graph_summary": taskGraphSummary,
						"plan_hash":          planHash,
					}, "decision_snapshot_payload")
					if errMarshal != nil {
						hlog.CtxErrorf(ctx, "DecisionSnapshot serialize failed: %v", errMarshal)
					} else {
						if _, err := h.jobEventStore.Append(ctx, jobIDOut, verPlan, jobstore.JobEvent{
							JobID: jobIDOut, Type: jobstore.DecisionSnapshot, Payload: dsPayload,
						}); err != nil {
							hlog.CtxErrorf(ctx, "追加 DecisionSnapshot 事件failed（不影响主流程）: %v", err)
						}
					}
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
			"error": "Agent Runtime not configured",
		})
		return
	}
	id := c.Param("id")
	agent, err := h.agentManager.Get(ctx, id)
	if err != nil || agent == nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "Agent not found",
		})
		return
	}
	sess := agent.Session
	c.JSON(consts.StatusOK, map[string]interface{}{
		"agent_id":        agent.ID,
		"status":          agent.GetStatus().String(),
		"current_task":    sess.GetCurrentTask(),
		"last_checkpoint": sess.GetLastCheckpoint(),
		"updated_at":      sess.GetUpdatedAt(),
	})
}

// AgentResume 恢复 Agent 执行
func (h *Handler) AgentResume(ctx context.Context, c *app.RequestContext) {
	if h.agentManager == nil || h.agentScheduler == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{
			"error": "Agent Runtime not configured",
		})
		return
	}
	id := c.Param("id")
	agent, _ := h.agentManager.Get(ctx, id)
	if agent == nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "Agent not found",
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
			"error": "Agent Runtime not configured",
		})
		return
	}
	id := c.Param("id")
	agent, _ := h.agentManager.Get(ctx, id)
	if agent == nil {
		c.JSON(consts.StatusNotFound, map[string]string{
			"error": "Agent not found",
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
			"error": "Agent Runtime not configured",
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
		c.JSON(consts.StatusNotFound, map[string]string{"error": "Agent not found"})
		return
	}
	tenantID := auth.GetTenantID(ctx)
	list, err := h.jobStore.ListByAgent(ctx, id, tenantID)
	if err != nil {
		hlog.CtxErrorf(ctx, "列出 Job failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "列出任务failed"})
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
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务not found"})
		return
	}
	if j.AgentID != id {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务not found"})
		return
	}
	tid := auth.GetTenantID(ctx)
	if tid == "" {
		tid = "default"
	}
	if j.TenantID != tid {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "任务not found"})
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
			"error": "Agent Runtime not configured",
		})
		return
	}
	list, err := h.agentManager.List(ctx)
	if err != nil {
		hlog.CtxErrorf(ctx, "列出 Agent failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"error": "列出 Agent failed",
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

// GetJob 按 job_id 返回 Job 元数据（供 Trace 等使用）；若 status 为 waiting 则附带 wait_correlation_key 供 JobSignal 使用（design/runtime-contract.md）
func (h *Handler) GetJob(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Job 未启用"})
		return
	}
	jobID := c.Param("id")
	j, ok := h.getJobAndCheckTenant(ctx, c, jobID)
	if !ok {
		return
	}
	resp := map[string]interface{}{
		"id":          j.ID,
		"agent_id":    j.AgentID,
		"goal":        j.Goal,
		"status":      j.Status.String(),
		"cursor":      j.Cursor,
		"retry_count": j.RetryCount,
		"created_at":  j.CreatedAt,
		"updated_at":  j.UpdatedAt,
	}
	if j.Status == job.StatusWaiting && h.jobEventStore != nil {
		events, _, _ := h.jobEventStore.ListEvents(ctx, jobID)
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Type == jobstore.JobWaiting {
				p, _ := jobstore.ParseJobWaitingPayload(events[i].Payload)
				if p.CorrelationKey != "" {
					resp["wait_correlation_key"] = p.CorrelationKey
					resp["wait_node_id"] = p.NodeID
				}
				break
			}
		}
	}
	c.JSON(consts.StatusOK, resp)
}

// JobStop 请求取消执行中的 Job（POST /api/jobs/:id/stop）；Worker 轮询到后取消 runCtx，Job 进入 CANCELLED
func (h *Handler) JobStop(ctx context.Context, c *app.RequestContext) {
	jobID := c.Param("id")
	j, ok := h.getJobAndCheckTenant(ctx, c, jobID)
	if !ok {
		return
	}
	if j.Status == job.StatusCompleted || j.Status == job.StatusFailed || j.Status == job.StatusCancelled {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "任务已结束，无法取消"})
		return
	}
	if err := h.jobStore.RequestCancel(ctx, jobID); err != nil {
		hlog.CtxErrorf(ctx, "RequestCancel failed: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "取消failed"})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id":  jobID,
		"status":  "cancelling",
		"message": "已请求取消，Worker 将中断执行",
	})
}

// JobSignalRequest POST /api/jobs/:id/signal 请求体；correlation_key 必须与当前 job_waiting 事件的 correlation_key 一致（design/runtime-contract.md）
type JobSignalRequest struct {
	CorrelationKey string                 `json:"correlation_key" binding:"required"`
	Payload        map[string]interface{} `json:"payload"`
}

// lastEventIsWaitCompletedWithCorrelationKey 判断事件列表最后一条是否为 wait_completed 且 payload 中 correlation_key 一致（用于 signal/message 幂等）
func lastEventIsWaitCompletedWithCorrelationKey(events []jobstore.JobEvent, correlationKey string) bool {
	if len(events) == 0 || correlationKey == "" {
		return false
	}
	last := events[len(events)-1]
	if last.Type != jobstore.WaitCompleted {
		return false
	}
	var m map[string]interface{}
	if json.Unmarshal(last.Payload, &m) != nil {
		return false
	}
	ck, _ := m["correlation_key"].(string)
	return ck == correlationKey
}

// JobSignal 向挂起的 Job 发送 signal，写入 wait_completed 事件并将 Job 置回 Pending 供 Worker 认领继续
func (h *Handler) JobSignal(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Job 或事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	j, ok := h.getJobAndCheckTenant(ctx, c, jobID)
	if !ok {
		return
	}
	if j.Status != job.StatusWaiting && j.Status != job.StatusParked {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "任务未在等待状态（Waiting/Parked），无法 signal"})
		return
	}
	events, ver, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取事件failed"})
		return
	}
	var waitPayload jobstore.JobWaitingPayload
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == jobstore.JobWaiting {
			waitPayload, _ = jobstore.ParseJobWaitingPayload(events[i].Payload)
			break
		}
	}
	if waitPayload.CorrelationKey == "" {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "job_waiting not found (missing correlation_key)"})
		return
	}
	var req JobSignalRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "请求体需包含 correlation_key"})
		return
	}
	if req.CorrelationKey != waitPayload.CorrelationKey {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "correlation_key 与当前等待不匹配"})
		return
	}
	// 幂等：若最后一条事件已是 wait_completed 且 correlation_key 一致，视为已送达，直接 200
	if lastEventIsWaitCompletedWithCorrelationKey(events, req.CorrelationKey) {
		c.JSON(consts.StatusOK, map[string]interface{}{
			"job_id":  jobID,
			"status":  j.Status,
			"message": "signal 已送达（幂等）",
		})
		return
	}
	if req.Payload == nil {
		req.Payload = make(map[string]interface{})
	}
	nodeID := waitPayload.NodeID
	payloadBytes, errMarshal := marshalJSON(ctx, req.Payload, "job_signal_request_payload")
	if errMarshal != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "signal payload 非法，无法序列化"})
		return
	}
	// 2.0 at-least-once：先写持久化 inbox，再 Append wait_completed，API 崩溃不丢 signal
	var signalID string
	if h.signalInbox != nil {
		signalID, err = h.signalInbox.Append(ctx, jobID, req.CorrelationKey, payloadBytes)
		if err != nil {
			hlog.CtxErrorf(ctx, "SignalInbox.Append: %v", err)
			c.JSON(consts.StatusInternalServerError, map[string]string{"error": "写入 signal 收件箱failed"})
			return
		}
	}
	evPayload, errMarshal := marshalJSON(ctx, map[string]interface{}{
		"node_id":         nodeID,
		"payload":         json.RawMessage(payloadBytes),
		"correlation_key": req.CorrelationKey,
	}, "job_wait_completed_payload")
	if errMarshal != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "构建 wait_completed 事件failed"})
		return
	}
	_, err = h.jobEventStore.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.WaitCompleted, Payload: evPayload,
	})
	if err != nil {
		if errors.Is(err, jobstore.ErrVersionMismatch) {
			latestEvents, _, listErr := h.jobEventStore.ListEvents(ctx, jobID)
			if listErr == nil && lastEventIsWaitCompletedWithCorrelationKey(latestEvents, req.CorrelationKey) {
				if h.signalInbox != nil && signalID != "" {
					_ = h.signalInbox.MarkAcked(ctx, jobID, signalID)
				}
				c.JSON(consts.StatusOK, map[string]interface{}{
					"job_id":  jobID,
					"status":  j.Status,
					"message": "signal 已送达（并发幂等）",
				})
				return
			}
		}
		hlog.CtxErrorf(ctx, "Append WaitCompleted: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "write event failed"})
		return
	}
	if err := h.jobStore.UpdateStatus(ctx, jobID, job.StatusPending); err != nil {
		hlog.CtxErrorf(ctx, "UpdateStatus Pending: %v", err)
	}
	if h.signalInbox != nil && signalID != "" {
		_ = h.signalInbox.MarkAcked(ctx, jobID, signalID)
	}
	if h.wakeupQueue != nil {
		_ = h.wakeupQueue.NotifyReady(ctx, jobID)
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id":  jobID,
		"status":  "pending",
		"message": "已发送 signal，Job 将重新入队执行",
	})
}

// JobMessageRequest POST /api/jobs/:id/message 请求体；向 Job 投递信箱消息，若 Job 处于 Waiting 且 wait_type=message 且 channel/correlation_key 匹配则写入 wait_completed 并重新入队（design/agent-process-model.md Mailbox）
type JobMessageRequest struct {
	MessageID      string                 `json:"message_id"`
	Channel        string                 `json:"channel"`
	CorrelationKey string                 `json:"correlation_key"`
	Payload        map[string]interface{} `json:"payload"`
}

// JobMessage 向指定 Job 写入一条 agent_message 事件；若 Job 处于 Waiting 且当前 job_waiting 的 wait_type=message 且 channel 或 correlation_key 匹配，则追加 wait_completed 并将 Job 置为 Pending
func (h *Handler) JobMessage(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Job 或事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	j, ok := h.getJobAndCheckTenant(ctx, c, jobID)
	if !ok {
		return
	}
	var req JobMessageRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "请求体需为 JSON"})
		return
	}
	if req.Payload == nil {
		req.Payload = make(map[string]interface{})
	}
	if req.MessageID == "" {
		req.MessageID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	msgPayload := jobstore.AgentMessagePayload{
		MessageID:      req.MessageID,
		Channel:        req.Channel,
		CorrelationKey: req.CorrelationKey,
		Payload:        req.Payload,
	}
	msgBytes, errMarshal := marshalJSON(ctx, msgPayload, "job_message_payload")
	if errMarshal != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "message payload 非法，无法序列化"})
		return
	}
	events, ver, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取事件failed"})
		return
	}
	_, err = h.jobEventStore.Append(ctx, jobID, ver, jobstore.JobEvent{
		JobID: jobID, Type: jobstore.AgentMessage, Payload: msgBytes,
	})
	if err != nil {
		hlog.CtxErrorf(ctx, "Append AgentMessage: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "写入消息failed"})
		return
	}
	if h.agentMessagingBus != nil {
		_, _ = h.agentMessagingBus.Send(ctx, "", j.AgentID, req.Payload, &messaging.SendOptions{Channel: req.Channel, Kind: messaging.KindUser})
	}
	if j.Status == job.StatusWaiting || j.Status == job.StatusParked {
		var waitPayload jobstore.JobWaitingPayload
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Type == jobstore.JobWaiting {
				waitPayload, _ = jobstore.ParseJobWaitingPayload(events[i].Payload)
				break
			}
		}
		matches := waitPayload.WaitType == "message" && waitPayload.CorrelationKey != "" &&
			(waitPayload.CorrelationKey == req.Channel || waitPayload.CorrelationKey == req.CorrelationKey || req.Channel == waitPayload.CorrelationKey)
		if matches {
			// 幂等：若最后一条事件已是 wait_completed 且 correlation_key 一致，视为已送达
			if lastEventIsWaitCompletedWithCorrelationKey(events, waitPayload.CorrelationKey) {
				c.JSON(consts.StatusOK, map[string]interface{}{
					"job_id":  jobID,
					"status":  "pending",
					"message": "消息已投递并解除等待（幂等）",
				})
				return
			}
			evPayload, errMarshal := marshalJSON(ctx, map[string]interface{}{
				"node_id":         waitPayload.NodeID,
				"payload":         req.Payload,
				"correlation_key": waitPayload.CorrelationKey,
			}, "job_message_wait_completed_payload")
			if errMarshal != nil {
				c.JSON(consts.StatusInternalServerError, map[string]string{"error": "构建 wait_completed 事件failed"})
				return
			}
			_, ver2, _ := h.jobEventStore.ListEvents(ctx, jobID)
			_, _ = h.jobEventStore.Append(ctx, jobID, ver2, jobstore.JobEvent{
				JobID: jobID, Type: jobstore.WaitCompleted, Payload: evPayload,
			})
			_ = h.jobStore.UpdateStatus(ctx, jobID, job.StatusPending)
			if h.wakeupQueue != nil {
				_ = h.wakeupQueue.NotifyReady(ctx, jobID)
			}
			c.JSON(consts.StatusOK, map[string]interface{}{
				"job_id":  jobID,
				"status":  "pending",
				"message": "已投递消息并解除等待，Job 将重新入队执行",
			})
			return
		}
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id":  jobID,
		"message": "已写入 agent_message 事件",
	})
}

// GetJobReplay 返回只读的 Replay 视图（从事件流推导，不触发任何执行）；含 current_state 供 Query 语义（design/agent-process-model.md）
func (h *Handler) GetJobReplay(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	if _, ok := h.getJobAndCheckTenant(ctx, c, jobID); !ok {
		return
	}
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取 Replay failed: " + err.Error()})
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
	resp := map[string]interface{}{
		"job_id":    jobID,
		"goal":      goal,
		"read_only": true,
		"timeline":  timeline,
	}
	stepNodeID := c.Query("step_node_id")
	// Query 语义：当前执行状态（已完成节点、游标、阶段），不推进执行
	builder := replay.NewReplayContextBuilder(h.jobEventStore)
	if rc, errBuild := builder.BuildFromEvents(ctx, jobID); errBuild == nil && rc != nil {
		completedIDs := make([]string, 0, len(rc.CompletedNodeIDs))
		for id := range rc.CompletedNodeIDs {
			completedIDs = append(completedIDs, id)
		}
		phaseStr := "unknown"
		switch rc.Phase {
		case replay.PhasePlanning:
			phaseStr = "planning"
		case replay.PhaseExecuting:
			phaseStr = "executing"
		case replay.PhaseCompleted:
			phaseStr = "completed"
		case replay.PhaseFailed:
			phaseStr = "failed"
		case replay.PhaseCancelled:
			phaseStr = "cancelled"
		}
		resp["current_state"] = map[string]interface{}{
			"completed_node_ids": completedIDs,
			"cursor_node":        rc.CursorNode,
			"phase":              phaseStr,
		}
		if stepNodeID != "" {
			stateAtStep := json.RawMessage([]byte("{}"))
			if payloadBytes, ok := rc.PayloadResultsByNode[stepNodeID]; ok && len(payloadBytes) > 0 {
				stateAtStep = json.RawMessage(payloadBytes)
			}
			stepResult := json.RawMessage([]byte("{}"))
			if resultBytes, ok := rc.CommandResults[stepNodeID]; ok && len(resultBytes) > 0 {
				stepResult = json.RawMessage(resultBytes)
			}
			resp["step_replay"] = map[string]interface{}{
				"step_node_id":  stepNodeID,
				"state_at_step": stateAtStep,
				"step_result":   stepResult,
			}
		}
	}
	c.JSON(consts.StatusOK, resp)
}

// GetJobEvents 返回该 Job 的原始事件列表
func (h *Handler) GetJobEvents(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	if _, ok := h.getJobAndCheckTenant(ctx, c, jobID); !ok {
		return
	}
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取事件failed"})
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

// GetJobVerify 返回 Job 执行验证证明（design/verification-mode.md）：execution_hash、event_chain_root_hash、ledger proof、replay proof
func (h *Handler) GetJobVerify(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	if _, ok := h.getJobAndCheckTenant(ctx, c, jobID); !ok {
		return
	}
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取事件failed"})
		return
	}
	var replayBuilder replay.ReplayContextBuilder
	if h.jobEventStore != nil {
		replayBuilder = replay.NewReplayContextBuilder(h.jobEventStore)
	}
	result, err := verify.Compute(ctx, events, jobID, replayBuilder)
	if err != nil {
		hlog.CtxErrorf(ctx, "Verify Compute: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "验证计算failed"})
		return
	}
	c.JSON(consts.StatusOK, result)
}

// GetJobTrace 返回执行时间线（由事件流派生）
func (h *Handler) GetJobTrace(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	if _, ok := h.getJobAndCheckTenant(ctx, c, jobID); !ok {
		return
	}
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取时间线failed: " + err.Error()})
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
							"node_id":     nodeID,
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
	narrative := BuildNarrative(events)
	resp := map[string]interface{}{
		"job_id":            jobID,
		"timeline":          timeline,
		"node_durations":    nodeDurations,
		"execution_tree":    executionTree,
		"timeline_segments": narrative.TimelineSegments,
		"steps":             narrative.Steps,
	}
	for _, e := range events {
		if e.Type == jobstore.DecisionSnapshot && len(e.Payload) > 0 {
			var ds map[string]interface{}
			if _ = json.Unmarshal(e.Payload, &ds); ds != nil {
				resp["decision_snapshot"] = ds
			}
			break
		}
	}
	c.JSON(consts.StatusOK, resp)
}

// GetJobNode 返回某节点的相关事件与 payload（输入/输出等）
func (h *Handler) GetJobNode(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	if _, ok := h.getJobAndCheckTenant(ctx, c, jobID); !ok {
		return
	}
	nodeID := c.Param("node_id")
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取节点详情failed"})
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
		"job_id":  jobID,
		"node_id": nodeID,
		"events":  nodeEvents,
	})
}

// GetJobCognitionTrace 返回 Trace 2.0 Cognition 聚合（design/trace-2.0-cognition.md）：reasoning_step_timeline、decision_tree、plan_evolution、tool_dependency_graph、memory_read_write
func (h *Handler) GetJobCognitionTrace(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "事件存储未启用"})
		return
	}
	jobID := c.Param("id")
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListEvents: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取事件failed"})
		return
	}
	// reasoning_step_timeline: node_*, agent_thought_recorded, decision_made, tool_selected, tool_result_summarized
	reasoningTypes := map[jobstore.EventType]bool{
		jobstore.NodeStarted: true, jobstore.NodeFinished: true,
		jobstore.AgentThoughtRecorded: true, jobstore.DecisionMade: true,
		jobstore.ToolSelected: true, jobstore.ToolResultSummarized: true,
	}
	reasoningStepTimeline := make([]map[string]interface{}, 0)
	for _, e := range events {
		if !reasoningTypes[e.Type] {
			continue
		}
		payload := json.RawMessage(e.Payload)
		if len(e.Payload) == 0 {
			payload = []byte("null")
		}
		reasoningStepTimeline = append(reasoningStepTimeline, map[string]interface{}{
			"type":       string(e.Type),
			"created_at": e.CreatedAt,
			"payload":    payload,
		})
	}
	// decision_tree: 使用现有 BuildExecutionTree
	decisionTree := BuildExecutionTree(events)
	// plan_evolution: plan_generated + decision_snapshot + plan_evolution
	planEvolution := make([]map[string]interface{}, 0)
	for _, e := range events {
		if e.Type != jobstore.PlanGenerated && e.Type != jobstore.DecisionSnapshot && e.Type != jobstore.PlanEvolution {
			continue
		}
		payload := json.RawMessage(e.Payload)
		if len(e.Payload) == 0 {
			payload = []byte("null")
		}
		planEvolution = append(planEvolution, map[string]interface{}{
			"type":       string(e.Type),
			"created_at": e.CreatedAt,
			"payload":    payload,
		})
	}
	// tool_dependency_graph: TaskGraph 结构 + 工具相关事件摘要
	toolDepGraph := map[string]interface{}{"nodes": []interface{}{}, "edges": []interface{}{}, "tool_events": []interface{}{}}
	for _, e := range events {
		if e.Type == jobstore.PlanGenerated && len(e.Payload) > 0 {
			var pl map[string]interface{}
			if json.Unmarshal(e.Payload, &pl) == nil && pl != nil {
				if tg, ok := pl["task_graph"]; ok {
					toolDepGraph["task_graph"] = tg
				}
			}
			break
		}
	}
	for _, e := range events {
		if e.Type == jobstore.ToolSelected || e.Type == jobstore.ToolResultSummarized || e.Type == jobstore.ToolInvocationStarted || e.Type == jobstore.ToolInvocationFinished {
			payload := json.RawMessage(e.Payload)
			if len(e.Payload) == 0 {
				payload = []byte("null")
			}
			toolEvs, _ := toolDepGraph["tool_events"].([]interface{})
			toolDepGraph["tool_events"] = append(toolEvs, map[string]interface{}{"type": string(e.Type), "created_at": e.CreatedAt, "payload": payload})
		}
	}
	// memory_read_write: memory_read / memory_write 事件
	memoryReadWrite := make([]map[string]interface{}, 0)
	for _, e := range events {
		if e.Type != jobstore.MemoryRead && e.Type != jobstore.MemoryWrite {
			continue
		}
		payload := json.RawMessage(e.Payload)
		if len(e.Payload) == 0 {
			payload = []byte("null")
		}
		memoryReadWrite = append(memoryReadWrite, map[string]interface{}{
			"type":       string(e.Type),
			"created_at": e.CreatedAt,
			"payload":    payload,
		})
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"job_id":                  jobID,
		"reasoning_step_timeline": reasoningStepTimeline,
		"decision_tree":           decisionTree,
		"plan_evolution":          planEvolution,
		"tool_dependency_graph":   toolDepGraph,
		"memory_read_write":       memoryReadWrite,
	})
}

// GetTraceOverviewPage 返回 Trace 聚合页（按 agent 展示多 Job 概览）。
func (h *Handler) GetTraceOverviewPage(ctx context.Context, c *app.RequestContext) {
	if h.jobStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Job 未启用"})
		return
	}
	agentIDs := c.Query("agent_ids")
	if agentIDs == "" {
		agentIDs = c.Query("agent_id")
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Trace Overview</title><style>")
	b.WriteString("body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Arial,sans-serif;margin:1rem;} table{border-collapse:collapse;width:100%;margin-top:0.6rem;} th,td{border:1px solid #ddd;padding:0.4rem 0.6rem;text-align:left;} th{background:#f6f6f6;} .agent-block{margin:1rem 0;padding:0.8rem;border:1px solid #ddd;border-radius:6px;} .muted{color:#666;} input{padding:0.35rem 0.45rem;} button{padding:0.35rem 0.6rem;}")
	b.WriteString("</style></head><body>")
	b.WriteString("<h1>Trace UI 2.0 Overview</h1>")
	b.WriteString("<p class=\"muted\">Multi-job aggregation by agent. Click trace links to inspect single-job details and step-level replay.</p>")
	b.WriteString("<form id=\"q\"><label>Agent IDs (comma-separated): <input id=\"agent_ids\" name=\"agent_ids\" value=\"")
	b.WriteString(html.EscapeString(agentIDs))
	b.WriteString("\"/></label> <button type=\"submit\">Load</button></form>")
	b.WriteString("<div id=\"content\"></div>")
	b.WriteString("<script>(function(){ function esc(s){ return String(s||'').replace(/[&<>\\\"]/g,function(c){ return ({'&':'&amp;','<':'&lt;','>':'&gt;','\\\"':'&quot;'}[c]); }); } function parseIDs(){ var raw = document.getElementById('agent_ids').value || ''; return raw.split(',').map(function(s){ return s.trim(); }).filter(Boolean); } function renderBlock(agentID, jobs){ var html = '<div class=\"agent-block\"><h3>Agent: '+esc(agentID)+'</h3>'; html += '<table><thead><tr><th>Job ID</th><th>Status</th><th>Updated</th><th>Goal</th><th>Trace</th></tr></thead><tbody>'; if(!jobs || jobs.length===0){ html += '<tr><td colspan=\"5\" class=\"muted\">No jobs</td></tr>'; } else { jobs.forEach(function(j){ html += '<tr><td>'+esc(j.id)+'</td><td>'+esc(j.status)+'</td><td>'+esc(j.updated_at)+'</td><td>'+esc(j.goal)+'</td><td><a href=\"/api/jobs/'+encodeURIComponent(j.id)+'/trace/page\" target=\"_blank\">open trace</a></td></tr>'; }); } html += '</tbody></table></div>'; return html; } function load(){ var ids = parseIDs(); var content = document.getElementById('content'); if(ids.length===0){ content.innerHTML = '<p class=\"muted\">Enter at least one agent id.</p>'; return; } content.innerHTML = '<p class=\"muted\">Loading...</p>'; var reqs = ids.map(function(id){ return fetch('/api/agents/'+encodeURIComponent(id)+'/jobs?limit=50').then(function(r){ return r.ok ? r.json() : { jobs: [], _error: 'HTTP '+r.status }; }).then(function(data){ return { id:id, jobs:(data.jobs||[]), error:data._error||'' }; }).catch(function(e){ return { id:id, jobs:[], error:String(e) }; }); }); Promise.all(reqs).then(function(all){ var html=''; all.forEach(function(x){ html += renderBlock(x.id, x.jobs); if(x.error){ html += '<p class=\"muted\">'+esc(x.error)+'</p>'; } }); content.innerHTML = html; }); } document.getElementById('q').addEventListener('submit', function(e){ e.preventDefault(); load(); }); load(); })();</script>")
	b.WriteString("</body></html>")
	c.WriteString(b.String())
}

// GetJobTracePage 返回简单 Trace 回放页（HTML）
func (h *Handler) GetJobTracePage(ctx context.Context, c *app.RequestContext) {
	if h.jobEventStore == nil || h.jobStore == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "Trace 未启用"})
		return
	}
	jobID := c.Param("id")
	j, ok := h.getJobAndCheckTenant(ctx, c, jobID)
	if !ok {
		return
	}
	events, _, err := h.jobEventStore.ListEvents(ctx, jobID)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取事件failed"})
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
	escJobID := html.EscapeString(jobID)
	escGoal := html.EscapeString(goal)
	escStatus := html.EscapeString(status)

	tree := BuildExecutionTree(events)
	flatSteps := FlattenSteps(tree)
	narrative := BuildNarrative(events)
	timeline := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		payload := json.RawMessage(e.Payload)
		if len(e.Payload) == 0 {
			payload = []byte("null")
		}
		timeline = append(timeline, map[string]interface{}{
			"type":       string(e.Type),
			"created_at": e.CreatedAt,
			"payload":    payload,
		})
	}
	dagNodes, dagEdges := DAGNodesAndEdges(tree)
	traceData := map[string]interface{}{
		"job_id":            jobID,
		"goal":              goal,
		"status":            status,
		"steps":             narrative.Steps,
		"flat_steps":        flatSteps,
		"timeline_segments": narrative.TimelineSegments,
		"tree":              tree,
		"timeline":          timeline,
		"dag_nodes":         dagNodes,
		"dag_edges":         dagEdges,
	}
	jsonBytes, err := json.Marshal(traceData)
	if err != nil {
		jsonBytes = []byte("{}")
	}
	jsonStr := string(jsonBytes)

	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Trace ")
	b.WriteString(escJobID)
	b.WriteString("</title><style>")
	b.WriteString(".trace-layout{display:flex;gap:1rem;margin:1rem 0;min-height:400px;}")
	b.WriteString(".timeline-bar{display:flex;flex-wrap:wrap;gap:2px;margin:0.5rem 0;padding:4px;background:#f0f0f0;border-radius:4px;}")
	b.WriteString(".timeline-bar .seg{padding:4px 8px;border-radius:3px;font-size:0.8em;white-space:nowrap;}")
	b.WriteString(".timeline-bar .seg.plan{background:#cce;}.timeline-bar .seg.node{background:#cec;}.timeline-bar .seg.tool{background:#eec;}.timeline-bar .seg.recovery{background:#ecc;}")
	b.WriteString(".timeline-bar .seg.failed{background:#fcc;}.timeline-bar .seg.retryable{background:#fdc;}")
	b.WriteString(".step-timeline{flex:0 0 300px;border:1px solid #ccc;overflow-y:auto;}")
	b.WriteString(".step-timeline .step{padding:0.4rem 0.6rem;cursor:pointer;border-bottom:1px solid #eee;}")
	b.WriteString(".step-timeline .step:hover{background:#f0f0f0;}")
	b.WriteString(".step-timeline .step.selected{background:#e0e8ff;}")
	b.WriteString(".step-timeline .step .label{font-weight:500;}")
	b.WriteString(".step-timeline .step .meta{font-size:0.85em;color:#666;}")
	b.WriteString(".step-timeline .step .state{font-size:0.8em;margin-top:2px;}")
	b.WriteString(".detail-panel{flex:1;border:1px solid #ccc;padding:0.8rem;overflow-y:auto;}")
	b.WriteString(".detail-panel h3{margin-top:0;}")
	b.WriteString(".detail-panel pre{background:#f5f5f5;padding:0.5rem;overflow:auto;font-size:0.9em;}")
	b.WriteString(".detail-panel .placeholder{color:#888;font-style:italic;}")
	b.WriteString(".detail-panel .step-view{display:grid;grid-template-columns:auto 1fr;gap:0.2rem 1rem;margin-bottom:0.8rem;font-size:0.9em;}")
	b.WriteString(".detail-panel #detail-state-diff-section{margin-top:1rem;padding:0.6rem;background:#f8f9fa;border:1px solid #dee2e6;border-radius:6px;}")
	b.WriteString(".detail-panel #detail-state-diff-section h4{margin:0 0 0.4rem 0;font-size:1em;}")
	b.WriteString(".detail-panel #detail-state-diff .changed-keys-list{margin:0.3rem 0;}")
	b.WriteString(".tree-section{margin-top:1rem;}")
	b.WriteString(".tree-section summary{cursor:pointer;}")
	b.WriteString(".tree-section ul{list-style:none;padding-left:1rem;}")
	b.WriteString(".tree-section li{cursor:pointer;padding:0.2rem 0;}")
	b.WriteString(".tree-section li:hover{background:#f0f0f0;}")
	b.WriteString(".tree-section details summary{list-style:none;}")
	b.WriteString(".tree-section details summary::-webkit-details-marker{display:none;}")
	b.WriteString(".event-filter-bar{margin:0.5rem 0;display:flex;gap:1rem;align-items:center;}")
	b.WriteString(".event-filter-bar label{display:inline-flex;align-items:center;gap:0.3rem;cursor:pointer;}")
	b.WriteString(".dag-section{margin-top:1rem;padding:0.6rem;border:1px solid #ccc;border-radius:6px;}")
	b.WriteString(".dag-section h4{margin:0 0 0.5rem 0;}")
	b.WriteString(".dag-container{min-height:120px;overflow:auto;}")
	b.WriteString(".dag-container svg{font-size:12px;}")
	b.WriteString("</style></head><body>")
	b.WriteString("<h1>Job: ")
	b.WriteString(escJobID)
	b.WriteString("</h1><p><b>Goal:</b> ")
	b.WriteString(escGoal)
	b.WriteString("</p><p><b>Status:</b> ")
	b.WriteString(escStatus)
	b.WriteString("</p>")
	b.WriteString("<div class=\"event-filter-bar\" id=\"event-filter-bar\">")
	b.WriteString("<label><input type=\"checkbox\" class=\"filter-type\" value=\"plan\" checked> plan</label>")
	b.WriteString("<label><input type=\"checkbox\" class=\"filter-type\" value=\"node\" checked> node</label>")
	b.WriteString("<label><input type=\"checkbox\" class=\"filter-type\" value=\"tool\" checked> tool</label>")
	b.WriteString("<label><input type=\"checkbox\" class=\"filter-type\" value=\"recovery\" checked> recovery</label>")
	b.WriteString("</div>")
	b.WriteString("<div class=\"timeline-bar\" id=\"timeline-bar\"></div>")
	b.WriteString("<div class=\"trace-layout\"><div class=\"step-timeline\" id=\"step-timeline\">")

	for _, st := range narrative.Steps {
		b.WriteString("<div class=\"step\" data-span-id=\"")
		b.WriteString(html.EscapeString(st.SpanID))
		b.WriteString("\" data-type=\"")
		b.WriteString(html.EscapeString(st.Type))
		b.WriteString("\">")
		b.WriteString("<div class=\"label\">")
		b.WriteString(html.EscapeString(st.Label))
		b.WriteString("</div><div class=\"meta\">")
		if st.StartTime != nil {
			b.WriteString(st.StartTime.Format("15:04:05"))
		}
		if st.DurationMs > 0 {
			b.WriteString(" &middot; ")
			b.WriteString(strconv.FormatInt(st.DurationMs, 10))
			b.WriteString("ms")
		}
		b.WriteString("</div>")
		if st.State != "" && st.State != "ok" {
			b.WriteString("<div class=\"state\">")
			b.WriteString(html.EscapeString(st.State))
			if st.Attempts > 1 {
				b.WriteString(" &middot; attempt ")
				b.WriteString(strconv.Itoa(st.Attempts))
			}
			if st.WorkerID != "" {
				b.WriteString(" &middot; ")
				b.WriteString(html.EscapeString(st.WorkerID))
			}
			b.WriteString("</div>")
		}
		b.WriteString("</div>")
	}

	b.WriteString("</div><div class=\"detail-panel\" id=\"detail-panel\">")
	b.WriteString("<p class=\"placeholder\" id=\"detail-placeholder\">Select a step or tree node.</p>")
	b.WriteString("<div id=\"detail-content\" style=\"display:none;\">")
	b.WriteString("<h3>Step</h3><div class=\"step-view\" id=\"detail-step-view\"></div>")
	b.WriteString("<h3>Replay control</h3><div><button id=\"replay-step-btn\" type=\"button\">Replay selected step</button><pre id=\"replay-step-result\"></pre></div>")
	b.WriteString("<h3>Payload</h3><pre id=\"detail-payload\"></pre>")
	b.WriteString("<h3>Tool I/O</h3><pre id=\"detail-tool-io\"></pre>")
	b.WriteString("<h3>Reasoning</h3><div id=\"detail-reasoning\"></div>")
	b.WriteString("<h3>What changed</h3><div id=\"detail-state-diff-section\"><div id=\"detail-state-diff\"></div></div>")
	b.WriteString("</div></div></div>")
	b.WriteString("<div class=\"tree-section\"><details open><summary>Execution tree (User → Plan → Node → Tool)</summary>")
	b.WriteString("<ul id=\"trace-tree\">")
	b.WriteString(renderTraceTreeHTML(tree))
	b.WriteString("</ul></details></div>")
	b.WriteString("<div class=\"dag-section\"><h4>Execution DAG</h4><div class=\"dag-container\" id=\"dag-container\"></div></div>")
	b.WriteString("<script>window.__TRACE__ = ")
	b.WriteString(jsonStr)
	b.WriteString(";</script><script>")
	writeTracePageScript(&b)
	b.WriteString("</script><script>")
	writeTraceFilterAndDAGScript(&b)
	b.WriteString("</script><script>")
	writeTraceReplayControlScript(&b)
	b.WriteString("</script></body></html>")
	return b.String()
}

// writeTraceFilterAndDAGScript writes JS for event-type filter and DAG visualization.
func writeTraceFilterAndDAGScript(b *strings.Builder) {
	b.WriteString("(function(){ var T = window.__TRACE__; function getFilterTypes(){ var types = []; document.querySelectorAll('.filter-type:checked').forEach(function(cb){ types.push(cb.value); }); return types; } function renderBar(){ var types = getFilterTypes(); var segs = (T.timeline_segments || []).filter(function(s){ return types.indexOf(s.type) >= 0; }); var bar = document.getElementById('timeline-bar'); if(!bar) return; bar.innerHTML = ''; segs.forEach(function(s){ var c = s.type; if(s.status === 'permanent_failure' || s.status === 'compensatable_failure') c += ' failed'; else if(s.status === 'retryable_failure') c += ' retryable'; var d = document.createElement('span'); d.className = 'seg ' + c; d.textContent = s.label + (s.duration_ms ? ' ' + s.duration_ms + 'ms' : ''); bar.appendChild(d); }); } function renderDAG(){ var nodes = T.dag_nodes || []; var edges = T.dag_edges || []; var el = document.getElementById('dag-container'); if(!el || nodes.length === 0) return; var w = Math.max(400, el.offsetWidth || 400); var h = Math.max(120, Math.min(300, nodes.length * 36)); var pad = 24; var boxW = 100; var boxH = 28; var byId = {}; nodes.forEach(function(n, i){ byId[n.id] = { n: n, x: pad + (i % 6) * (boxW + 40), y: pad + Math.floor(i / 6) * (boxH + 20) }; }); var svg = '<svg width=\"' + w + '\" height=\"' + h + '\" xmlns=\"http://www.w3.org/2000/svg\">'; edges.forEach(function(e){ var from = byId[e.from]; var to = byId[e.to]; if(from && to){ var x1 = from.x + boxW/2; var y1 = from.y + boxH; var x2 = to.x + boxW/2; var y2 = to.y; svg += '<line x1=\"' + x1 + '\" y1=\"' + y1 + '\" x2=\"' + x2 + '\" y2=\"' + y2 + '\" stroke=\"#999\" stroke-width=\"1\"/>'; } }); nodes.forEach(function(n){ var o = byId[n.id]; if(!o) return; var x = o.x; var y = o.y; var fill = '#cec'; if(n.type === 'plan') fill = '#cce'; if(n.type === 'tool') fill = '#eec'; svg += '<rect x=\"' + x + '\" y=\"' + y + '\" width=\"' + boxW + '\" height=\"' + boxH + '\" fill=\"' + fill + '\" stroke=\"#666\" rx=\"4\"/>'; svg += '<text x=\"' + (x + boxW/2) + '\" y=\"' + (y + boxH/2 + 4) + '\" text-anchor=\"middle\" font-size=\"11\">' + (n.label.length > 12 ? n.label.slice(0,11) + '…' : n.label) + '</text>'; }); svg += '</svg>'; el.innerHTML = svg; } renderBar(); renderDAG(); document.querySelectorAll('.filter-type').forEach(function(cb){ cb.addEventListener('change', renderBar); }); })();")
}

// writeTracePageScript writes the Trace page JS: timeline bar + select() with step view, reasoning, state diff.
func writeTracePageScript(b *strings.Builder) {
	b.WriteString("(function(){ var T = window.__TRACE__; var ph = document.getElementById('detail-placeholder'); var content = document.getElementById('detail-content'); var stepViewEl = document.getElementById('detail-step-view'); var payloadEl = document.getElementById('detail-payload'); var toolIoEl = document.getElementById('detail-tool-io'); var reasoningEl = document.getElementById('detail-reasoning'); var stateDiffEl = document.getElementById('detail-state-diff'); var segs = T.timeline_segments || []; var bar = document.getElementById('timeline-bar'); segs.forEach(function(s){ var c = s.type; if(s.status === 'permanent_failure' || s.status === 'compensatable_failure') c += ' failed'; else if(s.status === 'retryable_failure') c += ' retryable'; var d = document.createElement('span'); d.className = 'seg ' + c; d.textContent = s.label + (s.duration_ms ? ' ' + s.duration_ms + 'ms' : ''); bar.appendChild(d); }); function row(el,k,v){ if(!v) return; var p = document.createElement('div'); p.textContent = k + ':'; var p2 = document.createElement('div'); p2.textContent = v; el.appendChild(p); el.appendChild(p2); } function select(spanId){ document.querySelectorAll('.step-timeline .step').forEach(function(el){ el.classList.toggle('selected', el.getAttribute('data-span-id') === spanId); }); document.querySelectorAll('.tree-section [data-span-id]').forEach(function(el){ el.classList.toggle('selected', el.getAttribute('data-span-id') === spanId); }); var step = T.steps.find(function(s){ return s.span_id === spanId; }); if(!step){ ph.style.display='block'; content.style.display='none'; return; } ph.style.display='none'; content.style.display='block'; stepViewEl.innerHTML = ''; row(stepViewEl,'Step', step.label); row(stepViewEl,'State', step.state || 'ok'); row(stepViewEl,'Attempts', step.attempts ? String(step.attempts) : ''); row(stepViewEl,'Worker', step.worker_id); row(stepViewEl,'Duration', step.duration_ms ? step.duration_ms + 'ms' : ''); row(stepViewEl,'Result type', step.result_type); row(stepViewEl,'Reason', step.reason); var events = T.timeline.filter(function(e){ try{ var p = typeof e.payload === 'string' ? JSON.parse(e.payload) : e.payload; return (p && (p.trace_span_id === spanId || p.node_id === spanId)); }catch(_){ return false;} }); payloadEl.textContent = events.length ? JSON.stringify(events.map(function(e){ return { type: e.type, created_at: e.created_at, payload: e.payload }; }), null, 2) : ''; var io = []; var inv = step.tool_invocation; if(inv){ if(inv.input) io.push('Input: ' + (typeof inv.input === 'string' ? inv.input : JSON.stringify(inv.input))); if(inv.output) io.push('Output: ' + (typeof inv.output === 'string' ? inv.output : JSON.stringify(inv.output))); if(inv.summary) io.push('Summary: ' + inv.summary); if(inv.error) io.push('Error: ' + inv.error); if(inv.idempotent) io.push('Idempotent: true'); } if(!io.length){ var flat = (T.flat_steps || []).find(function(s){ return s.span_id === spanId; }); if(flat){ if(flat.input) io.push('Input: ' + (typeof flat.input === 'string' ? flat.input : JSON.stringify(flat.input))); if(flat.output) io.push('Output: ' + (typeof flat.output === 'string' ? flat.output : JSON.stringify(flat.output))); } } toolIoEl.textContent = io.length ? io.join('\\n\\n') : '(none)'; reasoningEl.innerHTML = ''; if(step.reasoning && step.reasoning.length){ step.reasoning.forEach(function(r){ var p = document.createElement('p'); p.innerHTML = '<strong>' + (r.role || '') + '</strong>: ' + (r.content || ''); reasoningEl.appendChild(p); }); } else { var p = document.createElement('p'); p.className = 'placeholder'; p.textContent = 'Reasoning snapshot (none recorded)'; reasoningEl.appendChild(p); } stateDiffEl.innerHTML = ''; if(step.state_diff && (step.state_diff.state_before || step.state_diff.state_after || (step.state_diff.changed_keys && step.state_diff.changed_keys.length) || (step.state_diff.state_changes && step.state_diff.state_changes.length))){ if(step.state_diff.changed_keys && step.state_diff.changed_keys.length){ var h4 = document.createElement('h4'); h4.textContent = 'Changed keys'; stateDiffEl.appendChild(h4); var ul = document.createElement('ul'); ul.className = 'changed-keys-list'; step.state_diff.changed_keys.forEach(function(k){ var li = document.createElement('li'); li.textContent = k; ul.appendChild(li); }); stateDiffEl.appendChild(ul); } var before = document.createElement('p'); before.textContent = 'Before: ' + (step.state_diff.state_before ? (typeof step.state_diff.state_before === 'string' ? step.state_diff.state_before : JSON.stringify(step.state_diff.state_before)) : '{}'); stateDiffEl.appendChild(before); var after = document.createElement('p'); after.textContent = 'After: ' + (step.state_diff.state_after ? (typeof step.state_diff.state_after === 'string' ? step.state_diff.state_after : JSON.stringify(step.state_diff.state_after)) : '{}'); stateDiffEl.appendChild(after); if(step.state_diff.tool_side_effects && step.state_diff.tool_side_effects.length){ var te = document.createElement('p'); te.textContent = 'Side effects: ' + step.state_diff.tool_side_effects.join('; '); stateDiffEl.appendChild(te); } if(step.state_diff.resource_refs && step.state_diff.resource_refs.length){ var rr = document.createElement('p'); rr.textContent = 'Resources: ' + step.state_diff.resource_refs.join(', '); stateDiffEl.appendChild(rr); } if(step.state_diff.state_changes && step.state_diff.state_changes.length){ var sch = document.createElement('h4'); sch.textContent = 'External state changed (audit)'; stateDiffEl.appendChild(sch); var ul = document.createElement('ul'); ul.className = 'state-changes-list'; step.state_diff.state_changes.forEach(function(c){ var li = document.createElement('li'); li.textContent = (c.resource_type || '') + ' ' + (c.resource_id || '') + ' ' + (c.operation || ''); ul.appendChild(li); }); stateDiffEl.appendChild(ul); } } else { var p = document.createElement('p'); p.className = 'placeholder'; p.textContent = 'State diff (none)'; stateDiffEl.appendChild(p); } } document.getElementById('step-timeline').addEventListener('click', function(ev){ var el = ev.target.closest('.step'); if(el) select(el.getAttribute('data-span-id')); }); document.getElementById('trace-tree').addEventListener('click', function(ev){ var el = ev.target.closest('[data-span-id]'); if(el) select(el.getAttribute('data-span-id')); }); })();")
}

// writeTraceReplayControlScript writes JS for step-level replay query.
func writeTraceReplayControlScript(b *strings.Builder) {
	b.WriteString("(function(){ var T = window.__TRACE__ || {}; var btn = document.getElementById('replay-step-btn'); var out = document.getElementById('replay-step-result'); if(!btn || !out) return; btn.addEventListener('click', function(){ var sel = document.querySelector('.step-timeline .step.selected'); if(!sel){ out.textContent = 'Select a step first.'; return; } var spanId = sel.getAttribute('data-span-id') || ''; if(!spanId){ out.textContent = 'Invalid step id.'; return; } var url = '/api/jobs/' + encodeURIComponent(T.job_id || '') + '/replay?step_node_id=' + encodeURIComponent(spanId); fetch(url).then(function(r){ if(!r.ok){ throw new Error('HTTP ' + r.status); } return r.json(); }).then(function(data){ out.textContent = JSON.stringify(data.step_replay || data, null, 2); }).catch(function(err){ out.textContent = 'Replay query failed: ' + String(err); }); }); })();")
}

// renderTraceTreeHTML renders tree nodes with data-span-id for selection. Nodes with children use <details>/<summary> for folding.
func renderTraceTreeHTML(root *ExecutionNode) string {
	if root == nil {
		return ""
	}
	if root.Type == "job" {
		var out string
		for _, c := range root.Children {
			out += renderTraceTreeHTML(c)
		}
		return out
	}
	label := root.SpanID
	switch root.Type {
	case "plan":
		label = "Plan"
	case "node":
		label = "Node " + root.NodeID
	case "tool":
		label = "Tool " + root.ToolName
	}
	if root.StartTime != nil {
		label += " " + root.StartTime.Format("15:04:05")
	}
	if root.EndTime != nil && root.StartTime != nil {
		label += fmt.Sprintf(" (%dms)", root.EndTime.Sub(*root.StartTime).Milliseconds())
	}
	var b strings.Builder
	b.WriteString("<li data-span-id=\"")
	b.WriteString(html.EscapeString(root.SpanID))
	b.WriteString("\">")
	if len(root.Children) > 0 {
		b.WriteString("<details open><summary><b>")
		b.WriteString(html.EscapeString(label))
		b.WriteString("</b> <code>")
		b.WriteString(html.EscapeString(root.Type))
		b.WriteString("</code></summary><ul>")
		for _, c := range root.Children {
			b.WriteString(renderTraceTreeHTML(c))
		}
		b.WriteString("</ul></details>")
	} else {
		b.WriteString("<b>")
		b.WriteString(html.EscapeString(label))
		b.WriteString("</b> <code>")
		b.WriteString(html.EscapeString(root.Type))
		b.WriteString("</code>")
	}
	b.WriteString("</li>")
	return b.String()
}

// GetObservabilitySummary 返回运维可观测性摘要：队列积压、卡住 Job 列表（2.0）；需 SetObservabilityReader
func (h *Handler) GetObservabilitySummary(ctx context.Context, c *app.RequestContext) {
	if h.observabilityReader == nil {
		c.JSON(consts.StatusOK, map[string]interface{}{
			"queue_backlog":           map[string]int{"default": 0},
			"stuck_job_ids":           []string{},
			"stuck_threshold_seconds": 3600,
		})
		return
	}
	olderThan := 1 * time.Hour
	if s := c.Query("older_than"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			olderThan = d
		}
	}
	pending, err := h.observabilityReader.CountPending(ctx, "")
	if err != nil {
		hlog.CtxErrorf(ctx, "CountPending: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取积压数failed"})
		return
	}
	stuck, err := h.observabilityReader.ListStuckRunningJobIDs(ctx, olderThan)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListStuckRunningJobIDs: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取卡住 Job failed"})
		return
	}
	metrics.QueueBacklog.WithLabelValues("default").Set(float64(pending))
	metrics.StuckJobCount.Set(float64(len(stuck)))
	c.JSON(consts.StatusOK, map[string]interface{}{
		"queue_backlog":           map[string]int{"default": pending},
		"stuck_job_ids":           stuck,
		"stuck_threshold_seconds": int(olderThan.Seconds()),
	})
}

// GetObservabilityStuck 返回卡住 Job 列表（2.0 SRE）；与 summary 中 stuck 字段一致，便于前端/脚本直接消费
func (h *Handler) GetObservabilityStuck(ctx context.Context, c *app.RequestContext) {
	if h.observabilityReader == nil {
		c.JSON(consts.StatusOK, map[string]interface{}{
			"stuck_job_ids":           []string{},
			"stuck_threshold_seconds": 3600,
		})
		return
	}
	olderThan := 1 * time.Hour
	if s := c.Query("older_than"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			olderThan = d
		}
	}
	stuck, err := h.observabilityReader.ListStuckRunningJobIDs(ctx, olderThan)
	if err != nil {
		hlog.CtxErrorf(ctx, "ListStuckRunningJobIDs: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "获取卡住 Job failed"})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"stuck_job_ids":           stuck,
		"stuck_threshold_seconds": int(olderThan.Seconds()),
	})
}

// ListTools 返回所有工具的 Manifest 列表（GET /api/tools）
func (h *Handler) ListTools(ctx context.Context, c *app.RequestContext) {
	if h.toolsRegistry == nil {
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "工具注册表not configured"})
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
		c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "工具注册表not configured"})
		return
	}
	name := c.Param("name")
	m := h.toolsRegistry.Manifest(name)
	if m == nil {
		c.JSON(consts.StatusNotFound, map[string]string{"error": "工具not found"})
		return
	}
	c.JSON(consts.StatusOK, m)
}
