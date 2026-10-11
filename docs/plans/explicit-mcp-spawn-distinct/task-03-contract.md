---
id: "03-contract"
title: "Align spawn profile and identity contracts"
status: done
depends_on: ["02-launch-intent"]
plan: "plan.md"
---

# Task 03: Align spawn profile and identity contracts

MAIN owns spawn_session_test.go, mcp/server/server.go, spawn_session_tool_test.go, existing spawn-session-effective-profile requirement and docs/public/agent-communication.md. Sequential.
Replace superseded spawn workflow-pin expectations, preserve actual launch response projection, explain profile inheritance and distinct session identity. Do not alter create_task or ordinary workflow semantics. Public docs page classification: Explanation.

Verification: `cd apps/backend && go test ./internal/mcp/server -run TestSpawnSessionToolDescribesEffectiveAgentProfile -count=1`; `node --test scripts/validate-public-docs.test.mjs`; `node scripts/validate-public-docs.mjs`; `git diff --check`.

## Results

Tool contract red confirmed against old description. Focused handler/server run passes, including tool description contract. `node --test scripts/validate-public-docs.test.mjs` passes 63/63; `node scripts/validate-public-docs.mjs` validates 47 pages; spec/catalog checks pass; `git diff --check` passes. Independent Review and QA follow committed candidate; only MAIN fixes and routes.
