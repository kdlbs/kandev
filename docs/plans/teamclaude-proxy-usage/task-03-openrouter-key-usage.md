---
id: "03-openrouter-key-usage"
title: "Add OpenRouter key-usage adapter"
status: planned
wave: 3
depends_on: ["02-profile-proxy-configuration"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.3
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.4
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 03: Add OpenRouter Key-Usage Adapter

Use an explicitly selected OpenRouter profile and its secret-store credential to
call the documented authenticated key endpoint. Project key usage, remaining
limit, and reset period into Kandev's provider-usage contract. Never expose the
key or user/workspace identifiers in usage DTOs, logs, or cache keys.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agent/usage ./internal/backendapp
```
