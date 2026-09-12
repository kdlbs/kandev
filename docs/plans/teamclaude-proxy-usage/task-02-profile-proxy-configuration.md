---
id: "02-profile-proxy-configuration"
title: "Persist profile proxy configuration"
status: planned
wave: 2
depends_on: ["01-read-teamclaude-pool-utilization"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.3
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.4
system_design: []
---

# Task 02: Persist Profile Proxy Configuration

## Summary

Replace temporary environment-variable selection with a structured agent-profile
proxy configuration. It explicitly names the gateway vendor and endpoint,
selects native/configured/disabled usage mode, and references an optional
stored credential without serializing its plaintext into profile DTOs.

## Acceptance

- A profile explicitly selects `teamclaude`, `openrouter`, `litellm`, or no
  gateway; endpoint inference never selects a vendor.
- The configuration persists through SQLite CRUD and is visible in profile
  settings, while credential values remain in the secret store.
- Resolver selection is based only on the persisted vendor configuration.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agent/settings/... ./internal/backendapp
```
