---
id: "04-startup-timings"
title: "Show startup durations and prove combined restart behavior"
status: done
wave: 4
depends_on:
  - "03-schedule-session-restoration"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-001
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-003
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-004
acceptance_criteria:
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-001.1
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-001.2
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-003.1
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-003.2
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-003.3
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-003.4
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-003.5
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-004.1
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-004.2
system_design:
  - ../../specs/platform/system-design/startup-housekeeping.md
---

# Startup durations and combined restart proof

## Summary and scope

Use TDD to retain bounded step-activation durations in the existing startup
reporter and render them in the Go bootstrap page, React restart dialog, and
launcher. Freeze completed and total startup timings. Prove the complete
housekeeping-to-two-session-restore flow with isolated desktop/mobile E2E.

## Owned files and dependencies

- `internal/startup/{progress,step_lifecycle,step_snapshot}.go` and tests.
- `internal/backendapp/startup_page.go`, startup-page tests and existing test fixture.
- `internal/launcher/health.go` and readiness parsing/output tests.
- `apps/web/lib/startup-progress/*`, `hooks/domains/system/use-startup-progress*`,
  `components/settings/system/restart-progress-dialog*` and tests.
- Backend/web startup catalogs, existing registry completeness tests.
- Existing layout startup E2E and new desktop/mobile session housekeeping E2E.
- Public docs: extend the existing startup/backup explanation in
  `docs/public/configuration.md` (reference/explanation section); search root
  README and screenshot catalog for affected wording without replacing media.

Task 03 must be complete. Preserve existing phase/status fallback, boot identity,
auth, Accept negotiation, route return, and localization behavior.

## Implementation acceptance

1. Wire history is bounded and compatible, concurrent reads are coherent,
   repeated activations are distinct, forced closures never claim success, and
   all durations freeze at their correct boundary. The maximum serialized
   payload fits each bounded consumer or its reader is updated in this change.
2. Go page, React dialog, and launcher show truthful current/completed timing;
   boot reset and lost contact preserve the specified state. All supported
   locale catalogs and phone containment checks pass.
3. A real isolated restart with two candidate sessions and many retained stopped
   rows finishes housekeeping before activation, reaches readiness despite a
   blocked remote, and restores the selected original conversation with unchanged
   prompt counts. Capture screenshots for desktop and phone; complete public docs.

## ASCII UI preview

UI-01 through UI-03 use the [combined plan](plan.md#ascii-ui-preview).
Shared desktop/phone hierarchy:

```text
Starting Kandev             Elapsed: 12 seconds
Reconciling queued work     Elapsed: 2 seconds
Progress unavailable
Completed steps
  Database backup          3 seconds
  Prompt ordering          1 second
  Service stores           6 seconds
```

On phone, total and per-step durations wrap below their labels; the title/current
step stay above the one scrolling completed list. Reuse the shipped mobile
restart dialog and Go startup page, respect safe areas and long translations,
and retain existing 44px phone action targets. UI-02 adds `Last known` on contact
loss; UI-03 never marks interrupted work complete. Timers do not repeatedly
announce the full list to screen readers. Copy/numbers here are illustrative.

## Verification

Run desktop and mobile commands sequentially. Managed E2E builds current source
and owns teardown. Confirm all new specs are discovered by their owning project.

```bash
(cd apps/backend && go test -race ./internal/startup ./internal/backendapp ./internal/launcher ./internal/i18n -run 'Startup|Housekeeping|Bootstrap|Ready|Locale|Registry|Snapshot' -count=1)
(cd apps/web && pnpm exec vitest run lib/startup-progress hooks/domains/system/use-startup-progress.test.ts components/settings/system/restart-progress-dialog.test.tsx hooks/domains/session/use-session-resumption*.test.*)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/startup-page.spec.ts tests/layout/restart-progress-startup-detail.spec.ts tests/session/startup-housekeeping.spec.ts tests/session/agent-survival-restart.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-startup-progress.spec.ts tests/session/mobile-startup-housekeeping.spec.ts tests/session/mobile-agent-survival-restart.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run changed-file Go lint and targeted frontend lint for every changed source.
Run local PR documentation coverage preflight as documented by `/pr` before
publication; do not treat this plan's existence as completed work-order evidence.

## Risks and exclusions

Do not introduce a second progress stream or derive a duration from the client's
clock. Malformed optional history must not invalidate an otherwise valid status.
Retain a bounded last-known view during downtime; no new native desktop shell
bridge, historical metrics dashboard, or separate completion screen.

## Results

Implemented bounded completed-step history, per-activation and frozen total timings,
and the five named housekeeping operations in the existing reporter. Go startup
page, React restart dialog, and launcher render the same snapshot. Optional history
is compatible with older/malformed payloads. All supported catalogs are updated.

Validation:

- Startup/history/bootstrap/launcher and locale-registry tests passed with the
  race detector; the final consolidated backend run covered nine affected packages.
- Focused Vitest suite: 120 tests passed across 11 files. Web typecheck passed
  with `NODE_OPTIONS=--max-old-space-size=8192`; targeted ESLint passed.
- Hant generation, `i18n:check`, and `i18n:ratchet` passed.
- Managed Chromium E2E: six checks passed, including startup renderers, the real
  two-conversation restart with 100 retained stopped rows and an unavailable SSH
  owner, and existing survival coverage. The final desktop capture rerun passed
  three renderer checks after explicitly awaiting dialog animations.
- Managed mobile-chrome E2E: three checks passed, covering both timing renderers,
  the real restart, and existing survival behavior. The combined renderer test
  preserves both phone assets in its manifest. Desktop and phone assets were
  compressed and visually inspected; they remain ignored under `.pr-assets`.
- Public documentation validation: 63 tests and all 48 published pages passed.
  Spec validation and all-file lint passed; the spec linter's 36 tests passed.

The isolated restart asserts completed housekeeping, frozen ready time, preserved
remote ownership, unchanged conversation/turn identity, prompt and queue counts,
and phone containment. Controlled prompt-history timing evidence is in task 01;
these checks do not claim a universal startup latency target.

PR review refinement: boot-id completion continues progress polling until the final
ready snapshot, then freezes it; missing final detail clears an earlier active
projection. The new hook regression failed before the fix and passed after.
Desktop and phone completion checks now hold the final response behind boot-id
completion to exercise the race. The real restart helper waits for the persisted
agent message before asserting UI content, avoiding a custom DOM wait budget.

The launcher payload check now uses the largest registered fields, full active
progress, maximum numeric widths, and 32 history entries. Latest focused frontend
verification passed 118 tests across 11 files; typecheck and targeted ESLint passed.

CI exposed a stale restart-dialog assertion prohibiting final progress polling.
It now expects final-ready detail after completion and preserves no-poll checks
for idle, starting and error phases. The focused run passed 129 tests across 12
files; production UI is unchanged from the verified post-commit captures.

The real Kubernetes restart case also retained an obsolete per-session log wait.
It now checks waiting-session identity, unchanged user messages, and retained Pod
and PVC identities before proving workspace readiness and manual recovery. The
old assertion reproduced locally; both real Kind cases passed with retries
explicitly disabled. No production UI change requires new screenshots.
