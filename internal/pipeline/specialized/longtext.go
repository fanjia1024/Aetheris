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

package specialized

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/pipeline/ingest"
)

const (
	defaultMaxSegmentChars = 4000
	defaultOverlapChars    = 200
)

// LongTextPipeline 长文本/长 PDF 专用 Pipeline：分段后输出 []*common.Document 供 ingest 入库
type LongTextPipeline struct {
	name            string
	maxSegmentChars int
	overlapChars    int
}

// NewLongTextPipeline 创建长文本 Pipeline
func NewLongTextPipeline() *LongTextPipeline {
	return &LongTextPipeline{
		name:            "longtext",
		maxSegmentChars: defaultMaxSegmentChars,
		overlapChars:    defaultOverlapChars,
	}
}

// SetMaxSegmentChars 设置单段最大字符数
func (p *LongTextPipeline) SetMaxSegmentChars(n int) {
	if n > 0 {
		p.maxSegmentChars = n
	}
}

// SetOverlapChars 设置段间重叠字符数
func (p *LongTextPipeline) SetOverlapChars(n int) {
	if n >= 0 {
		p.overlapChars = n
	}
}

// Name 实现 Pipeline
func (p *LongTextPipeline) Name() string {
	return p.name
}

// Stages 实现 Pipeline
func (p *LongTextPipeline) Stages() []common.PipelineStage {
	return nil
}

// Execute 实现 Pipeline
func (p *LongTextPipeline) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	return p.ProcessSpecialized(input)
}

// AddStage 实现 Pipeline
func (p *LongTextPipeline) AddStage(stage common.PipelineStage) error {
	return nil
}

// RemoveStage 实现 Pipeline
func (p *LongTextPipeline) RemoveStage(name string) error {
	return nil
}

// ProcessSpecialized 实现 SpecializedPipeline
// 输入：*common.Document（用 Content）、string（文件路径，自动识别 PDF 并提取正文）、[]byte（按 PDF 或纯文本处理）
// 输出：[]*common.Document，每段一个 Document，供上层调用 ingest_pipeline 写入向量库
func (p *LongTextPipeline) ProcessSpecialized(input interface{}) (interface{}, error) {
	content, baseMeta, err := p.normalizeInput(input)
	if err != nil {
		return nil, err
	}
	if content == "" {
		return []*common.Document{}, nil
	}

	segments := p.segmentByLengthAndBoundary(content)
	docs := make([]*common.Document, 0, len(segments))
	for i, seg := range segments {
		doc := &common.Document{
			ID:      uuid.New().String(),
			Content: seg,
			Metadata: map[string]interface{}{
				"segment_index":     i + 1,
				"total_segments":    len(segments),
				"longtext_pipeline": p.name,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		for k, v := range baseMeta {
			doc.Metadata[k] = v
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// normalizeInput 将输入规范化为正文字符串与基础元数据
func (p *LongTextPipeline) normalizeInput(input interface{}) (content string, baseMeta map[string]interface{}, err error) {
	baseMeta = make(map[string]interface{})

	switch v := input.(type) {
	case *common.Document:
		if v == nil {
			return "", baseMeta, nil
		}
		content = v.Content
		if v.Metadata != nil {
			for k, val := range v.Metadata {
				baseMeta[k] = val
			}
		}
		return content, baseMeta, nil

	case string:
		// 视为文件路径
		content, err = p.loadTextFromPath(v)
		if err != nil {
			return "", nil, fmt.Errorf("从路径加载: %w", err)
		}
		baseMeta["file_path"] = v
		baseMeta["file_name"] = filepath.Base(v)
		return content, baseMeta, nil

	case []byte:
		if len(v) == 0 {
			return "", baseMeta, nil
		}
		// 简单启发：若含 PDF 魔数则按 PDF 提取
		if isPDF(v) {
			content, err = ingest.ExtractPDFText(v)
			if err != nil {
				return "", nil, fmt.Errorf("PDF 提取: %w", err)
			}
		} else {
			content = string(v)
		}
		return content, baseMeta, nil

	default:
		return "", nil, fmt.Errorf("不支持的输入类型: %T，支持 *common.Document、string（路径）、[]byte", input)
	}
}

func isPDF(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	return string(data[:4]) == "%PDF"
}

// loadTextFromPath 从路径读取正文（PDF 则提取，否则读为文本）
func (p *LongTextPipeline) loadTextFromPath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if isPDF(data) {
		return ingest.ExtractPDFText(data)
	}
	return string(data), nil
}

// segmentByLengthAndBoundary 按长度与句/段边界切分，尽量不在句中断开
func (p *LongTextPipeline) segmentByLengthAndBoundary(text string) []string {
	maxLen := p.maxSegmentChars
	overlap := p.overlapChars
	if maxLen <= 0 {
		maxLen = defaultMaxSegmentChars
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var segments []string
	runes := []rune(text)
	start := 0
	for start < len(runes) {
		end := start + maxLen
		if end > len(runes) {
			end = len(runes)
		} else {
			// 在边界内尽量在句号、换行、段落边界处截断
			end = p.findBreak(runes, start, end)
		}
		seg := strings.TrimSpace(string(runes[start:end]))
		if seg != "" {
			segments = append(segments, seg)
		}
		start = end
		if overlap > 0 && end < len(runes) {
			start -= overlap
			if start < 0 {
				start = 0
			}
		}
	}
	return segments
}

func (p *LongTextPipeline) findBreak(runes []rune, start, end int) int {
	// 从 end 向左找句号、问号、感叹号、换行、双换行
	for i := end - 1; i > start; i-- {
		r := runes[i]
		if r == '\n' {
			if i > start && runes[i-1] == '\n' {
				return i + 1
			}
			return i + 1
		}
		if r == '。' || r == '!' || r == '?' || r == '.' {
			return i + 1
		}
	}
	return end
}
