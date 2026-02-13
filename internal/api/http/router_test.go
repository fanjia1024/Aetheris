package http

import (
	"bytes"
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"rag-platform/internal/api/http/middleware"
)

func buildRouterForTest(forensicsExperimental bool) *server.Hertz {
	h := NewHandler(nil, nil)
	mw := middleware.NewMiddleware()
	r := NewRouter(h, mw)
	r.SetForensicsExperimental(forensicsExperimental)
	return r.Build(":0")
}

func TestRouter_ForensicsRoutesDisabledByDefault(t *testing.T) {
	s := buildRouterForTest(false)

	body := []byte(`{}`)
	w := ut.PerformRequest(s.Engine, "POST", "/api/forensics/query", &ut.Body{Body: bytes.NewReader(body), Len: len(body)})
	if got := w.Result().StatusCode(); got != 404 {
		t.Fatalf("POST /api/forensics/query status = %d, want 404", got)
	}

	w = ut.PerformRequest(s.Engine, "GET", "/api/jobs/job_x/evidence-graph", &ut.Body{Body: bytes.NewReader(nil), Len: 0})
	if got := w.Result().StatusCode(); got != 404 {
		t.Fatalf("GET /api/jobs/:id/evidence-graph status = %d, want 404", got)
	}

	w = ut.PerformRequest(s.Engine, "GET", "/api/jobs/job_x/audit-log", &ut.Body{Body: bytes.NewReader(nil), Len: 0})
	if got := w.Result().StatusCode(); got != 404 {
		t.Fatalf("GET /api/jobs/:id/audit-log status = %d, want 404", got)
	}
}

func TestRouter_ForensicsRoutesEnabled(t *testing.T) {
	s := buildRouterForTest(true)

	body := []byte(`{}`)
	w := ut.PerformRequest(s.Engine, "POST", "/api/forensics/query", &ut.Body{Body: bytes.NewReader(body), Len: len(body)})
	status := w.Result().StatusCode()
	if status == 404 {
		t.Fatalf("POST /api/forensics/query status = %d, want non-404 when experimental enabled", status)
	}

	w = ut.PerformRequest(s.Engine, "GET", "/api/jobs/job_x/evidence-graph", &ut.Body{Body: bytes.NewReader(nil), Len: 0})
	status = w.Result().StatusCode()
	if status == 404 {
		t.Fatalf("GET /api/jobs/:id/evidence-graph status = %d, want non-404 when experimental enabled", status)
	}
}

func TestForensicsConsistencyValidationErrorShape(t *testing.T) {
	h := NewHandler(nil, nil)
	s := server.Default(server.WithHostPorts(":0"))
	s.GET("/test/not-implemented", func(ctx context.Context, c *app.RequestContext) {
		h.ForensicsConsistencyCheck(ctx, c)
	})

	w := ut.PerformRequest(s.Engine, "GET", "/test/not-implemented", &ut.Body{Body: bytes.NewReader(nil), Len: 0})
	if got := w.Result().StatusCode(); got != 400 {
		t.Fatalf("status = %d, want 400", got)
	}

	respBody := w.Result().Body()
	if !bytes.Contains(respBody, []byte(`"error":"job_id is required"`)) {
		t.Fatalf("response body missing validation error field: %s", respBody)
	}
}
