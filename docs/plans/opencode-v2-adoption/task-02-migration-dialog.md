---
id: "02-migration-dialog"
title: "Activate migration through the update dialog"
status: completed
wave: 2
depends_on:
  - "01-runtime-selection"
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
acceptance_criteria:
  - AC-AGENTS-OPENCODE-V2-001.3
  - AC-AGENTS-OPENCODE-V2-001.4
  - AC-AGENTS-OPENCODE-V2-001.5
  - AC-AGENTS-OPENCODE-V2-001.6
  - AC-AGENTS-OPENCODE-V2-001.8
  - AC-AGENTS-OPENCODE-V2-001.9
  - AC-AGENTS-OPENCODE-V2-001.10
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 02: Activate migration through the update dialog

## Summary

Deliver an explicit migration action in the current update dialog with the matching backend transaction.
Only a validated candidate becomes the durable managed v2 selection for all OpenCode profiles.

## In scope

- Trusted family/revision DTOs, HTTP handlers, preview/status/job projections, and client contracts.
- Server authorization, stale-preview checks, duplicate-job handling, and maintenance admission shared with lifecycle launches.
- Block activation while owned executions remain live; handle unknown remote liveness conservatively.
- Exact managed staging, isolated ACP probe, atomic Save, and post-commit real capability refresh.
- Failures before/after activation, interrupted jobs, re-open/restart status, and no global native update.
- Desktop Dialog and phone Drawer with shared state, explicit scope, translated copy, and all six locales.
- Existing v1 updates and existing other-agent update interactions continue to work.

## Out of scope

- Saved-session restore behavior and automated conversion of upstream data or plugins.

## Acceptance

1. Explicit migration installs/probes managed v2 and saves only on success; installation, probe, DB, stale-revision, permission, and liveness failures have the specified boundaries.
2. Launch admission and activation cannot race; no active OpenCode process is killed, and fresh discovery failure after commit leaves v2 selected.
3. Desktop and phone deliver opt-in, scope disclosure, progress, retry, and durable success with accessible geometry and correct translations.

## ASCII UI preview

### Dialog simplification (2026-09-28)

The user requested less update explanation in the dialog. This revision supersedes the expanded explanatory text and command block in UI-01/UI-02 below.
Reuse `SettingsInfo`: hover/focus reveals details on desktop, and tapping opens its accessible information sheet on touch devices.
Keep the version change, runtime choice, migration warning, and primary action visible. Hide the redundant header description visually but retain its accessible description.
The command is an initially collapsed native details disclosure. Existing translations are reused.

```text
Desktop dialog                  Phone update drawer
Update OpenCode                 Update OpenCode
Upgrade to v2 (i)               Upgrade to v2       (i)
1.18.32 -> 2.0.18                1.18.32 -> 2.0.18
Version summary                 Version summary
[Update v1] [Upgrade to v2]      [Update v1] [Upgrade to v2]
Stop external v1 processes      Stop external v1 processes
> Command that will run         > Command that will run
[Cancel] [Upgrade to v2]         [Upgrade to v2] [Cancel]
```

The information disclosure holds shared profile scope, unchanged standalone CLI, model refresh behavior, and future-launch semantics.
The desktop test checks hover/keyboard disclosure; the phone test checks tap, a 44px target, closing the information sheet back to the update drawer, and expanding the command.
No update request is sent by either disclosure. Runtime behavior and activation semantics are unchanged.



Excerpts of [UI-01, UI-02, and UI-03](plan.md#ascii-ui-preview), covering AC-AGENTS-OPENCODE-V2-001.3, .5, .6, .8-.10.

```text
UI-01 Desktop Dialog
+------------------------------------------------+
| Update OpenCode                            [X] |
| Current v1              Target managed v2       |
| [Update v1] [Upgrade to v2]                     |
| All profiles. Standalone CLI unchanged.         |
| Stop external v1 processes sharing data.        |
| Details and progress (scrolling body)          |
|------------------------------------------------|
| [Cancel]                       [Upgrade to v2]  |
+------------------------------------------------+

UI-02 Phone inset Drawer
  +--------------------------------+
  | Update OpenCode            [X] | fixed
  | Current v1 -> managed v2       |
  | [Update v1] [Upgrade to v2]    |
  | Shared scope and details      | scroll
  | Progress or error             |
  |--------------------------------|
  | [       Upgrade to v2        ] | fixed
  | [          Cancel           ] | safe area
  +--------------------------------+

UI-03: Blocked / Installing / Checking ACP / Saving
       Failed, v1 retained -> Retry upgrade
       Succeeded -> Done
       V2 selected, discovery failed -> Retry discovery
```

Control order, explicit choice, shared scope, scrolling, and footer reachability are required.
Spacing is illustrative. Use existing primitives; phone targets measure at least 44px.

## Verification

Run from the repository root. Install dependencies once if this worktree has no `apps/node_modules`.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/agent/settings/... ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agent/managedruntime)
(cd apps/web && pnpm exec vitest run lib/agent-runtime-update.test.ts lib/api/domains/agent-update-api.test.ts components/settings/agent-runtime-update-control.test.tsx hooks/domains/settings/use-agent-runtime-updates.test.tsx hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings/agent-runtime-update-control.tsx components/settings/agent-runtime-update-surface.tsx lib/agent-runtime-update.ts lib/api/domains/agent-update-api.ts hooks/domains/settings/use-agent-runtime-updates.ts hooks/domains/settings/use-agent-runtime-update-statuses.ts)
(cd apps/web && pnpm e2e:run --project chromium -- tests/settings/agent-runtime-update.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/settings/mobile-agent-runtime-update.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Extend the explicit lint/test file list if implementation extracts new helpers.
Inspect one rendered phone view and record its match to UI-02 with the targeted E2E results.

## Files likely touched

- `apps/backend/internal/agent/settings/controller/agent_update.go`, `agent_update_job.go`, `agent_update_status.go`, maintenance helpers, and new `opencode_migration_test.go`.
- `apps/backend/internal/agent/settings/dto/dto.go` and agent-update HTTP handlers/tests.
- `apps/backend/internal/agent/runtime/lifecycle/` launch admission and `hostutility/` probe environment/capability publication.
- `apps/web/lib/api/domains/agent-update-api.ts`, `lib/agent-runtime-update.ts`, their tests, and settings hooks/tests.
- `apps/web/components/settings/agent-runtime-update-control.tsx`, `agent-runtime-update-surface.tsx`, and control tests.
- `apps/web/app/settings/agents/page.tsx` if new status projection needs wiring.
- `apps/web/src/locales/` and both existing Settings runtime-update E2E files/helpers.

## Dependencies

Task 01: authoritative runtime resolver and persisted family/source.

## Risks

An isolated probe does not discover authenticated user models; publishing that catalogue would erase the visible model list.
The existing maintenance coordinator may not gate every lifecycle entry; verify admission coverage instead of assuming it.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/opencode-v2-adoption.md): Activation, Settings, Security.
- Existing `runExactCandidate`, update-surface component, and Settings E2E fixtures.
- [Plan test and E2E matrices](plan.md#tests).

## Results

Implemented family/revision-aware previews and migration jobs, serialized launch admission, candidate staging/probing, atomic selection activation, and translated desktop dialog/mobile drawer controls.

- Migration controller tests passed for stage/probe/save failures, stale revisions, duplicate jobs, blocked admission, and request binding.
- Focused web tests passed: 59 tests across 6 files; web typecheck and i18n checks/ratchet passed.
- Desktop runtime-update E2E passed: 17/17, including migration opt-in, failed-probe retention, and explicit retry.
- Mobile runtime-update E2E passed: 6/6, including touch-size and drawer overflow checks.
- Targeted ESLint completed with zero errors and 11 warnings.

## Review follow-up (2026-09-28)

Moved OpenCode utility command resolution inside the shared operation lease for profile prompts, ordinary prompts, capability refreshes, and model-configuration probes. Migration clears capability and model-config caches after saving the v2 selection and before releasing exclusive utility admission. Native-source update, repair, and Use Kandev default jobs keep the source-native selection valid and report the probed installed version. The isolated migration probe keeps its HOME, XDG, and OpenCode config/database overrides after subprocess sanitization and strips inherited OpenCode directory/content overrides.

- `go test -race ./internal/agent/hostutility -count=1`: passed, including subprocess environment isolation, all four utility request paths, and both utility-first and migration-first admission orderings.
- `go test -race ./internal/agent/settings/controller -count=1`: passed, including SQLite-backed install/update regressions and migration activation ordering.
- `go test -race ./internal/agentctl/server/utility -run TestProbe -count=1`: passed.
- `go test -race ./internal/agent/runtime/lifecycle -run 'OpenCode|ManagedRuntime' -count=1`: passed, including deterministic utility-versus-migration admission orderings.
- The subprocess-boundary isolation regression verified effective HOME/XDG/OpenCode paths, ignored inherited OpenCode overrides, preserved npm cache/userconfig, and unchanged sentinel user config/database files.
- Local Docker/SSH recovery E2E remains unverified because the shared `/tmp` filesystem was full and the container runtime could not create a temporary runc process file. All six container shards passed in the PR CI run after the review fixes.
- PR run `36400881920`, attempt 2 at `82cf819d078`, ended with 57 checks passed, 10 skipped, one failed, and two E2E aggregates pending. The leaf failure included the mobile clarification send target being covered by the transient update toast; the then-current local test passed after waiting for the toast to hide. The shard also reported the task-deletion test as flaky after a stale deletion preview once returned 409 and its retry passed.
- The verification-record update passed `node scripts/validate-public-docs.mjs` (47 pages) and `TMPDIR=/root/.cache/kandev-go-tmp node --test scripts/validate-public-docs.test.mjs` (62/62).
- `git diff --check`: passed.

## Fixup verification (2026-09-28)

The update dialog keeps its migration warning and version choice visible while details and the command move behind disclosures. The existing Settings info component provides hover/focus details on desktop and a touch-open information sheet on phones. The user-facing runtime choice and migration semantics are unchanged.

- Desktop runtime-update E2E passed 17/17 with retries disabled; mobile runtime-update E2E passed 6/6 with retries disabled. The production Vite and backend builds completed as part of these runs.
- The desktop update-control component and API-client helper Vitest files passed 13/13. Web typecheck and targeted ESLint passed.
- The task-deletion helper now refreshes only on the stale-preview 409; its new unit regression passed, and the focused deletion E2E passed with retries disabled. Four isolated pre-fix repetitions passed on the exact prior head, so the CI timing race was not reproduced locally.
- `go test -race ./internal/agent/settings/controller`: passed, including the managed npm prefix preparation regression.
- `python3 scripts/list-docs.py validate` validated 321 decisions and 1220 specifications; `python3 scripts/lint-spec-files.py --all`: passed.
- At the time of this initial fixup record, CI for remote head `e56f4dd3126` was still pending. The `82cf819d078` counts above are historical and do not verify that head or the later merge commit.

## Current-main merge verification (2026-09-28)

The PR's recorded base `89ff7ff7131` merged cleanly with the branch. The live `main` tip `a5b344b0368` conflicted in the mobile clarification spec: the branch's temporary toast wait overlapped with a newer main-side regression that deliberately shows the notification and asserts it does not cover Send. Current main moves the update toast to the top, so the merge retained that implementation and the explicit overlap assertion rather than the wait.

- The mobile runtime-update and clarification specs passed together 16/16 with retries disabled. This includes the main-side notification non-overlap regression.
- The desktop runtime-update and dev-server lifecycle specs passed together 20/20 with retries disabled, including the task deletion flow.
- `go build ./...` passed. `go test -race ./internal/agent/settings/controller` and OpenCode/managed-runtime-focused race tests passed for agents, hostutility, managedruntime, lifecycle, agentctl utility, and backendapp. The settings-handler package compiled, but no tests matched that name filter.
- Web typecheck and targeted ESLint passed; focused Vitest passed 18/18. `i18n:check`, `i18n:ratchet`, public-doc validation (47 pages), documentation catalog validation (324 decisions, 1230 specifications), and full spec lint passed.
- Live Sprites install verification remains unavailable; the local controller regression verifies private npm prefix preparation and selected-cache readiness.
- The merge commit and its exact-head PR CI are still pending at the time of this record.
