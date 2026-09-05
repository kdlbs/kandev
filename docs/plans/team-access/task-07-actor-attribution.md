---
id: "07-actor-attribution"
title: "Actor attribution and audit"
status: todo
wave: 3
depends_on: ["04-management-api"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/human-assignee.md"
---

# Task 07: Actor Attribution and Audit

This is the layer a shared login cannot provide, and the reason #2824 is not
solved by "just share the password".

## Acceptance

- `task_session_messages.author_user_id`, `queued_messages.author_user_id`, and
  `task_step_transitions.actor_user_id` are added via idempotent migrations and
  populated from the request identity.
- Attribution survives the message queue: a prompt queued by member B and
  dispatched later is still attributed to B, not to the dispatching goroutine or
  the workspace owner.
- Agent output carries an empty author; engine-driven step transitions carry an
  empty actor. Empty means "not a human", never "unknown human".
- No code path falls back to the workspace owner when the identity is absent.
  A test asserts that an identity-free write produces an empty author rather
  than the owner's ID.
- Agent stop/cancel and task state changes record the acting user.
- Attribution is asserted at each **producer boundary** — one test per write
  path — not only at a single consumer that renders it.
- DTOs carry `author_user_id` plus the resolved display name.

## Verification

- `go test ./internal/task/... ./internal/orchestrator/... -run 'TestAttribution|TestQueuedMessageAuthor'`
- `go test ./internal/... -run TestNoOwnerFallbackAttribution`

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/orchestrator/messagequeue/`
- `apps/backend/internal/task/service/service_events.go`, session message writes
- `apps/backend/internal/workflow/` step transition recording

## Inputs

- Spec: What (attribution), Data model, Persistence guarantees (attribution is
  permanent).
- Patterns: the derived-environment-contract rule in `apps/backend/AGENTS.md` —
  drive values through the producer seam and pair consumer coverage with a
  producer-boundary assertion so a missing publication fails loudly.

## Output Contract

List every write path that gained attribution with its producer-boundary test,
report the no-owner-fallback proof, RED/GREEN commands, and set this task plus
its plan checkbox to done.
