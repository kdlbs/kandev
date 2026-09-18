---
id: "10-workspace-grants"
title: "Explicit linked-workspace scope"
status: done
wave: 8
depends_on: ["05-tool-authority","08-assistant-ui","09-workflow-improvements"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-009
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-009.1
  - AC-ORCHESTRATION-ASSISTANT-009.2
  - AC-ORCHESTRATION-ASSISTANT-009.3
  - AC-ORCHESTRATION-ASSISTANT-009.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 10: Explicit linked-workspace scope

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S18, S20, and [plan](plan.md), Backend 8 and Frontend. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. An authorized owner explicitly links a workspace with operation and context-export scope; no automatic scan/adoption or profile fallback occurs.
2. Every broker operation rechecks owner access, target, account/profile, grant and intent revision; revocation blocks subsequent reads/wakes/writes.
3. Old workspace_coordinator tokens still reject foreign workspaces, and changing the central profile cannot silently export previously restricted context.

## Likely files

- apps/backend/internal/orchestration/models/assistant.go
- apps/backend/internal/orchestration/repository/sqlite/workspace_grants.go, workspace_grants_test.go (new)
- apps/backend/internal/orchestration/runtime/workspace_grants.go, workspace_grants_test.go (new)
- apps/backend/internal/agent/runtimeauth (new explicitly scoped assistant credential, compatibility checks)
- apps/backend/internal/backendapp/adapters_assistant_workspace.go (new)
- apps/web/app/assistant/workspace-links.tsx, workspace-links.test.tsx (new); assistant-api.ts

## Implementation sequence

Add human grant/revoke flows and owner checks before broker credentials. Bind target dispatch to grant revision and invalidate on revoke/profile change. Keep scope revalidation at the service boundary even with stale lists or queued runs. Explain context export and historical transcript limits in the UI. Test real and synthetic local identity without treating admin as cross-owner visibility.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/... ./internal/agent/runtimeauth ./internal/backendapp -run 'TestAssistant(Workspace|Grant|Broker)|TestRuntimeAPIScopesWritesAndRevokesFinishedRuns')
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test app/assistant/workspace-links.test.tsx)
(cd apps/web && pnpm run typecheck)
```

## Dependencies and risks

Dependencies: `05-tool-authority`, `08-assistant-ui`, `09-workflow-improvements`. Execute in the primary session unless the user explicitly authorizes subagents.

This is a new authorization/context-export boundary, not merely a workspace dropdown. Do not change the existing scoped runtime token's meaning.

## Output

One central conversation across explicitly authorized workspaces without flattening work/personal isolation.

## Detailed implementation checklist

1. Add Orchestration-owned grant rows with owner/binding/workspace uniqueness,
   operation allowlist, context-export allowlist, revision and revocation state.
   Migrate additively; default home-workspace scope never auto-links another workspace.
2. Add explicit human grant create/update/revoke APIs and UI. Validate current user
   visibility and allowed operations; require a revision for changes. Display
   exactly which workspace/context can be accessed, with generic examples only.
3. Introduce a separate assistant broker credential with its own audience and
   authority evaluator. Resolve one authorized target per read/wake/write and
   invoke existing canonical services. Never widen `workspace_coordinator` tokens
   or let callers choose a foreign target through ambiguous task metadata.
4. Workers keep their own workspace/profile credentials. Context assembly exports
   only fields covered by an explicit current grant; account-bound descriptors
   cannot migrate merely because task routing changes. Reconfirm incompatible
   grants after binding/profile/account changes.
5. Check grants at discovery, pagination, context export, scheduling and final
   dispatch. Revocation invalidates cached discovery/queues; it does not promise
   to erase context already delivered to a provider or undo completed writes.
6. Test concurrent grant update/revoke against an in-flight operation and durable
   restart. Keep idempotency keys scoped to owner/binding/target/action. Add both
   engine conformance and restore coverage for grant rows.
7. Update capability UI, settings/API documentation and audit receipts. Explicitly
   retain the old coordinator foreign-workspace rejection tests as a compatibility
   condition, even when the new assistant broker has an authorized linked target.

## Detailed evidence map

| Criterion | Planned test | Required edge cases |
| --- | --- | --- |
| AC-ORCHESTRATION-ASSISTANT-009.1 | `TestAssistantWorkspaceGrantDefaultsAndCAS` | Home only, invisible target, explicit allowlist, stale update, revoke |
| AC-ORCHESTRATION-ASSISTANT-009.2 | `TestAssistantWorkspaceGrantDispatchRace` | Wrong target, revoke after discovery, old queue/context, binding/profile change |
| AC-ORCHESTRATION-ASSISTANT-009.3 | `TestAssistantWorkspaceBrokerIsolation` plus existing coordinator rejection | Separate audience, worker account, old token cannot use broker, foreign task |
| AC-ORCHESTRATION-ASSISTANT-009.4 | `TestAssistantWorkspaceGrantRevocationReceipt` | Previously delivered context, completed write, truthful future-only revocation |

Run race tests around grants/authority, SQL guard and both-engine conformance.
Task 11 exercises grant creation, target selection, revocation and denial via UI.

## Scope boundaries and delivery

Do not implement organization-wide discovery, implicit workspace linking or
shared-user authorization in this task. If upstream contribution excludes grants,
keep the existing strict single-workspace credential contract intact.

## Parallelism

`sequential`

## Results

Complete on 2026-09-18. Grants are explicit owner/binding/workspace records with
operation/export allowlists, exact receiving profile/account revisions, CAS,
revocation and event audit. The existing coordinator token retains its home
workspace. The assistant broker resolves only an explicitly named linked target;
there is no installation-wide discovery or raw foreign task/transcript export.

Linked reads return bounded directory, task summary, result and current-input
projections. Context packets preserve the target worker profile and carry only
explicitly user-wide preferences from the central assistant, never home workspace
memory or account-bound credential descriptors. Native effects, queued context
and final wake/session dispatch recheck authority. Target/grant revision is part
of operation identity; a completed foreign receipt cannot replay against home.
Resolved attention can be acknowledged by an active session without bypassing
the original grant check.

Possible-delivery receipts retain the workspace/field/receiver metadata. A new
receiving account requires historical export reconfirmation before launch.
Revocation blocks future access; explicit forgetting deletes cached handoffs and
increments the grant revision so identical old references cannot be recreated.
Neither action claims to remove provider history or native tasks. The localized
Details panel exposes grant/review/revoke/forget controls, consent invalidation
when receiver or scope changes, and the paginated context-delivery audit.

### Verification

Behavioral red/green checks cover missing grants, CAS/concurrent revisions,
in-flight revocation, foreign-owner/native access, explicit target selection,
old coordinator denial, worker account preservation, home-memory exclusion,
queued context invalidation, receiving-account history reconfirmation, bounded
input export, wake dispatch and acknowledgement. Native adapter tests assert no
task creation when access is revoked immediately before its first effect.

With the repository toolchain, an owned PostgreSQL fixture and immutable local
maintenance image, these commands passed from `apps/backend`:

```sh
go run ./cmd/sqlguard ./internal
GIN_MODE=release go test -tags fts5 -race -count=1 ./internal/orchestration/... ./internal/persistence/storeconformance ./internal/backendapp ./cmd/agentctl
GIN_MODE=release go test -tags fts5 -race -count=1 ./internal/orchestration/... ./internal/agent/runtimeauth ./cmd/agentctl
GIN_MODE=release go test -tags fts5 -race -count=1 ./internal/orchestration/runtime ./internal/backendapp -run 'TestAssistantWorkspace|TestAssistantAttention|TestAssistantContext'
GIN_MODE=release go test -tags fts5 -race -count=1 ./internal/orchestration/runtime
golangci-lint run --new-from-rev=upstream/release-v0.94.0
```

Full store conformance passed in 127.7 seconds, including both-engine fresh,
replay and previous-stable upgrades. Full native backend checks passed in 51.3
seconds. Final runtime race checks after the wake acknowledgement repair passed
in 20.7 seconds. The two consent-form tests, web typecheck/scoped lint, all locale
checks, SQL guard, architecture, harness and specification validation passed.

Browser commands from `apps/web`, run separately with one worker and no retries:

```sh
pnpm e2e:run --host --shards 1 --project chromium tests/orchestration/assistant-workspace-links.spec.ts -- --retries=0
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/orchestration/mobile-assistant-workspace-links.spec.ts -- --retries=0
```

Each passed one test. They exercise explicit receiving-account consent, save,
forget, revoke and reload with only synthetic data, preserve the same central
conversation and assert no native agent session was launched. Both screenshots
were inspected. The mock central profile remains correctly unsupported. Combined
cross-feature/provider qualification and final PR media remain task 11; this
checkpoint neither qualifies a live build nor changes the running service.

Normal commit-hook review required decomposing six functions to stay within the
repository complexity limit. After that refactor the full Orchestration, native
adapter and both-engine store race checks passed again (34.4, 57.4 and 141.1
seconds respectively); SQL guard, PR-scope lint and a fresh desktop browser flow
also passed. No hook was bypassed.
