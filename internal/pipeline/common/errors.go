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

// 定义 Pipeline 相关error
var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrDocumentNotFound = errors.New("document not found")
	ErrChunkNotFound    = errors.New("chunk not found")
	ErrQueryFailed      = errors.New("query failed")
	ErrRetrievalFailed  = errors.New("retrieval failed")
	ErrGenerationFailed = errors.New("generation failed")
	ErrEmbeddingFailed  = errors.New("embedding failed")
	ErrIndexingFailed   = errors.New("indexing failed")
	ErrSplittingFailed  = errors.New("splitting failed")
	ErrParsingFailed    = errors.New("parsing failed")
	ErrLoadingFailed    = errors.New("loading failed")
	ErrValidationFailed = errors.New("validation failed")
	ErrTimeout          = errors.New("timeout")
	ErrRateLimit        = errors.New("rate limit")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrInternal         = errors.New("internal error")
)

// PipelineError Pipeline error结构体
type PipelineError struct {
	Stage   string
	Message string
	Err     error
}

// Error 实现 error 接口
func (e *PipelineError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[Pipeline] %s stage error: %s: %v", e.Stage, e.Message, e.Err)
	}
	return fmt.Sprintf("[Pipeline] %s stage error: %s", e.Stage, e.Message)
}

// Unwrap 实现 errors.Unwrap 接口
func (e *PipelineError) Unwrap() error {
	return e.Err
}

// NewPipelineError 创建新的 Pipeline error
func NewPipelineError(stage string, message string, err error) *PipelineError {
	return &PipelineError{
		Stage:   stage,
		Message: message,
		Err:     err,
	}
}

// IsPipelineError 检查是否为 Pipeline error
func IsPipelineError(err error) bool {
	var pipelineErr *PipelineError
	return errors.As(err, &pipelineErr)
}

// GetPipelineError 获取 Pipeline error
func GetPipelineError(err error) (*PipelineError, bool) {
	var pipelineErr *PipelineError
	if errors.As(err, &pipelineErr) {
		return pipelineErr, true
	}
	return nil, false
}

// ValidationError 验证error
type ValidationError struct {
	Field   string
	Message string
}

// Error 实现 error 接口
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s: %s", e.Field, e.Message)
}

// NewValidationError 创建新的验证error
func NewValidationError(field string, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// IsValidationError 检查是否为验证error
func IsValidationError(err error) bool {
	var valErr *ValidationError
	return errors.As(err, &valErr)
}

// GetValidationError 获取验证error
func GetValidationError(err error) (*ValidationError, bool) {
	var valErr *ValidationError
	if errors.As(err, &valErr) {
		return valErr, true
	}
	return nil, false
}
