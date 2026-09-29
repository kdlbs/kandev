---
created: 2026-09-25
status: draft
requirements:
  - REQ-OFFICE-IDENTITY-GRADUATION-001
  - REQ-OFFICE-IDENTITY-GRADUATION-002
  - REQ-OFFICE-IDENTITY-GRADUATION-003
  - REQ-OFFICE-IDENTITY-GRADUATION-004
  - REQ-CANVASES-DEFAULT-AVAILABILITY-001
  - REQ-PLATFORM-LSP-CONTINUITY-GRADUATION-001
  - REQ-AGENTS-DYNAMIC-ROUTING-GRADUATION-001
system_design:
  - ../../specs/office/system-design/session-identity-graduation.md
  - ../../specs/canvases/system-design/default-availability.md
  - ../../specs/platform/system-design/lsp-continuity-graduation.md
  - ../../specs/agents/system-design/dynamic-routing-graduation.md
legacy_specs: []
---

# Implementation Plan: Graduate four runtime features

## Overview

Make Office per-agent session identity, canvases, browser-independent LSP
continuity, and dynamic agent routing permanent behavior. The final release has
no live feature toggle for these four capabilities. Office session identity has
been default-on since v0.94.0 and can enter its retirement work order now.
The other three first ship default-on with their kill switches intact, then
retire their flags in a later stable release. This follows the release sequence
in [ADR 0007](../../decisions/0007-runtime-feature-flags.md) and preserves a
working rollback path during first exposure. It is a delivery sequence, not a
change to the requested final behavior.

## Scope

### In scope

- Make Office participant-session binding and decision re-evaluation
  unconditional, while keeping `features.office` independent.
- Promote canvases, LSP continuity, and dynamic routing to default-on in all
  shipped profiles, then remove their release gates in later stable releases.
- Retire all four exact key/environment identities, leave old SQLite overrides
  inert, and remove their active feature response and settings entries.
- Preserve canvas grants and isolation, LSP resource/start policies, and dynamic
  routing fail-closed behavior; prove desktop and phone user outcomes.
- Update public configuration, feature-status, operator, and feature guides in
  the same implementation work orders as their behavior changes.

### Out of scope

- Enabling Office mode itself, changing authentication or multi-tenancy, or
  adding phone LSP.
- Changing canvas trust/grants, auto-creating applications, or making dynamic
  profiles the default selection for existing users.
- Rewriting historical flag overrides, Office session rows, or concrete agent
  profile bindings.

## Current evidence and dependencies

| Capability | Current state | Required before retirement |
| --- | --- | --- |
| Office session identity | `prod/dev/e2e=true` since v0.94.0; existing retirement criteria are explicit. | Focused Office identity and quorum regressions. |
| LSP continuity | Merged after v0.95.1, all profiles off. [Base plan](../lsp-browser-continuity/plan.md) is implemented. | One stable default-on release and Local PC/Docker lifecycle proof. |
| Canvases | All profiles off. [Base plan](../plugin-backed-canvases/plan.md) and [runtime permission fixes](../canvas-runtime-permission-fixes/plan.md) are done; [UX follow-up](../plugin-backed-canvases-ux-follow-up/plan.md) has Task 07 in progress. | Complete Task 07, verify permissions/isolation and real canvas publication, then one default-on release. |
| Dynamic routing | All profiles off. [Rollout blockers](../dynamic-agent-routing-rollout-blockers/plan.md) are complete, but the [base plan](../dynamic-agent-routing/plan.md) still records unfinished conductor, caller, Office, observability, and E2E work. | Reconcile and complete the base plan, then one default-on release. |

The original plans keep their recorded statuses. Their open work orders are
dependencies, not silently reclassified as done by this package.

## Technical approach

### Release boundaries

1. Retire `features.officeSessionIdentity` with its already-default-on behavior.
2. Complete the canvas and dynamic prerequisites. Promote each of the three
   remaining flags by setting `prod/dev/e2e=true` in the canonical
   `apps/backend/internal/profiles/profiles.yaml`; retain their active registry
   entries, override precedence, and disabled paths. LSP can promote when its
   first stable release includes the implemented lease path.
3. After a stable release has exercised each default-on path, remove that
   flag's backend and frontend conditionals, legacy branch, typed field,
   profile key, registry entry, and public setting. Feature retirements can
   share a release after their individual evidence gates pass; they remain
   separate work orders because their runtime boundaries differ.

### Shared retirement contract

For each exact identity, append its key and environment variable to
`retiredRuntimeFlagIdentities` in
`apps/backend/internal/runtimeflags/registry.go`. Keep unknown
`runtime_flag_overrides` rows. Remove active fields from
`apps/backend/internal/common/config/config.go`, `defaultFeatureFlags`,
`/api/v1/features`, and the Feature Toggles registry. Remove any startup
configuration catalog entry that still classifies the old environment variable
as live. Tests must seed stale `false` environment and SQLite values and prove
the target behavior remains on and the former identity is absent from active
metadata. Other feature flags and settings remain unchanged.

### Runtime boundaries

- **Office:** make participant binding and calling-session re-evaluation
  unconditional in orchestrator and Office dashboard. Keep the Office-only
  transaction guard and live-preferring lookup; add no session migration or
  table-level uniqueness constraint.
- **LSP:** always construct and use the bounded lease manager at the gateway;
  remove the browser-owned proxy/client branch. Retain separate editor
  auto-start, executor, authorization, capacity, and phone-viewer checks.
- **Canvases:** always construct canvas services, authoring tools,
  notifications, and routes. Keep authorization, package validation, grant
  review, sandbox, and data-scope checks. Remove UI release-gate checks from
  desktop and phone surfaces without changing their composition.
- **Dynamic routing:** after the base plan is complete, always admit dynamic
  profile CRUD, selection, execution, route actions, and recovery scheduling.
  Remove only flag-dependent refusal paths; keep candidate validation,
  pre-result evidence, generation fencing, and durable route state.

## ASCII UI preview

`UI-01: Desktop, normal installation after promotion` maps to canvas AC .4 and
dynamic routing AC .5. The existing sidebar, settings card, and task panels
become available without an admin feature-toggle step.

```text
Workspace sidebar                 Settings > Agents
  Tasks                            Dynamic agents       [Add profile]
  Canvases                         Profile: Balanced    [Edit]
  Settings

Task > Canvas panel: [Create canvas] | existing canvases | runtime state
```

`UI-02: Phone, normal installation after promotion` maps to the same criteria.
The existing mobile navigation and direct profile editor remain separate
focused surfaces; a canvas opens as a task or workspace destination.

```text
Workspace navigation drawer       Settings > Agents > Dynamic profile
  Tasks                            Balanced
  Canvases                         Candidates  [Reorder] [Edit]
  Settings                         [Save]
```

`UI-03: LSP lifecycle` maps to LSP AC .4. Desktop and tablet retain their
existing status and Stop/Retry controls. Phone stays a file viewer.

```text
Desktop toolbar: Go  [~] Reconnecting ... [Stop] -> [Ready]
Tablet drawer:  Language server | Reconnecting ... | [Stop]
Phone viewer:   < Back | main.go | file content (no LSP control)
```

Structural choices are required; labels and spacing are illustrative and use
localized copy. Desktop and phone use existing domain state and distinct
presentation. Phone navigation and actions remain visible touch targets with
one scroll owner; no new modal or compressed desktop panel is proposed.

## Tests

| Acceptance | Evidence |
| --- | --- |
| Office 001/002/003/004 | Orchestrator, Office dashboard, session repository, and retired-identity contract tests. |
| LSP graduation 001.1-001.5 | Profile/registry contracts, gateway lease tests, client reconnection tests, desktop/tablet/phone E2E. |
| Canvas availability 001.1-001.5 | Profile/registry contracts, service/MCP/HTTP authorization and package tests, desktop/phone E2E. |
| Dynamic graduation 001.1-001.5 | Base-plan completion evidence, profile/route/conductor/recovery tests, desktop/phone E2E. |

Keep permanent enabled behavior tests when deleting disabled-path tests. Run
the focused runtime registry, config, and profile contract checks after every
retirement, since they require exact backend/frontend/profile key equality.

## E2E tests

Build the backend and `apps/web` E2E bundle after changing embedded profiles or
frontend code. For canvas specs, also build the E2E plugin UI/package. Use one
worker per guarded shard as documented in `apps/web/e2e/README.md`.

- Office: `tests/office/workflow-quorum-transitions.spec.ts` on `chromium`.
- LSP: `tests/lsp/lsp-file-intelligence.spec.ts` on `chromium`,
  `tests/lsp/mobile-lsp-file-intelligence.spec.ts` on `mobile-chrome`, and
  `tests/docker/lsp-file-intelligence.spec.ts` on `containers` with Docker.
- Canvas: `tests/canvas/plugin-canvas.spec.ts` on `chromium` and
  `tests/canvas/mobile-plugin-canvas.spec.ts` on `mobile-chrome`;
  include `canvas-host-origins.spec.ts` for the hosting boundary.
- Dynamic routing: finish the base plan's dedicated desktop/mobile project
  matrix and rerun those specs without a feature override; include settings,
  task, utility, workflow, and Office routing scenarios.

## Work orders

- [x] [Task 01: Retire Office session identity flag](task-01-office-session-identity.md)
- [x] [Task 02: Promote LSP continuity](task-02-lsp-default-on.md)
- [ ] [Task 03: Promote canvases](task-03-canvases-default-on.md)
- [ ] [Task 04: Promote dynamic routing](task-04-dynamic-routing-default-on.md)
- [ ] [Task 05: Retire LSP continuity flag](task-05-lsp-retirement.md)
- [ ] [Task 06: Retire canvas flag](task-06-canvases-retirement.md)
- [ ] [Task 07: Retire dynamic routing flag](task-07-dynamic-routing-retirement.md)

Execute sequentially in the primary conversation. A promotion and its
retirement cannot ship in the same stable release. Task 04 waits for the base
dynamic routing plan; Task 03 waits for the canvas authoring follow-up. Tasks
05-07 wait for their own first default-on stable release and recorded evidence.

## Verification results

Task 01 retired Office session identity after its prior default-on release.
Task 02 promoted LSP continuity to default-on with the restart-required kill
switch retained. Focused Go tests, backend lint/build, frontend typecheck/build,
32 focused Vitest tests, public-doc validation, 19 desktop E2E tests, 4
phone/tablet E2E tests, and 3 Local Docker E2E tests passed. See each work order
for command details.

Tasks 03 and 04 remain pending behind the canvas authoring follow-up and dynamic
routing base plan. Tasks 05-07 remain pending until their default-on stable
release evidence exists.

## Risks

- Retiring an old false override intentionally turns the behavior on for that
  installation. Release notes must call out this compatibility change.
- Canvas default exposure increases the number of installations able to run
  agent-authored packaged code. Existing grants, isolation, and startup failure
  recovery must be proven before promotion.
- LSP continuity retains task hosts and processes after browser detachment;
  capacity and one-hour expiry must bound resource use under default-on load.
- Dynamic routing cannot be promoted merely because its rollout-blocker repair
  is complete. Its base plan still lists unfinished lifecycle and E2E work.
- Each retirement touches shared registry/profile/frontend contract files.
  Implement them serially and rerun the exact contract tests after each edit.
