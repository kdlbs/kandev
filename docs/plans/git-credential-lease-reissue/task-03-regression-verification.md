---
id: "03-regression-verification"
title: "Verify lease recovery contracts"
status: completed
wave: 3
depends_on: ["01-capability-and-broker", "02-executor-and-helper"]
plan: "plan.md"
spec: "../../specs/platform/requirements/git-credential-lease-reissue.md"
---

# Task 03: Verify lease recovery contracts

- **Acceptance:** Rotation, backend-restart-equivalent, forged capability, and
  expired capability scenarios have focused regression coverage.
- **Acceptance:** Formatting, diff checks, targeted tests, build, vet, and the
  changed-file linter are recorded accurately.
- **Verification:** `cd apps/backend && go build ./... && go vet ./internal/gitcredentials ./internal/github ./internal/orchestrator/executor ./cmd/agentctl`

Likely files: the tests added by tasks 01 and 02 plus this plan's result
sections.

Risk: do not claim E2E or remote runtime evidence from local unit tests.
