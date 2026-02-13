// Copyright 2026 fanjia1024
// Sharded JobStore implementation for horizontal scaling

package jobstore

import (
	"context"
	"hash/fnv"
)

// ShardedStore 分片 JobStore，支持水平扩展
type ShardedStore struct {
	shards []JobStore
	count  int
}

// NewShardedStore 创建分片存储
func NewShardedStore(shards []JobStore) *ShardedStore {
	return &ShardedStore{
		shards: shards,
		count:  len(shards),
	}
}

// getShard 根据 jobID 计算分片
func (s *ShardedStore) getShard(jobID string) JobStore {
	h := fnv.New32a()
	h.Write([]byte(jobID))
	idx := int(h.Sum32()) % s.count
	return s.shards[idx]
}

// ListEvents 实现 JobStore 接口
func (s *ShardedStore) ListEvents(ctx context.Context, jobID string) ([]JobEvent, int, error) {
	return s.getShard(jobID).ListEvents(ctx, jobID)
}

// Append 实现 JobStore 接口
func (s *ShardedStore) Append(ctx context.Context, jobID string, expectedVersion int, event JobEvent) (int, error) {
	return s.getShard(jobID).Append(ctx, jobID, expectedVersion, event)
}

// Claim 跨分片 claim（轮询所有分片）
func (s *ShardedStore) Claim(ctx context.Context, workerID string) (string, int, string, error) {
	for _, shard := range s.shards {
		jobID, version, attemptID, err := shard.Claim(ctx, workerID)
		if err == ErrNoJob {
			continue
		}
		if err != nil {
			return "", 0, "", err
		}
		return jobID, version, attemptID, nil
	}
	return "", 0, "", ErrNoJob
}

// ClaimJob 指定 job claim
func (s *ShardedStore) ClaimJob(ctx context.Context, workerID string, jobID string) (int, string, error) {
	return s.getShard(jobID).ClaimJob(ctx, workerID, jobID)
}

// Heartbeat 实现 JobStore 接口
func (s *ShardedStore) Heartbeat(ctx context.Context, workerID string, jobID string) error {
	return s.getShard(jobID).Heartbeat(ctx, workerID, jobID)
}

// Watch 实现 JobStore 接口
func (s *ShardedStore) Watch(ctx context.Context, jobID string) (<-chan JobEvent, error) {
	return s.getShard(jobID).Watch(ctx, jobID)
}

// ListJobIDsWithExpiredClaim 跨分片查询过期 lease
func (s *ShardedStore) ListJobIDsWithExpiredClaim(ctx context.Context) ([]string, error) {
	var allJobIDs []string
	for _, shard := range s.shards {
		jobIDs, err := shard.ListJobIDsWithExpiredClaim(ctx)
		if err != nil {
			return nil, err
		}
		allJobIDs = append(allJobIDs, jobIDs...)
	}
	return allJobIDs, nil
}

// GetCurrentAttemptID 实现 JobStore 接口
func (s *ShardedStore) GetCurrentAttemptID(ctx context.Context, jobID string) (string, error) {
	return s.getShard(jobID).GetCurrentAttemptID(ctx, jobID)
}

// CreateSnapshot 实现快照接口
func (s *ShardedStore) CreateSnapshot(ctx context.Context, jobID string, upToVersion int, snapshot []byte) error {
	return s.getShard(jobID).CreateSnapshot(ctx, jobID, upToVersion, snapshot)
}

// GetLatestSnapshot 实现快照接口
func (s *ShardedStore) GetLatestSnapshot(ctx context.Context, jobID string) (*JobSnapshot, error) {
	return s.getShard(jobID).GetLatestSnapshot(ctx, jobID)
}

// DeleteSnapshotsBefore 实现快照接口
func (s *ShardedStore) DeleteSnapshotsBefore(ctx context.Context, jobID string, beforeVersion int) error {
	return s.getShard(jobID).DeleteSnapshotsBefore(ctx, jobID, beforeVersion)
}
