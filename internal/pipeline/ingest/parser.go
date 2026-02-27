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
	"strings"

	"rag-platform/internal/pipeline/common"
)

// DocumentParser 文档解析器
type DocumentParser struct {
	name    string
	parsers map[string]Parser
}

// Parser 解析器接口
type Parser interface {
	Parse(content string, metadata map[string]interface{}) (string, error)
	Supports(contentType string) bool
}

// NewDocumentParser 创建新的文档解析器
func NewDocumentParser() *DocumentParser {
	parser := &DocumentParser{
		name:    "document_parser",
		parsers: make(map[string]Parser),
	}

	// 注册内置解析器
	parser.registerParsers()

	return parser
}

// Name 返回组件名称
func (p *DocumentParser) Name() string {
	return p.name
}

// Execute 执行解析操作
func (p *DocumentParser) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := p.Validate(input); err != nil {
		return nil, common.NewPipelineError(p.name, "输入验证failed", err)
	}

	// 解析文档
	doc, ok := input.(*common.Document)
	if !ok {
		return nil, common.NewPipelineError(p.name, "输入类型error", fmt.Errorf("expected *common.Document, got %T", input))
	}

	// 处理文档
	parsedDoc, err := p.ProcessDocument(doc)
	if err != nil {
		return nil, common.NewPipelineError(p.name, "解析文档failed", err)
	}

	return parsedDoc, nil
}

// Validate 验证输入
func (p *DocumentParser) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	if _, ok := input.(*common.Document); !ok {
		return fmt.Errorf("unsupported input输入类型: %T", input)
	}

	return nil
}

// ProcessDocument 处理文档
func (p *DocumentParser) ProcessDocument(doc *common.Document) (*common.Document, error) {
	// 获取内容类型
	contentType, ok := doc.Metadata["content_type"].(string)
	if !ok {
		contentType = "text/plain"
	}

	// 选择合适的解析器
	parser, err := p.selectParser(contentType)
	if err != nil {
		return nil, common.NewPipelineError(p.name, "选择解析器failed", err)
	}

	// 执行解析
	parsedContent, err := parser.Parse(doc.Content, doc.Metadata)
	if err != nil {
		return nil, common.NewPipelineError(p.name, "解析内容failed", err)
	}

	// 更新文档内容
	doc.Content = parsedContent
	doc.Metadata["parsed"] = true
	doc.Metadata["parser"] = p.name

	return doc, nil
}

// registerParsers 注册解析器
func (p *DocumentParser) registerParsers() {
	// 注册文本解析器
	p.parsers["text/plain"] = &TextParser{}
	p.parsers["text/markdown"] = &MarkdownParser{}
	p.parsers["text/html"] = &HTMLParser{}
	p.parsers["application/json"] = &JSONParser{}
	p.parsers["application/pdf"] = &PDFParser{}

	// 注册默认解析器
	p.parsers["default"] = &TextParser{}
}

// selectParser 选择解析器
func (p *DocumentParser) selectParser(contentType string) (Parser, error) {
	// 尝试直接匹配
	if parser, exists := p.parsers[contentType]; exists {
		return parser, nil
	}

	// 尝试匹配子类型
	for ct, parser := range p.parsers {
		if strings.HasPrefix(contentType, strings.Split(ct, "/")[0]+"/") {
			return parser, nil
		}
	}

	// 使用默认解析器
	if parser, exists := p.parsers["default"]; exists {
		return parser, nil
	}

	return nil, fmt.Errorf("找不到合适的解析器: %s", contentType)
}

// AddParser 添加自定义解析器
func (p *DocumentParser) AddParser(contentType string, parser Parser) {
	p.parsers[contentType] = parser
}

// TextParser 文本解析器
type TextParser struct{}

// Parse 解析文本
func (p *TextParser) Parse(content string, metadata map[string]interface{}) (string, error) {
	// 文本解析器只是返回原始内容
	return content, nil
}

// Supports 支持的内容类型
func (p *TextParser) Supports(contentType string) bool {
	return strings.HasPrefix(contentType, "text/")
}

// MarkdownParser Markdown 解析器
type MarkdownParser struct{}

// Parse 解析 Markdown
func (p *MarkdownParser) Parse(content string, metadata map[string]interface{}) (string, error) {
	// 这里可以添加 Markdown 解析逻辑
	// 暂时返回原始内容
	return content, nil
}

// Supports 支持的内容类型
func (p *MarkdownParser) Supports(contentType string) bool {
	return contentType == "text/markdown"
}

// HTMLParser HTML 解析器
type HTMLParser struct{}

// Parse 解析 HTML
func (p *HTMLParser) Parse(content string, metadata map[string]interface{}) (string, error) {
	// 这里可以添加 HTML 解析逻辑
	// 暂时返回原始内容
	return content, nil
}

// Supports 支持的内容类型
func (p *HTMLParser) Supports(contentType string) bool {
	return contentType == "text/html"
}

// JSONParser JSON 解析器
type JSONParser struct{}

// Parse 解析 JSON
func (p *JSONParser) Parse(content string, metadata map[string]interface{}) (string, error) {
	// 这里可以添加 JSON 解析逻辑
	// 暂时返回原始内容
	return content, nil
}

// Supports 支持的内容类型
func (p *JSONParser) Supports(contentType string) bool {
	return contentType == "application/json"
}

// PDFParser PDF 解析器（正文已在 Loader 阶段提取，此处仅透传）
type PDFParser struct{}

// Parse 原样返回已提取的正文
func (p *PDFParser) Parse(content string, metadata map[string]interface{}) (string, error) {
	return content, nil
}

// Supports 支持的内容类型
func (p *PDFParser) Supports(contentType string) bool {
	return contentType == "application/pdf"
}
