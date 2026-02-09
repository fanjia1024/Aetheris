package job

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := NewJobStoreMem()
	id, _ := store.Create(ctx, &Job{AgentID: "a1", Goal: "g"})
	var runCount int32
	runJob := func(_ context.Context, j *Job) error {
		atomic.AddInt32(&runCount, 1)
		return nil
	}
	sched := NewScheduler(store, runJob, SchedulerConfig{
		MaxConcurrency: 1,
		RetryMax:       0,
		Backoff:        10 * time.Millisecond,
	})
	sched.Start(ctx)
	defer sched.Stop()
	// 等待被拉取并执行完成
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		j, _ := store.Get(ctx, id)
		if j != nil && j.Status == StatusCompleted {
			break
		}
	}
	j, _ := store.Get(ctx, id)
	if j == nil || j.Status != StatusCompleted {
		t.Errorf("expected job completed, got %+v", j)
	}
	if atomic.LoadInt32(&runCount) != 1 {
		t.Errorf("expected runCount 1, got %d", runCount)
	}
}

func TestScheduler_RetryThenSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := NewJobStoreMem()
	id, _ := store.Create(ctx, &Job{AgentID: "a1", Goal: "g"})
	var runCount int32
	runJob := func(_ context.Context, j *Job) error {
		c := atomic.AddInt32(&runCount, 1)
		if c == 1 {
			return context.DeadlineExceeded // 第一次失败
		}
		return nil
	}
	sched := NewScheduler(store, runJob, SchedulerConfig{
		MaxConcurrency: 1,
		RetryMax:       2,
		Backoff:        20 * time.Millisecond,
	})
	sched.Start(ctx)
	defer sched.Stop()
	for i := 0; i < 80; i++ {
		time.Sleep(50 * time.Millisecond)
		j, _ := store.Get(ctx, id)
		if j != nil && j.Status == StatusCompleted {
			break
		}
	}
	j, _ := store.Get(ctx, id)
	if j == nil || j.Status != StatusCompleted {
		t.Errorf("expected job completed after retry, got %+v", j)
	}
	if atomic.LoadInt32(&runCount) != 2 {
		t.Errorf("expected runCount 2 (fail then success), got %d", runCount)
	}
}

func TestScheduler_MaxConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := NewJobStoreMem()
	_, _ = store.Create(ctx, &Job{AgentID: "a1", Goal: "g1"})
	_, _ = store.Create(ctx, &Job{AgentID: "a1", Goal: "g2"})
	var concurrent int32
	var maxSeen int32
	runJob := func(_ context.Context, j *Job) error {
		atomic.AddInt32(&concurrent, 1)
		defer atomic.AddInt32(&concurrent, -1)
		cur := atomic.LoadInt32(&concurrent)
		if cur > atomic.LoadInt32(&maxSeen) {
			atomic.StoreInt32(&maxSeen, cur)
		}
		time.Sleep(30 * time.Millisecond)
		return nil
	}
	sched := NewScheduler(store, runJob, SchedulerConfig{
		MaxConcurrency: 2,
		RetryMax:       0,
		Backoff:        time.Millisecond,
	})
	sched.Start(ctx)
	defer sched.Stop()
	time.Sleep(300 * time.Millisecond)
	if atomic.LoadInt32(&maxSeen) > 2 {
		t.Errorf("expected max concurrency 2, saw %d", maxSeen)
	}
}
