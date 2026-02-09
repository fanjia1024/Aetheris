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

// Package tracing 提供最小占位，便于后续接入链路追踪（设计 struct.md 4；不依赖 internal）
package tracing

// Span 占位：后续可对接 OpenTelemetry 等
type Span struct{}

// StartSpan 占位：返回 no-op span
func StartSpan(name string) *Span {
	return &Span{}
}

// End 占位
func (s *Span) End() {}

// SetTag 占位
func (s *Span) SetTag(key string, value interface{}) {}
