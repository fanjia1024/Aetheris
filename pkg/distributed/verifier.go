// Copyright 2026 fanjia1024
// Distributed verifier for multi-org validation (3.0-M4)

package distributed

import (
	"context"
	"fmt"
)

// DistributedVerifier 分布式验证器
type DistributedVerifier struct {
	source OrgEventSource
}

// OrgEventSource 组织侧事件拉取接口。
type OrgEventSource interface {
	PullOrgEvents(ctx context.Context, orgID string, jobID string) ([]Event, error)
}

type protocolEventSource struct {
	protocol SyncProtocol
}

func (p *protocolEventSource) PullOrgEvents(ctx context.Context, orgID string, jobID string) ([]Event, error) {
	return p.protocol.Pull(ctx, orgID, jobID)
}

// NewDistributedVerifier 创建分布式验证器
func NewDistributedVerifier() *DistributedVerifier {
	return &DistributedVerifier{}
}

// WithSyncProtocol 使用同步协议作为事件来源。
func (v *DistributedVerifier) WithSyncProtocol(protocol SyncProtocol) *DistributedVerifier {
	if protocol == nil {
		v.source = nil
		return v
	}
	v.source = &protocolEventSource{protocol: protocol}
	return v
}

// WithEventSource 设置自定义事件来源。
func (v *DistributedVerifier) WithEventSource(source OrgEventSource) *DistributedVerifier {
	v.source = source
	return v
}

// VerifyAcrossOrgs 跨组织验证证据链
func (v *DistributedVerifier) VerifyAcrossOrgs(ctx context.Context, jobID string, orgs []string) (*MultiOrgVerifyResult, error) {
	result := &MultiOrgVerifyResult{
		JobID:         jobID,
		Organizations: append([]string(nil), orgs...),
		Consensus:     true,
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if len(orgs) == 0 || v.source == nil {
		return result, nil
	}

	rootByOrg := make(map[string]string, len(orgs))
	for _, orgID := range orgs {
		events, err := v.source.PullOrgEvents(ctx, orgID, jobID)
		if err != nil {
			result.Consensus = false
			result.Divergences = append(result.Divergences, fmt.Sprintf("%s: pull failed: %v", orgID, err))
			continue
		}
		if len(events) == 0 {
			result.Consensus = false
			result.Divergences = append(result.Divergences, fmt.Sprintf("%s: empty event stream", orgID))
			continue
		}
		lastHash := events[len(events)-1].Hash
		if lastHash == "" {
			result.Consensus = false
			result.Divergences = append(result.Divergences, fmt.Sprintf("%s: missing last event hash", orgID))
			continue
		}
		rootByOrg[orgID] = lastHash
	}

	var expected string
	for _, orgID := range orgs {
		root := rootByOrg[orgID]
		if root == "" {
			continue
		}
		if expected == "" {
			expected = root
			continue
		}
		if root != expected {
			result.Consensus = false
			result.Divergences = append(result.Divergences, fmt.Sprintf("%s: root hash mismatch", orgID))
		}
	}

	return result, nil
}

// MultiOrgVerifyResult 多方验证结果
type MultiOrgVerifyResult struct {
	JobID         string   `json:"job_id"`
	Organizations []string `json:"organizations"`
	Consensus     bool     `json:"consensus"`
	Divergences   []string `json:"divergences,omitempty"`
}
