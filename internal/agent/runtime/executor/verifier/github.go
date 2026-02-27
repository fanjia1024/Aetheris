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

package verifier

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"rag-platform/internal/agent/runtime/executor"
)

// GitHubVerifier 对 resource_type=github_issue 等做 Confirmation Replay：请求 external_ref 或构造 API URL 校验资源仍存在
type GitHubVerifier struct {
	// Token 可选；用于 GitHub API 认证，空则匿名（易受限流）
	Token string
	// BaseURL 可选；默认 https://api.github.com
	BaseURL string
	// Client 可选；不设则用默认 http.Client
	Client *http.Client
}

// NewGitHubVerifier 创建 GitHub ResourceVerifier；token 可为空（匿名）
func NewGitHubVerifier(token string) *GitHubVerifier {
	return &GitHubVerifier{Token: token, BaseURL: "https://api.github.com"}
}

// Verify 实现 executor.ResourceVerifier：仅处理 resource_type=github_issue；用 external_ref 作 API URL 做 GET，200 则 ok
func (g *GitHubVerifier) Verify(ctx context.Context, jobID, stepID, resourceType, resourceID, operation, externalRef string) (ok bool, err error) {
	if resourceType != "github_issue" && resourceType != "github_pr" {
		// 非 GitHub 资源，不校验（交给其他 verifier 或跳过）
		return true, nil
	}
	url := strings.TrimSpace(externalRef)
	if url == "" {
		// 无 external_ref 时可根据 resource_id 构造；简单实现要求 external_ref 必填
		return false, fmt.Errorf("verifier/github: resource_type=%s requires external_ref（API URL）", resourceType)
	}
	// 若非完整 URL，可拼接 BaseURL；这里假定 external_ref 为完整 URL 或 path
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		base := strings.TrimSuffix(g.BaseURL, "/")
		url = base + "/" + strings.TrimPrefix(url, "/")
	}
	req := resty.New()
	if g.Client != nil {
		req.SetTransport(g.Client.Transport)
	}
	req.SetTimeout(10 * time.Second)
	r := req.R().SetContext(ctx)
	if g.Token != "" {
		r.SetHeader("Authorization", "Bearer "+g.Token)
	}
	resp, err := r.Get(url)
	if err != nil {
		return false, fmt.Errorf("verifier/github: GET %s: %w", url, err)
	}
	if resp.StatusCode() == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode() == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("verifier/github: GET %s 返回 %d", url, resp.StatusCode())
}

// Ensure GitHubVerifier implements executor.ResourceVerifier
var _ executor.ResourceVerifier = (*GitHubVerifier)(nil)
