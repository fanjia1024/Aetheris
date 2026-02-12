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

package runtime

import "context"

// asyncEventSinkKey is the context key for the async event sink (design/step-contract.md § Async event handling).
type asyncEventSinkKey struct{}

// AsyncEventSink records async events emitted by steps (e.g. "request_sent", "approval_requested").
// Used for trace and replay; idempotency of downstream handlers is the caller's responsibility.
type AsyncEventSink interface {
	Emit(ctx context.Context, name string, payload interface{}) error
}

// WithAsyncEventSink attaches an AsyncEventSink to the context. Steps may call EmitAsyncEvent.
func WithAsyncEventSink(ctx context.Context, sink AsyncEventSink) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, asyncEventSinkKey{}, sink)
}

// EmitAsyncEvent emits an async event if a sink is set on the context. No-op otherwise.
// Steps use this for informational events; side effects must still go through Tools.
func EmitAsyncEvent(ctx context.Context, name string, payload interface{}) error {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(asyncEventSinkKey{})
	if v == nil {
		return nil
	}
	sink, ok := v.(AsyncEventSink)
	if !ok || sink == nil {
		return nil
	}
	return sink.Emit(ctx, name, payload)
}
