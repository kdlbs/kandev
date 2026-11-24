---
id: "03-promptsync-backend"
title: "Prompt Sync backend"
status: done
wave: 2
depends_on: ["01-extract-reposync", "02-prompt-ownership"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-002
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
  - REQ-INTEGRATIONS-PROMPT-SYNC-005
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-001.2
  - AC-INTEGRATIONS-PROMPT-SYNC-001.3
  - AC-INTEGRATIONS-PROMPT-SYNC-001.4
  - AC-INTEGRATIONS-PROMPT-SYNC-001.5
  - AC-INTEGRATIONS-PROMPT-SYNC-002.1
  - AC-INTEGRATIONS-PROMPT-SYNC-002.2
  - AC-INTEGRATIONS-PROMPT-SYNC-002.3
  - AC-INTEGRATIONS-PROMPT-SYNC-002.4
  - AC-INTEGRATIONS-PROMPT-SYNC-002.5
  - AC-INTEGRATIONS-PROMPT-SYNC-003.4
  - AC-INTEGRATIONS-PROMPT-SYNC-003.7
  - AC-INTEGRATIONS-PROMPT-SYNC-004.5
  - AC-INTEGRATIONS-PROMPT-SYNC-004.6
  - AC-INTEGRATIONS-PROMPT-SYNC-005.1
  - AC-INTEGRATIONS-PROMPT-SYNC-005.2
  - AC-INTEGRATIONS-PROMPT-SYNC-005.3
  - AC-INTEGRATIONS-PROMPT-SYNC-005.4
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 03: Prompt Sync backend

## Summary

Add `internal/promptsync`: the single-row config store, the service (config,
sync run, status), the poller, and the HTTP routes. Wire it into
`backendapp` with authorization, store conformance, and E2E reset.

## In scope

- `Store` with the `prompt_sync_config` table: get, upsert, delete, status,
  failure, and circuit writes. Register its requiredstores descriptor and
  storeconformance adapter.
- `Service`: config read, save, and delete with the install mutex. The sync
  run follows the system design's control flow: GitHub host binding and
  commit, `.md` selection, parsing, the reference-text read from the workflow
  repository, apply, and unresolved-reference warnings. Status and failure
  recording go through `reposync`.
- `ListPromptReferenceTexts` on the workflow repository, covering all
  workspaces, workflow prompts, and step prompts.
- A `Poller` on the 60s tick with Start/Stop/WaitGroup ownership and
  `goleak.VerifyTestMain`, plus fingerprint-triggered resume.
- Routes `/api/v1/prompt-sync/{config,sync}` with
  `authz.RequireOrgScope(ScopeOrgConfigManage)` on mutations and the workspace
  access check in the service.
- Expvar counters and structured logs from the system design.
- E2E reset deletes the config and releases synced prompts.

## Out of scope

UI, Workflow Sync warnings, and public docs.

## Acceptance

- Service tests with fake providers cover these cases: a `.md`-only
  immediate-file selection; name and content derivation; each invalid-file
  case freezing its prompt; duplicate names; a listing or download failure
  changing nothing; an empty source; a source change; and removal releasing
  synced prompts before deleting the config (a failed release keeps it).
- Failure tests cover transient backoff, GitHub credential and missing-path
  suspension, a missing credential workspace, a fingerprint reset, and
  sanitized errors. Poller tests cover the startup delay and the due interval.
- Handler tests cover `204` when unset, validation errors, a `403` without
  `org.config.manage`, a `404` for a foreign credential workspace, and a
  `404` for `sync` when unset.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/promptsync ./internal/workflow/repository/... -count=1
go test -trimpath -race ./internal/promptsync ./internal/persistence/storeconformance -count=1
go run ./cmd/sqlguard ./internal
go build -trimpath ./cmd/kandev
```

## Likely files

- `apps/backend/internal/promptsync/{models.go,store.go,service.go,parse.go,poller.go,handlers.go,metrics_vars.go,provider.go}` (new) and tests
- `apps/backend/internal/workflow/repository/*` (reference-text query)
- `apps/backend/internal/backendapp/{services.go,helpers.go,types.go,e2e_reset.go}`
- `apps/backend/internal/persistence/requiredstores/catalog.go` and `apps/backend/internal/persistence/storeconformance/adapters.go`

## Risks

- PostgreSQL dialect: use `{{boolean}}` templating and `Rebind`, and add an
  env-gated PostgreSQL store test.
- Hold the install mutex across fetch and apply, as Workflow Sync does, so a
  removal cannot interleave with an in-flight apply.

## Results

- `go test -trimpath ./internal/promptsync ./internal/workflow/repository/... -count=1`: pass.
- `go test -trimpath -race ./internal/promptsync ./internal/persistence/storeconformance -count=1`: pass.
- `go run ./cmd/sqlguard ./internal`: pass.
- `go build -trimpath ./cmd/kandev`: pass.
- `go test -trimpath ./internal/backendapp -count=1`: pass.
