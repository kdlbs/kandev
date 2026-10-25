---
id: "04-warn-site-review"
title: "Review shutdown WARN sites; downgrade host-utility 404"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/shutdown-log-noise.md"
parallelism: sequential
---

# Task 04: WARN-site review

## Context

The user asked to review all shutdown noise, including WARN lines. This task
covers the one actionable WARN downgrade and records the deliberate no-ops so
reviewers do not re-litigate them.

Actionable:

- `internal/agent/hostutility/manager.go:244` "failed to delete host utility
  instance" (WARN). During teardown the control client returns
  `instance not found (status 404)` because the instance is already gone. This
  is benign and idempotent.

Deliberate no-ops (leave unchanged, documented in the spec):

- `internal/agent/runtime/lifecycle/manager_events.go:544` "agent updates
  stream disconnected" (WARN, no stack) already correct.
- `internal/agent/runtime/agentctl/launcher/launcher.go:465` relays agentctl
  child **stderr** at WARN by design for visibility; the
  `connection closed component=acp-conn` line is the child's own output.

## Acceptance

- The host-utility delete logs at DEBUG only when the error is a not-found /
  404 (instance already gone); every other delete failure stays WARN.
- The two no-op sites are unchanged.
- No change to cleanup control flow (delete remains best-effort/idempotent).

## Verification

`cd apps/backend && go test ./internal/agent/hostutility/...`

Add a focused regression test asserting DEBUG for a 404/not-found error and
WARN for any other error.

## Files Likely Touched

- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/hostutility/manager_test.go`

## Output Contract

Report the not-found predicate used (sentinel vs string match), tests added and
run, and confirm the no-op sites were reviewed; mark this task done in this file
and `plan.md`.
