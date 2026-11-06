---
id: "01-step-entry-causation-from-ledger"
title: "Resolve step-entry wake causation from the step-transition ledger"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-CAUSATION-001
  - REQ-OFFICE-LAUNCH-SAFETY-003
acceptance_criteria:
  - AC-OFFICE-RUN-CAUSATION-001.3
  - AC-OFFICE-RUN-CAUSATION-001.9
  - AC-OFFICE-RUN-CAUSATION-001.25
  - AC-OFFICE-LAUNCH-SAFETY-003.2
system_design:
  - ../../specs/office/system-design/unattended-launch-safety-01.md
---

# Task 01: Resolve step-entry wake causation from the step-transition ledger

## Scope

- Carry the step-transition ledger id on step-entry `queue_run` and
  `queue_run_for_each_participant` requests (ledger `DispatchStepEntry`,
  including the marker-bearing path) and on the Office `auto_start_agent`
  enqueue.
- In the workflow-engine run-queue adapter, when the ledger id is present, read
  the `task_step_transitions` row. Office task launches carry their trusted causing
  run ID through lifecycle events into that row. Run-owned completions use their
  owner run ID. An `agent` row with `causing_run_id` resolves that exact run, without
  relying on mutable session binding or claim fields. A legacy row without the ID
  uses the session-and-time lookup. A `human` row yields a `user` actor with the
  human-rooted flag and no creating run. Any other actor falls back to the existing
  task-boundary carrier.
- Preserve existing trigger and actor semantics for all other workflow transitions.

## Acceptance

A review, reject, rework, review loop driven by agent `step_complete` calls
queues each successive step-entry run as a child of the previous run in one
chain, with `causation_depth` increasing by one per hop. The loop is refused at
the configured `office.maxCausationDepth`. A human board move queues a
`user`-actor, human-rooted root run at depth `0`.

## Verification

```bash
cd apps/backend && go test ./internal/steptelemetry ./internal/task/repository/sqlite \
  ./internal/runs/repository/sqlite ./internal/office/repository/sqlite \
  ./internal/office/service ./internal/orchestrator/watcher ./internal/orchestrator
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```

## PR Integration Verification Results

The conflict resolution keeps the causing run separate from runtime ownership
and durable delivery identity. It preserves delivery settlement before and after
workflow evaluation and the cancellation-specific completion cause.

The following combined race run passed eleven packages. Its task repository
package reached Go's ten-minute default timeout in the unchanged, landed
`TestDeliveryRetentionStorage/SQLite/1024_bytes` million-event storage test.
This is a local full-package validation limit, not a passing result.

```bash
cd apps/backend
go test -trimpath -race ./internal/steptelemetry ./internal/task/repository/sqlite \
  ./internal/runs/repository/sqlite ./internal/office/repository/sqlite \
  ./internal/office/service ./internal/orchestrator/watcher ./internal/orchestrator \
  ./internal/workflow/engine ./internal/runs/service ./internal/backendapp \
  ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor
```

All 48 tests declared in `step_transitions*_test.go` passed in a focused race
run using an anchored alternation of their test names:

```bash
step_transition_tests="^($(rg --no-filename '^func Test' \
  internal/task/repository/sqlite/step_transitions*_test.go \
  | sed -E 's/^func (Test[[:alnum:]_]+).*/\1/' | sort | paste -sd '|'))$"
go test -trimpath -race ./internal/task/repository/sqlite \
  -run "$step_transition_tests" -count=1
go test -trimpath -race ./internal/agent/runtime/lifecycle \
  -run '^TestHandleAgentEvent_CompleteConsumesCausingRunIDAfterReady$' -count=1
go run ./cmd/sqlguard ./internal
go test -trimpath -race ./internal/persistence/storeconformance -count=1
golangci-lint run ./... --new-from-rev=add376a2af17753ec349d9e7b042306c0f91100b --timeout=5m
```

These focused checks passed. Lint first reported zero issues but timed out;
the retry completed with zero issues. No PostgreSQL test DSN was configured,
so PostgreSQL-gated coverage was skipped locally. Specification lint, catalog
validation, harness lint, and the harness/spec validator tests passed.
Required CI must pass for the pushed integration head before merge; these
local results do not waive any remote failure.

## Frontend CI Remediation

The integration head's frontend check failed because the landed `useSecrets`
hook exceeded the 100-effective-line function limit (105 lines). Local ESLint
reproduced that exact warning before the fix. Extracting the existing visible
item selection into a private helper preserves global filtering, workspace
isolation, loading state, and mutation-publication semantics without changing
any rendered surface. The four existing owning test files cover those contracts;
no new production export or test-only seam was introduced.

```bash
cd apps/web
pnpm exec eslint --max-warnings 0 hooks/domains/settings/use-secrets.ts
pnpm exec vitest run hooks/domains/settings/use-secrets.test.ts \
  hooks/domains/settings/use-secrets.workspace-publication.test.tsx \
  hooks/domains/settings/use-secrets.lifetime.test.tsx \
  hooks/domains/settings/use-secrets.global-publication.test.tsx
pnpm run lint
pnpm run typecheck
```

The focused lint, 44 hook tests, full web lint, and typecheck passed.
This behavior-preserving extraction
requires no public documentation, specification, screenshot, or mobile-layout
change. Earlier integration-head CI evidence is superseded by the remediation
push; the new head must pass required CI before merge.


## Resumed Integration Verification

Actual current-main conflicts were resolved while retaining trusted causing-run
identity, passthrough launch fields, native-conversation replacement, the newer
executor-admission path, and ownership-schema initialization before the nullable
causing-run migration. The canonical main secrets hook already contains the
previous size-limit extraction and newer failure publication behavior, so that
implementation is retained without an additional frontend delta.

All 48 transition test functions plus
`TestDeliveryRetentionStorageSteadyStateMinimumQuantum` passed under race with a
disposable local PostgreSQL 16 DSN configured. This covers both SQLite and
PostgreSQL transition paths and the previously failing retention regression.
The full agent settings store package also passed under race with that DSN,
and `go run ./cmd/sqlguard ./internal` passed. The owned disposable database
container was removed after both runs completed.

The earlier local full-retention-suite timeout remains historical; it is not
represented as a passing full-package run. Fresh remote checks must validate
the new integration head before delivery.

The four owning secrets hook files passed all 44 tests. Full frontend lint and
full typecheck passed; typecheck required the same 8 GiB Node heap used in CI
after the default 4 GiB heap exhausted locally. Harness lint/tests, specification
lint/tests, and documentation catalog validation also passed for the integration.

All eleven affected backend packages passed the combined race run:

```bash
cd apps/backend
go test -trimpath -race -p 2 ./internal/steptelemetry \
  ./internal/runs/repository/sqlite ./internal/office/repository/sqlite \
  ./internal/office/service ./internal/orchestrator/watcher \
  ./internal/orchestrator ./internal/workflow/engine ./internal/runs/service \
  ./internal/backendapp ./internal/agent/runtime/lifecycle \
  ./internal/orchestrator/executor
```
