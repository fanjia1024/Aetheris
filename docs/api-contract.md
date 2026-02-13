# Aetheris API Contract (v2.0)

## 1. Scope

This document defines the external API compatibility boundary for Aetheris `2.x`.

- Stable APIs: backward-compatible across `2.x` minors and patches
- Experimental APIs: may change in minor releases
- Internal packages (`internal/`) are out of compatibility scope

## 2. Versioning and Compatibility

- Major (`x.0.0`): may include breaking changes
- Minor (`0.x.0`): backward-compatible feature additions
- Patch (`0.0.x`): bug fixes and non-breaking behavior fixes

Compatibility window:
- Stable APIs are guaranteed across all `2.x`
- Deprecated stable APIs are supported for at least 2 minor versions before removal

## 3. Stable API Surface (2.0)

### Job APIs

- `POST /api/agents/:id/message`
- `GET /api/jobs/:id`
- `POST /api/jobs/:id/stop`
- `POST /api/jobs/:id/signal`
- `GET /api/jobs/:id/events`
- `GET /api/jobs/:id/trace`
- `POST /api/jobs/:id/export`

### Agent APIs

- `POST /api/agents`
- `GET /api/agents`
- `GET /api/agents/:id/state`
- `POST /api/agents/:id/resume`

### Observability Pages

- `GET /api/jobs/:id/trace/page`
- `GET /api/trace/overview/page`

## 4. Experimental Surface

Experimental APIs may change without major bump, but should be noted in release notes:

- New endpoints not listed in Section 3
- Optional response fields marked experimental in docs/release notes
- Adapter-specific runtime internals

## 5. Request/Response Change Policy

For stable endpoints:
- Allowed:
  - Add optional request fields
  - Add optional response fields
  - Add new non-default query parameters
- Not allowed in `2.x`:
  - Remove required fields
  - Rename existing fields
  - Change field types incompatibly
  - Change endpoint semantics incompatibly

## 6. Deprecation Policy

When deprecating a stable API:
1. Mark as deprecated in docs
2. Add migration path in release notes
3. Keep API available for >= 2 minor versions
4. Remove only in next major, or after window with explicit notice

Example:
- Deprecated in `v2.2.0`
- Earliest removal target: `v2.4.0` (or `v3.0.0`)

## 7. Release Gates for Contract Safety (P0)

A `2.x` release should not be published unless:

- Contract docs are updated (`docs/api-contract.md`)
- Compatibility checks pass for stable endpoints (smoke + regression tests)
- Deprecations (if any) include migration guidance
- Release notes include API delta summary

## 8. References

- `docs/release-checklist-2.0.md`
- `docs/upgrade-1.x-to-2.0.md`
- `docs/runtime-guarantees.md`
