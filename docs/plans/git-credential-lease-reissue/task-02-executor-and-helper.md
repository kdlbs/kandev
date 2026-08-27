---
id: "02-executor-and-helper"
title: "Wire helper lease recovery"
status: completed
wave: 2
depends_on: ["01-capability-and-broker"]
plan: "plan.md"
spec: "../../specs/platform/requirements/git-credential-lease-reissue.md"
---

# Task 02: Wire helper lease recovery

- **Acceptance:** Each managed launch scope includes only its matching opaque
  capability in local, Docker, and SSH helper environments.
- **Acceptance:** Git helper and `gh` shim reissue and retry exactly once for
  eligible lease failures, without scope fallback.
- **Verification:** `cd apps/backend && go test -count=1 ./internal/orchestrator/executor ./cmd/agentctl ./internal/agent/runtime/lifecycle`

Likely files: `internal/orchestrator/executor`, `internal/githubauth`,
`internal/agent/runtime/lifecycle`, and `cmd/agentctl`.

Risk: multi-repository scope selection must retain exact path matching.
