---
id: "04-litellm-usage"
title: "Add LiteLLM configured usage adapter"
status: planned
wave: 3
depends_on: ["02-profile-proxy-configuration"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 04: Add LiteLLM Configured Usage Adapter

Use an explicitly selected LiteLLM profile, configured admin/spend endpoint,
and secret-store credential. Validate the deployment-specific response and map
only bounded spend/budget information into the provider-usage contract.
Missing admin access remains unavailable rather than being guessed from normal
OpenAI-compatible inference responses.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agent/usage ./internal/backendapp
```
