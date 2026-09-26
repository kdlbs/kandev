---
spec: docs/specs/tasks/requirements/workflow-session-settings.md
decision: docs/decisions/2026-08-01-workflow-session-original-configuration.md
created: 2026-08-01
status: complete
---

# Implementation Plan: Conditional Workflow Session Settings

## Overview

Add a validated `configure_session` workflow action that selects one rule by the original session's immutable agent family, mutates that same session without changing tabs, and preserves successful values as explicit runtime overrides. New-session initialization gains a separate profile-settled checkpoint so Kandev can capture the immutable original effective configuration before a start-step rule is applied. The workflow editor reuses `ModelConfigSelector`, explains the mutually exclusive agent behaviors, and performs conservative family-specific carry-forward analysis over configured workflow transitions.

No workflow table migration or new HTTP/WebSocket endpoint is required. Rules continue to live in `workflow_steps.events`; original-session identity, original effective configuration, and any launch-time application bookkeeping live in task-session metadata.

## Architecture

### Workflow contract and validation

Add `configure_session` to the backend and frontend workflow action unions. Decode its config into typed rule helpers at the validation boundary rather than spreading `map[string]interface{}` assertions across the orchestrator. One reusable validator must cover step create, step update, and workflow import, including:

- one action per step and one rule per `agent_name`;
- `set`, `keep`, and `restore_original` payload rules;
- non-empty set values; and
- mutual exclusion with `workflow_step.agent_profile_id`.

Export remains a lossless JSON round trip. Family names, models, and ACP option IDs are not remapped through workspace profile matching.

### Original session and immutable configuration

Mark the first session created for a task with immutable original-session provenance in `TaskSession.Metadata`; do not use mutable `IsPrimary` as identity. Preserve the existing workflow-switch provenance marker for later sessions and add a conservative legacy resolver that accepts an earliest non-workflow-switch session only when the choice is unambiguous.

Add a metadata helper for the original effective configuration. Its write-once compare-and-set behavior must remain separate from:

- `acp_config_baseline` (provider-default comparison baseline);
- `runtime_config` (latest provider state); and
- `runtime_config_overrides` (explicit durable selections).

Refactor ACP initialization to carry profile settings and explicit runtime overrides as separate layers. The required order is:

1. initialize/load the ACP session and observe provider defaults;
2. apply profile model and profile select options;
3. read the settled model and every advertised select option and synchronously record the original snapshot for the original task session;
4. apply durable runtime overrides and any matching start-step workflow operation, field by field; and
5. dispatch the first prompt.

The runtime-facing API should use typed layer/result structures and narrow injected persistence callbacks. It must not make the lifecycle package depend directly on workflow models or task-service implementations.

### Runtime rule application

Resolve the original family from the immutable original session/profile snapshot, never from the currently selected step profile. Add a narrow session-runtime configuration writer to the orchestrator, implemented by the task service and wired in `backendapp`, so workflow changes use the same durable override semantics as chat configuration changes. Extend the existing runtime/agent-manager seam for dynamic ACP config options instead of importing lifecycle internals into workflow code.

For a running original session, apply model/options individually and persist each successful value. For a not-yet-running original session, carry the typed operation through launch initialization so it runs after original capture and before the prompt. `restore_original` expands to the captured model and all still-advertised select options. `keep` and no-match rules produce no provider or metadata mutation.

Collect field-level outcomes. One sanitized session warning reports rejected/unsupported fields and persistence failures while preserving successful fields and allowing auto-start to continue. A non-original active session produces a visible warning and no tab activation or mutation. Missing legacy restore data also warns and no-ops.

### Editor and carry-forward analysis

Replace the step header's single profile selector with an explicit agent-behavior choice:

- keep current behavior;
- use fixed agent profile; or
- configure original session by agent family.

The fixed-profile and conditional modes are mutually exclusive in local editor state as well as backend validation. Conditional rules use available-agent capability data grouped by stable `agent_name`. `set` embeds the shared `ModelConfigSelector`; `keep` and `restore_original` show explanatory copy and no value controls. Existing persisted values remain readable/removable if discovery is unavailable, but a new unverifiable `set` rule cannot be saved.

Implement carry-forward analysis as a pure frontend utility. Reuse or extract the configured transition-graph resolution from `replay-cycle-analysis.ts` so next/previous/explicit edges have one interpretation. For each family, run a fixed-point analysis from the workflow start step with an `original | changed | maybe_changed` lattice:

- `set` outputs changed;
- `restore_original` outputs original;
- `keep` preserves incoming state and records explicit intent;
- no rule preserves incoming state; and
- joins and cycles conservatively produce `maybe_changed` when paths differ.

Warn only when changed settings can enter a step and that step has no explicit rule for the family. Manual card moves remain outside the graph.

The step header keeps the fixed profile selector and adds an `Override original
session options` checkbox beside it. The checkbox's help copy includes the
Sol-to-Luna implementation example and is mutually exclusive with a fixed
profile. The conditional editor renders below WIP controls only while the
checkbox is enabled. Family choices are derived solely from configured agent
profiles; capability entries without a profile remain unavailable as new rule
targets.

### Mobile contract

Desktop and mobile use the same expanded workflow-step panel and the same rule data/helpers. Desktop may place family, operation, and selector controls in a compact row; below the existing phone breakpoint each rule becomes a one-column card. The page/document remains the single vertical scroll owner, with no nested full-height surface and no horizontal overflow. Add/remove/operation controls must retain at least 44px touch targets, and the shared selector's existing touch disclosure remains responsible for model/option selection.

## Tests

- Go model/controller/import tests cover every valid operation, duplicate families/actions, empty sets, profile/action conflicts, and export/import round trips.
- Go task/runtime tests cover immutable original-session provenance, write-once snapshot semantics, profile/default capture, restart/resume preservation, launch ordering, and legacy ambiguity.
- Go orchestrator tests cover family match/no-match, original-session enforcement, partial success, persistence failure, unsupported restore options, missing snapshots, start-step ordering, and auto-start continuation.
- TypeScript unit/component tests cover editor serialization, mutual exclusion, discovery failure, shared selector reuse, fixed-point path analysis, joins, explicit and relative transitions, and cycles.
- Playwright covers desktop creation/edit/reload/import behavior, an actual same-tab runtime switch before auto-start, visible partial-failure warnings, read-only synced display, and phone rule editing without overflow.

## E2E and documentation

Likely files:

- `apps/web/e2e/tests/workflow/workflow-agent-profile.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- a focused workflow session-settings runtime spec under `apps/web/e2e/tests/workflow/`
- `apps/web/e2e/pages/workflow-settings-page.ts`
- `docs/public/tasks-and-workflows.md`

Update public workflow documentation with the three agent behaviors, family matching, carry-forward/restore semantics, failure behavior, and the fact that conditional settings reuse the original conversation tab.

## Waves

Wave 1:

- [x] [task-01-workflow-contract-and-validation](task-01-workflow-contract-and-validation.md)

Wave 2 (can proceed independently after Task 01):

- [x] [task-02-original-session-initialization](task-02-original-session-initialization.md)
- [x] [task-04-editor-and-carry-analysis](task-04-editor-and-carry-analysis.md)

Wave 3:

- [x] [task-03-runtime-rule-application](task-03-runtime-rule-application.md)

Wave 4:

- [x] [task-05-e2e-docs-and-verification](task-05-e2e-docs-and-verification.md)

Wave 5:

- [x] [task-06-workflow-session-options-toggle](task-06-workflow-session-options-toggle.md)

## Verification

Targeted backend:

```bash
cd apps/backend && go test ./internal/workflow/... ./internal/task/models ./internal/task/service ./internal/agent/runtime/... ./internal/orchestrator/... ./internal/backendapp/...
```

Targeted frontend:

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/workflows components/settings
cd apps/web && pnpm run typecheck
```

Desktop and mobile E2E after rebuilding the web assets required by the fixture:

```bash
cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-agent-profile.spec.ts tests/workflow/workflow-session-settings.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts
```

Documentation and final repository checks:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
make fmt
make typecheck
make test
make lint
```

Recorded results for the implementation:

- `cd apps/backend && go test ./internal/workflow/models ./internal/workflow/service ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/backendapp` — 2751 tests passed in 5 packages.
- `cd apps && pnpm --filter @kandev/web exec vitest run lib/workflows components/settings` — 62 files and 321 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web run lint` — passed with zero warnings.
- `cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-settings.spec.ts` — 14 tests passed with production-build setup.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts` — 3 tests passed with production-build setup.
- `cd apps/web && pnpm run i18n:check` — passed with zero orphan keys and pseudo-locale in sync.
- `cd apps/web && pnpm run i18n:ratchet` — passed; all added and modified files clean.
- `node --test scripts/validate-public-docs.test.mjs` — 58 tests passed.
- `node scripts/validate-public-docs.mjs` — 41 published pages validated.
- Public workflow docs updated in `docs/public/tasks-and-workflows.md` for the new toggle and profile-backed family choices.
- `git diff --check` and backend `gofmt -l` checks — clean.

The repository-wide backend `make test` target was started but stopped after it exceeded the useful local verification window; the focused packages above cover every changed backend subsystem.

Task 06 verification for the header toggle and profile-backed family choices is complete.

PR conflict fixup: merged `origin/main` at `332353f647cf8ae157db893f529dfc4cb3516ba2` into
`feature/workflow-step-change-rkm` with merge commit `72bd08c0d116d513e1c63ba274ec94c17c49bbb1`.
The only conflict was `apps/backend/internal/task/models/models.go`; both original-session
provenance and context-window metadata keys were retained. At the new PR head,
`checks_snapshot_complete=true`, `failed_checks=[]`, `pending_checks` contains 15 queued or
pending checks, and unresolved review threads are zero; GitHub reports the PR as mergeable.

## Risks

- ACP initialization currently receives only the final effective configuration. Splitting profile and runtime layers must preserve existing chat-selected override, reset, resume, and compact-summary behavior.
- Provider config updates may arrive asynchronously. Original capture must wait for the profile-settled state and still complete before the first prompt; provider-default baseline events must not be repurposed.
- Start-step partial failures need field results without turning best-effort configuration into a launch failure. Keep configuration outcome reporting separate from fatal session initialization errors.
- Workflow cycles can make a family both original and changed at the same step. The editor must converge with a bounded lattice/fixed-point algorithm and avoid warning churn based on step ordering.
- Multiple profiles may share one agent family. Rules intentionally target the family; capability grouping must retain stable IDs and make unavailable persisted values visible instead of silently translating them.
