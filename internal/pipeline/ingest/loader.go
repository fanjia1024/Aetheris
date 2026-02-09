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

package ingest

import (
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"rag-platform/internal/pipeline/common"
)

// DocumentLoader 文档加载器
type DocumentLoader struct {
	name        string
	supportedTypes []string
	maxSize     int64
}

// NewDocumentLoader 创建新的文档加载器
func NewDocumentLoader() *DocumentLoader {
	return &DocumentLoader{
		name: "document_loader",
		supportedTypes: []string{
			"text/plain",
			"text/markdown",
			"text/html",
			"application/pdf",
			"application/json",
			"application/msword",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		maxSize: 100 * 1024 * 1024, // 100MB
	}
}

// Name 返回组件名称
func (l *DocumentLoader) Name() string {
	return l.name
}

// Execute 执行加载操作
func (l *DocumentLoader) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := l.Validate(input); err != nil {
		return nil, common.NewPipelineError(l.name, "输入验证失败", err)
	}

	// 根据输入类型执行加载
	switch v := input.(type) {
	case string:
		// 文件路径
		return l.loadFromPath(ctx, v)
	case *multipart.FileHeader:
		// 上传文件
		return l.loadFromFile(ctx, v)
	case []byte:
		// 字节数据
		return l.loadFromBytes(ctx, v)
	default:
		return nil, common.NewPipelineError(l.name, "不支持的输入类型", fmt.Errorf("expected string, *multipart.FileHeader, or []byte, got %T", input))
	}
}

// Validate 验证输入
func (l *DocumentLoader) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	switch v := input.(type) {
	case string:
		// 检查文件是否存在
		if _, err := os.Stat(v); os.IsNotExist(err) {
			return fmt.Errorf("文件不存在: %s", v)
		}
	case *multipart.FileHeader:
		// 检查文件大小
		if v.Size > l.maxSize {
			return fmt.Errorf("文件大小超过限制: %d > %d", v.Size, l.maxSize)
		}
	case []byte:
		// 检查字节大小
		if int64(len(v)) > l.maxSize {
			return fmt.Errorf("数据大小超过限制: %d > %d", len(v), l.maxSize)
		}
	default:
		return fmt.Errorf("不支持的输入类型: %T", input)
	}

	return nil
}

// ProcessDocument 处理文档
func (l *DocumentLoader) ProcessDocument(doc *common.Document) (*common.Document, error) {
	// 这里可以添加文档预处理逻辑
	return doc, nil
}

// loadFromPath 从文件路径加载
func (l *DocumentLoader) loadFromPath(ctx *common.PipelineContext, path string) (*common.Document, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, common.NewPipelineError(l.name, "读取文件失败", err)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, common.NewPipelineError(l.name, "获取文件信息失败", err)
	}

	contentType := l.getContentType(path)
	docContent := string(content)
	if contentType == "application/pdf" {
		extracted, err := extractPDFText(content)
		if err != nil {
			return nil, common.NewPipelineError(l.name, "PDF 文本提取失败", err)
		}
		docContent = extracted
	}

	doc := &common.Document{
		ID:      uuid.New().String(),
		Content: docContent,
		Metadata: map[string]interface{}{
			"file_path":    path,
			"file_name":    filepath.Base(path),
			"file_size":    fileInfo.Size(),
			"content_type": contentType,
			"created_at":   fileInfo.ModTime(),
			"loader":       l.name,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx.Metadata["document_id"] = doc.ID
	ctx.Metadata["file_name"] = filepath.Base(path)

	return doc, nil
}

// loadFromFile 从上传文件加载
func (l *DocumentLoader) loadFromFile(ctx *common.PipelineContext, fileHeader *multipart.FileHeader) (*common.Document, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, common.NewPipelineError(l.name, "打开文件失败", err)
	}
	defer file.Close()

	content, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, common.NewPipelineError(l.name, "读取文件失败", err)
	}

	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = l.getContentType(fileHeader.Filename)
	}
	docContent := string(content)
	if contentType == "application/pdf" || strings.ToLower(filepath.Ext(fileHeader.Filename)) == ".pdf" {
		extracted, err := extractPDFText(content)
		if err != nil {
			return nil, common.NewPipelineError(l.name, "PDF 文本提取失败", err)
		}
		docContent = extracted
	}

	doc := &common.Document{
		ID:      uuid.New().String(),
		Content: docContent,
		Metadata: map[string]interface{}{
			"file_name":    fileHeader.Filename,
			"file_size":    fileHeader.Size,
			"content_type": contentType,
			"loader":       l.name,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx.Metadata["document_id"] = doc.ID
	ctx.Metadata["file_name"] = fileHeader.Filename

	return doc, nil
}

// loadFromBytes 从字节数据加载
func (l *DocumentLoader) loadFromBytes(ctx *common.PipelineContext, data []byte) (*common.Document, error) {
	// 创建文档
	doc := &common.Document{
		ID:        uuid.New().String(),
		Content:   string(data),
		Metadata: map[string]interface{}{
			"file_size":    len(data),
			"content_type": "text/plain",
			"loader":       l.name,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx.Metadata["document_id"] = doc.ID

	return doc, nil
}

// getContentType 获取文件内容类型
func (l *DocumentLoader) getContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt":
		return "text/plain"
	case ".md", ".markdown":
		return "text/markdown"
	case ".html", ".htm":
		return "text/html"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}
