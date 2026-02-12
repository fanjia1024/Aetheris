# Aetheris API Contract (v2.0)

## Overview

This document defines the stable public API contract for Aetheris 2.0. APIs marked as **stable** will not introduce breaking changes without a major version bump.

## Versioning Policy

- **Major version** (x.0.0): Breaking changes
- **Minor version** (0.x.0): Backward-compatible features
- **Patch version** (0.0.x): Bug fixes

## Stable APIs (v2.0+)

### Job Management

- `POST /api/agents/:id/message` - Submit job to agent
- `GET /api/jobs/:id` - Get job status
- `POST /api/jobs/:id/stop` - Stop running job
- `POST /api/jobs/:id/signal` - Send signal to parked job
- `GET /api/jobs/:id/events` - Get job event stream
- `GET /api/jobs/:id/trace` - Get job trace
- `POST /api/jobs/:id/export` - Export forensics package

### Agent Management

- `POST /api/agents` - Create agent instance
- `GET /api/agents` - List agents
- `GET /api/agents/:id/state` - Get agent state
- `POST /api/agents/:id/resume` - Resume agent

## Unstable APIs (subject to change)

- Any API not listed above
- Internal packages under `internal/`

## Deprecation Policy

- APIs marked deprecated will be supported for at least 2 minor versions
- Example: Deprecated in v2.1.0 → Removed in v2.3.0

## Breaking Change Notification

Breaking changes will be announced:
1. In release notes
2. In this document (with migration guide)
3. Via deprecation warnings (when possible)
