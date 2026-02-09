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
