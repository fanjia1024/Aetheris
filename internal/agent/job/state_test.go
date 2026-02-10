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

package job

import (
	"testing"
	"time"

	"rag-platform/internal/runtime/jobstore"
)

func TestDeriveStatusFromEvents(t *testing.T) {
	now := time.Now()
	ev := func(typ jobstore.EventType) jobstore.JobEvent {
		return jobstore.JobEvent{JobID: "j1", Type: typ, CreatedAt: now}
	}

	tests := []struct {
		name   string
		events []jobstore.JobEvent
		want   JobStatus
	}{
		{"empty", nil, StatusPending},
		{"empty slice", []jobstore.JobEvent{}, StatusPending},
		{"job_created", []jobstore.JobEvent{ev(jobstore.JobCreated)}, StatusPending},
		{"job_running", []jobstore.JobEvent{ev(jobstore.JobCreated), ev(jobstore.JobRunning)}, StatusRunning},
		{"job_waiting", []jobstore.JobEvent{ev(jobstore.JobRunning), ev(jobstore.JobWaiting)}, StatusWaiting},
		{"job_completed", []jobstore.JobEvent{ev(jobstore.JobRunning), ev(jobstore.JobCompleted)}, StatusCompleted},
		{"job_failed", []jobstore.JobEvent{ev(jobstore.JobRunning), ev(jobstore.JobFailed)}, StatusFailed},
		{"job_cancelled", []jobstore.JobEvent{ev(jobstore.JobRunning), ev(jobstore.JobCancelled)}, StatusCancelled},
		{"job_requeued", []jobstore.JobEvent{ev(jobstore.JobFailed), ev(jobstore.JobRequeued)}, StatusPending},
		{"wait_completed", []jobstore.JobEvent{ev(jobstore.JobWaiting), ev(jobstore.WaitCompleted)}, StatusPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveStatusFromEvents(tt.events)
			if got != tt.want {
				t.Errorf("DeriveStatusFromEvents() = %v, want %v", got, tt.want)
			}
		})
	}
}
