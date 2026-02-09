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
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestHealthCheck(t *testing.T) {
	h := server.Default(server.WithHostPorts(":0"))
	handler := NewHandler(nil, nil)
	h.GET("/api/health", func(ctx context.Context, c *app.RequestContext) {
		handler.HealthCheck(ctx, c)
	})
	w := ut.PerformRequest(h.Engine, "GET", "/api/health", &ut.Body{Body: bytes.NewReader(nil), Len: 0})
	resp := w.Result()
	if resp.StatusCode() != 200 {
		t.Errorf("HealthCheck status: got %d", resp.StatusCode())
	}
	if !bytes.Contains(resp.Body(), []byte("ok")) {
		t.Errorf("HealthCheck body: %s", resp.Body())
	}
}
