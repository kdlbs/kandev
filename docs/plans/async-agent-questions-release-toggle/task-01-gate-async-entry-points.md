---
id: "01-gate-async-entry-points"
title: "Gate async question entry points"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-011
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-011.1
  - AC-TASKS-ASYNC-QUESTIONS-011.2
  - AC-TASKS-ASYNC-QUESTIONS-011.3
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 01: Gate async question entry points

## Summary

Resolve one startup flag and enforce it at every backend entry point for async
questions. Keep the blocking ask and reply protocols available.

## In scope

- Register `features.asyncQuestions` with the environment variable
  `KANDEV_FEATURES_ASYNC_QUESTIONS`, feature kind, experimental stability,
  medium risk, mutable override, and restart required. Use
  `AsyncQuestions bool` with `mapstructure:"async_questions"` and
  `json:"asyncQuestions"`. Add the all-false shipped profile defaults and
  frontend contract key in the same change. Reuse the existing registry,
  runtime endpoints, and boot projection.
- Use the effective startup value in the executor and the composed backend.
  Gate the live service, authority and wait hooks, worker, HTTP routes, Inbox
  readers, and async attention subscriptions. With no async reader, shared
  Inbox hide, snooze, and restore actions for async IDs return 404 before
  writes. The default boot uses the normal schema and blocking composition.
  Confirm the flag key and environment name do not collide with active or
  retired registrations.
- Make the typed `CapabilityAsyncQuestions` grant explicit. Remove the
  unconditional grant in `profile.Legacy`. Cover Core package and native
  tool listing, input and result schemas, and tool descriptions. A valid
  Autopilot root can still read children when enabled, and a child keeps
  exclusive parent-question authority.
- Use the same decision for prompt guidance at task start and restart.
  Intersect any prepared profile and persisted guidance booleans with the
  current startup flag before generating a new catalog or prompt. Trace
  manual, workflow, prewarmed, and recovery paths. Preserve stored history,
  original title and completion instructions, and scoped MCP bindings.
- Exercise direct WebSocket actions and stale agent catalogs with the service
  disabled. Preserve blocking ask, timeout, rejection, and parent reply
  handling. In `handleMessageTask`, verify that an async ID cannot bypass
  `validateParentQuestionReply` and dispatch as a normal message.

## Out of scope

- Retained wait and queue cleanup, which task 02 owns.
- Client mount conditions and rendered Settings verification, which task 03
  owns. This task adds the feature contract key and backend registry metadata.
- Changing the provider process or a user's workflow to refresh a cached tool list.

## Acceptance

- The real startup resolver provides the same effective value to the Settings
  registry, feature API, hydrated boot state, runtime, and MCP profile.
  Off by default and environment locking both have regression coverage.
- Disabled or stale calls cannot create, read, answer, forward, withdraw, or
  wait asynchronously. Both blocking flows retain their previous behavior.
- On and off tests prove route absence and enabled authorization, no async
  Inbox or boot contribution while off, and no off worker or event producer.

## ASCII UI preview

### UI-01: Feature Toggles, administrator view

The registry adds this card to the existing settings layout.
See the [desktop and phone views](plan.md#ui-01-feature-toggles-administrator-view).
Coverage: AC-TASKS-ASYNC-QUESTIONS-011.1.

```text
Desktop: [Async agent questions   Off] [Use default]
Phone:   [Async agent questions]
         [Off]
         [Use default]
```

Task 03 verifies the existing save, restart, and environment lock states.

## Verification

From the repo root:

```bash
(cd apps/backend && go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/profile ./internal/mcp/server ./internal/mcp/handlers ./internal/orchestrator/executor ./internal/orchestrator ./internal/task/handlers ./internal/sysprompt ./internal/backendapp -run 'AsyncQuestion|AskUserQuestion|ParentQuestion|Blocking|ResumeWorkspaceHelperMCP|BootInitialStateIncludesFeatureFlags' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/questions ./internal/clarification -run 'AsyncQuestion|Inbox' -count=1)
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
git diff --check
```

Use the [scoped Go lint command](plan.md#verification-results). In a fresh
worktree install dependencies with `(cd apps && pnpm install --frozen-lockfile)`
before the frontend command.

## Files likely touched

- `profiles.yaml`, `apps/backend/internal/common/config/config.go`,
  `apps/backend/internal/runtimeflags/registry.go`, and their contract tests.
- `apps/backend/internal/backendapp/{helpers,gateway,orchestrator,boot_state_routes}.go`
  and the async question and resume catalog tests.
- `apps/backend/internal/orchestrator/executor/{executor,executor_execute,executor_builtin_mcp}.go`,
  the prompt and recovery callers in `internal/orchestrator`, and their tests.
- `apps/backend/internal/mcp/{profile,server,handlers}/` capability and relay
  code and tests, including `profile.go`, `core_registration.go`,
  `handlers_ask.go`, and `parent_question.go`.
- `apps/web/lib/state/slices/features/{types.ts,features-contract.test.ts}`.

## Dependencies

None. Base the work on the current PR #4980 branch.

## Risks

- The legacy MCP constructor and a prepared or recovered profile can bypass
  a gate placed only on the fresh launcher. Test both profile shapes and the
  complementary backend rejection when a stale catalog survives.
- Blocking parent replies share part of the async reply path. Keep the
  correlation and authorization checks in the blocking fallback.

## Parallelism

`sequential`

## Inputs

- [Installation release gate](../../specs/tasks/system-design/async-agent-questions.md#installation-release-gate).
- [Requirements](../../specs/tasks/requirements/async-agent-questions.md)
  and [runtime flag contract](../../decisions/0007-runtime-feature-flags.md).
- `SetCanvasesEnabled` and the existing typed runtime flag tests as precedents.

## Results

Merged the 2026-11-22 base with normal hooks before implementation. Red
regressions confirmed the missing profile defaults and flag registration,
the legacy async grant and output union, unsupported executor and prepared
profiles, stale prompt guidance, and deletion with the feature service absent.

The exact verification commands above passed on the resulting worktree:

- Flag, config, and profile suites passed in all 3 packages.
- The entry-point filter passed in all 8 packages. The first run found MCP
  schema and description expectations that were corrected to use the existing
  blocking contract and an explicit enabled control.
- The async question and Inbox filter passed in both packages.
- The frontend feature contract passed all 11 tests.
- Scoped Go lint passed with 0 issues on a sequential retry. The first run
  reached its timeout after reporting 0 issues. Whitespace check passed.

Direct async calls and stale correlated replies cannot create work while off.
The disabled startup leaves the async service, routes, and notification
subjects absent. The shared Inbox rejects async sidecar writes. Task deletion
now purges async rows in the repository transaction, independent of the
optional service.
