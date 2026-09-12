---
id: "03-openrouter-key-usage"
title: "Add OpenRouter key-usage adapter"
status: planned
wave: 3
depends_on: ["02-profile-proxy-configuration"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
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
