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
