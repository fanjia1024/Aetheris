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

package common

import (
	"errors"
	"fmt"
)

// 定义 Pipeline 相关错误
var (
	ErrInvalidInput       = errors.New("无效的输入")
	ErrDocumentNotFound   = errors.New("文档不存在")
	ErrChunkNotFound      = errors.New("切片不存在")
	ErrQueryFailed        = errors.New("查询失败")
	ErrRetrievalFailed    = errors.New("检索失败")
	ErrGenerationFailed   = errors.New("生成失败")
	ErrEmbeddingFailed    = errors.New("向量化失败")
	ErrIndexingFailed     = errors.New("索引失败")
	ErrSplittingFailed    = errors.New("切片失败")
	ErrParsingFailed      = errors.New("解析失败")
	ErrLoadingFailed      = errors.New("加载失败")
	ErrValidationFailed   = errors.New("验证失败")
	ErrTimeout            = errors.New("超时")
	ErrRateLimit          = errors.New("速率限制")
	ErrUnauthorized       = errors.New("未授权")
	ErrForbidden          = errors.New("禁止访问")
	ErrInternal           = errors.New("内部错误")
)

// PipelineError Pipeline 错误结构体
type PipelineError struct {
	Stage   string
	Message string
	Err     error
}

// Error 实现 error 接口
func (e *PipelineError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[Pipeline] %s 阶段错误: %s: %v", e.Stage, e.Message, e.Err)
	}
	return fmt.Sprintf("[Pipeline] %s 阶段错误: %s", e.Stage, e.Message)
}

// Unwrap 实现 errors.Unwrap 接口
func (e *PipelineError) Unwrap() error {
	return e.Err
}

// NewPipelineError 创建新的 Pipeline 错误
func NewPipelineError(stage string, message string, err error) *PipelineError {
	return &PipelineError{
		Stage:   stage,
		Message: message,
		Err:     err,
	}
}

// IsPipelineError 检查是否为 Pipeline 错误
func IsPipelineError(err error) bool {
	var pipelineErr *PipelineError
	return errors.As(err, &pipelineErr)
}

// GetPipelineError 获取 Pipeline 错误
func GetPipelineError(err error) (*PipelineError, bool) {
	var pipelineErr *PipelineError
	if errors.As(err, &pipelineErr) {
		return pipelineErr, true
	}
	return nil, false
}

// ValidationError 验证错误
type ValidationError struct {
	Field   string
	Message string
}

// Error 实现 error 接口
func (e *ValidationError) Error() string {
	return fmt.Sprintf("验证错误: %s: %s", e.Field, e.Message)
}

// NewValidationError 创建新的验证错误
func NewValidationError(field string, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// IsValidationError 检查是否为验证错误
func IsValidationError(err error) bool {
	var valErr *ValidationError
	return errors.As(err, &valErr)
}

// GetValidationError 获取验证错误
func GetValidationError(err error) (*ValidationError, bool) {
	var valErr *ValidationError
	if errors.As(err, &valErr) {
		return valErr, true
	}
	return nil, false
}
