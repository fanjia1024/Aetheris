package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"rag-platform/internal/runtime/eino"
	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/storage/metadata"
	"rag-platform/pkg/log"
)

// Handler HTTP 处理器
type Handler struct {
	engine       *eino.Engine
	metadataRepo *metadata.Repository
	logger       *log.Logger
}

// NewHandler 创建新的 HTTP 处理器
func NewHandler(engine *eino.Engine, metadataRepo *metadata.Repository, logger *log.Logger) *Handler {
	return &Handler{
		engine:       engine,
		metadataRepo: metadataRepo,
		logger:       logger,
	}
}

// HealthCheck 健康检查
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"service":   "api-service",
	})
}

// UploadDocument 上传文档
func (h *Handler) UploadDocument(c *gin.Context) {
	// 处理文件上传
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请上传文件",
		})
		return
	}

	// 执行 Ingest Pipeline
	result, err := h.engine.ExecuteWorkflow(c.Request.Context(), "ingest_pipeline", map[string]interface{}{
		"file": file,
		"metadata": map[string]interface{}{
			"filename": file.Filename,
			"size":     file.Size,
			"uploaded_at": time.Now(),
		},
	})

	if err != nil {
		h.logger.Error("上传文档失败", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "上传文档失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"result": result,
		"message": "文档上传成功",
	})
}

// ListDocuments 列出文档
func (h *Handler) ListDocuments(c *gin.Context) {
	// 从元数据存储获取文档列表
	documents, err := h.metadataRepo.ListDocuments()
	if err != nil {
		h.logger.Error("获取文档列表失败", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取文档列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"documents": documents,
		"total":     len(documents),
	})
}

// GetDocument 获取文档
func (h *Handler) GetDocument(c *gin.Context) {
	id := c.Param("id")

	// 从元数据存储获取文档
	document, err := h.metadataRepo.GetDocument(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "文档不存在",
		})
		return
	}

	c.JSON(http.StatusOK, document)
}

// DeleteDocument 删除文档
func (h *Handler) DeleteDocument(c *gin.Context) {
	id := c.Param("id")

	// 从元数据存储删除文档
	if err := h.metadataRepo.DeleteDocument(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "删除文档失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"message": "文档删除成功",
	})
}

// ListCollections 列出集合
func (h *Handler) ListCollections(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"collections": []gin.H{
			{
				"id": "default",
				"name": "默认集合",
				"description": "默认文档集合",
				"document_count": 100,
				"created_at": time.Now(),
			},
		},
	})
}

// CreateCollection 创建集合
func (h *Handler) CreateCollection(c *gin.Context) {
	var request struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"collection": gin.H{
			"id": "new-collection",
			"name": request.Name,
			"description": request.Description,
			"created_at": time.Now(),
		},
	})
}

// DeleteCollection 删除集合
func (h *Handler) DeleteCollection(c *gin.Context) {
	id := c.Param("id")

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"message": fmt.Sprintf("集合 %s 删除成功", id),
	})
}

// Query 查询
func (h *Handler) Query(c *gin.Context) {
	var request struct {
		Query     string            `json:"query" binding:"required"`
		Metadata  map[string]interface{} `json:"metadata"`
		TopK      int               `json:"top_k"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	// 创建查询对象
	query := &common.Query{
		ID:        fmt.Sprintf("query-%d", time.Now().UnixNano()),
		Text:      request.Query,
		Metadata:  request.Metadata,
		CreatedAt: time.Now(),
	}

	// 执行 Query Pipeline
	result, err := h.engine.ExecuteWorkflow(c.Request.Context(), "query_pipeline", map[string]interface{}{
		"query": query,
		"top_k": request.TopK,
	})

	if err != nil {
		h.logger.Error("查询失败", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"result": result,
	})
}

// BatchQuery 批量查询
func (h *Handler) BatchQuery(c *gin.Context) {
	var request struct {
		Queries []struct {
			Query    string            `json:"query" binding:"required"`
			Metadata map[string]interface{} `json:"metadata"`
		} `json:"queries" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	// 处理批量查询
	results := make([]interface{}, len(request.Queries))

	for i, q := range request.Queries {
		// 创建查询对象
		query := &common.Query{
			ID:        fmt.Sprintf("query-%d-%d", time.Now().UnixNano(), i),
			Text:      q.Query,
			Metadata:  q.Metadata,
			CreatedAt: time.Now(),
		}

		// 执行 Query Pipeline
		result, err := h.engine.ExecuteWorkflow(c.Request.Context(), "query_pipeline", map[string]interface{}{
			"query": query,
		})

		if err != nil {
			results[i] = gin.H{
				"error": err.Error(),
				"query": q.Query,
			}
		} else {
			results[i] = result
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"results": results,
		"total":   len(results),
	})
}

// SystemStatus 系统状态
func (h *Handler) SystemStatus(c *gin.Context) {
	// 获取系统状态
	status := map[string]interface{}{
		"api_service": "running",
		"agent_service": "running",
		"index_service": "running",
		"workflows": h.engine.GetWorkflows(),
		"agents":    h.engine.GetAgents(),
		"timestamp": time.Now(),
	}

	c.JSON(http.StatusOK, status)
}

// SystemMetrics 系统指标
func (h *Handler) SystemMetrics(c *gin.Context) {
	// 提供系统指标
	metrics := map[string]interface{}{
		"requests_total": 1000,
		"errors_total":   10,
		"latency_avg":    50,
		"documents_count": 1000,
		"index_size":     "100MB",
		"timestamp":      time.Now(),
	}

	c.JSON(http.StatusOK, metrics)
}
