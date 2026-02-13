package distributed

import (
	"context"
	"errors"
	"testing"
)

type fakeOrgEventSource struct {
	events map[string][]Event
	errs   map[string]error
}

func (f *fakeOrgEventSource) PullOrgEvents(ctx context.Context, orgID string, jobID string) ([]Event, error) {
	if err, ok := f.errs[orgID]; ok {
		return nil, err
	}
	return append([]Event(nil), f.events[orgID]...), nil
}

func TestVerifyAcrossOrgs_Consensus(t *testing.T) {
	v := NewDistributedVerifier().WithEventSource(&fakeOrgEventSource{
		events: map[string][]Event{
			"org_a": {{Hash: "h1"}, {Hash: "root"}},
			"org_b": {{Hash: "h1"}, {Hash: "root"}},
		},
	})
	res, err := v.VerifyAcrossOrgs(context.Background(), "job_1", []string{"org_a", "org_b"})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !res.Consensus {
		t.Fatalf("expected consensus=true, divergences=%v", res.Divergences)
	}
}

func TestVerifyAcrossOrgs_Divergence(t *testing.T) {
	v := NewDistributedVerifier().WithEventSource(&fakeOrgEventSource{
		events: map[string][]Event{
			"org_a": {{Hash: "root_a"}},
			"org_b": {{Hash: "root_b"}},
		},
		errs: map[string]error{
			"org_c": errors.New("timeout"),
		},
	})
	res, err := v.VerifyAcrossOrgs(context.Background(), "job_2", []string{"org_a", "org_b", "org_c"})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if res.Consensus {
		t.Fatal("expected consensus=false")
	}
	if len(res.Divergences) == 0 {
		t.Fatal("expected divergences")
	}
}
