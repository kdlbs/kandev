---
id: "01-latest-run-indicator"
title: "Deliver latest-run indicators across automation navigation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RUNS-001.1
  - AC-OFFICE-AUTOMATION-RUNS-001.9
  - AC-OFFICE-AUTOMATION-RUNS-001.19
  - AC-OFFICE-AUTOMATION-RUNS-001.20
  - AC-OFFICE-AUTOMATION-RUNS-001.21
  - AC-OFFICE-AUTOMATION-RUNS-001.22
system_design:
  - ../../specs/office/system-design/automation-latest-run-indicator.md
---

# Task 01: Deliver Latest-Run Indicators Across Automation Navigation

## Summary

Make the automation navigation outcome match the newest run in history without
changing automation activity state or backend contracts. Deliver desktop sidebar,
agenda and active phone rows together, preserving running priority, paused-gray
priority and current refresh ownership.

## In scope

- Add the shared presentation helper and apply the scoped design's precedence.
- Sidebar non-success metadata before the same run's age, and agenda outcome
  metadata distinct from its next-firing/reason.
- Carry newest run through `useShortcutActivity` without changing activity state
  or custom pin rendering. Render active builtin phone rows using a small domain
  row; use it in the fallback too. Preserve catalog identities and availability.
- Translate the composite status/age frame in all complete catalogs and regenerate
  Traditional Chinese and pseudo locales. Wrap long labels without hiding failure.
- Behavioral unit/component regressions, active-route browser proof and screenshots.
- After proof, document indicator precedence in the public automation explanation.

## Out of scope

Backend/schema/status-projection changes, latest-success filtering, retry-group
aggregation, new refresh ownership, pin status changes, navigation redesign,
preferences writes, ages on phone/agenda, generic audits, commits or publishing.

## Acceptance

1. The sidebar and agenda use the newest authoritative run's status colors and
   metadata, while running/paused/never-run priority follows the scoped design.
   A newer success removes failure metadata after the accepted refresh, without
   reload; outcome and age cannot come from different runs.
2. The shipped saved-layout phone path shows the same outcome with visible status,
   retains distinct Running/Paused labels, and preserves history navigation,
   dismissal, >=44-px row target and zero horizontal document overflow. Custom
   shortcut indicators and loading/error semantics remain unchanged.
3. New behavioral coverage fails before the minimal production fix and passes
   afterward; all exact checks below pass against fresh managed browser builds.
   Inspect UI-01/UI-02/UI-03 and update the public explanation after proof.

## Implementation sequence

1. Read the linked requirement and full scoped design. Mark only this work order
   in_progress after an explicit implementation request. Bootstrap dependencies
   once if absent. Use /tdd, /mobile-parity and /e2e.
2. Confirm references to exported types with LSP if available; the planning
   environment reported no language server. Keep activity-state APIs unchanged.
3. Add failure-vs-idle and priority regressions first. Add real summary refresh
   component cases, including a failed newest run followed by a newer success,
   independent Running + Skipped, and paused + Failed. Use consumer-visible
   assertions, not palette-map copy tests or incidental function wiring.
4. Add the pure presentation helper, consume it in desktop row and agenda, and
   extend the existing phone activity projection with newest-run data. Keep one
   phone poller. Replace builtin phone row rendering only; do not change generic
   shortcut indicators, overflow aggregation, or pin labels. Share the new phone
   row with the fallback without activating the fallback route.
5. Update rendered/a11y metadata, seven-language catalogs and pseudo locale. Keep
   the age tick and established formatRelativeTime semantics. Run the focused
   unit checks once after integration, then fresh managed desktop/phone scenarios.
6. Update the public explanation and delivery Results with exercised commands,
   screenshot paths, actual counts and mismatches. Keep broad migrated
   automation-runs specs draft: this work proves only this bounded capability,
   not every behavior in the existing requirement/design. Mark this task done
   and the plan implemented only when all listed checks pass.

## ASCII UI preview

Full structural annotations and states: [plan previews](plan.md#ascii-ui-preview).
All three labels below match the plan; spacing and ages are illustrative.

### UI-01: Desktop sidebar, expanded Automations

```text
[red dot]   PR tagging                 Failed, 4m ago
[green dot] Dependency audit                   2m ago
[spinner]   Nightly drift                      1m ago
[gray dot]  Paused monitor             Failed, 8m ago
[green dot] Never-run check
```

### UI-02: Agenda row on /automations

```text
[red dot] ~09:00 tomorrow | PR tagging | Failed
[gray dot] Switched off  | Monitor    | Failed
```

### UI-03: Phone navigation drawer, expanded Automations

```text
v Automations
[red dot] PR tagging          Failed
[green dot] Dependency audit Succeeded
[blue dot] Nightly drift     Running
[gray dot] Paused monitor
                      Paused, Failed
Set up an automation
Open automations
```

UI-01/UI-02 cover AC-001.19-.21; UI-03 covers .20-.22. Preserve the existing
phone inset drawer and its nav scroller. No age on phone; ordinary short labels
remain compact and long status metadata wraps without horizontal overflow.

## Verification

From the repository root, with each package command independently rooted.
The install is a one-time prerequisite; do not repeat an already successful
bootstrap. New presentation and phone-row test files below are planned additions.

```bash
# From the repository root. Install once if this worktree is not bootstrapped.
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/runs/automation-run-presentation.test.ts components/runs/automation-rows.test.ts components/runs/run-status.test.ts components/runs/runs-list-page.test.tsx components/app-sidebar/sections/automations-section.test.tsx hooks/domains/sidebar/use-shortcut-activity.test.ts components/navigation/mobile-automation-row.test.tsx components/navigation/mobile-automations-section.test.tsx components/navigation/app-nav-sheet.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/runs/automation-run-presentation.ts components/runs/automation-run-presentation.test.ts components/runs/runs-list-page.tsx components/runs/runs-list-page.test.tsx components/app-sidebar/sections/automations-section.tsx components/app-sidebar/sections/automations-section.test.tsx hooks/domains/sidebar/use-shortcut-activity.ts hooks/domains/sidebar/use-shortcut-activity.test.ts components/navigation/mobile-automation-row.tsx components/navigation/mobile-automation-row.test.tsx components/navigation/mobile-automations-section.tsx components/navigation/mobile-automations-section.test.tsx components/navigation/mobile-sidebar-layout-navigation.tsx components/navigation/app-nav-sheet.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/automations-sidebar.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-navigation-hierarchy.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- apps/web docs/specs/office docs/plans/automation-latest-run-indicator docs/public/automation-and-mcp.md
```

Before each final browser run, use the managed runner's normal fresh build (no
--no-build). Do not run the two projects concurrently. Existing settings in the
worker backend must be restored in failure cleanup if changed by a fixture.

### Required browser evidence

- `tests/automations-sidebar.spec.ts`, Chromium: new case
  `latest failed run is red and recovers after a newer success`. Match Failed
  color/status to that run in details; keep agenda next-firing data. Observe
  newest-success replacement in a still-mounted sidebar using a payload-matched
  `automation.summaries` response, no reload/store mutation. Existing running and
  age cases remain controls; add paused failure.
- `tests/layout/mobile-navigation-hierarchy.spec.ts`, mobile-chrome: new case
  `phone automation rows expose latest failure and keep row-tap navigation`.
  Exercise SavedMobileAutomationRows, keep the drawer open through the newer
  success, then tap and assert correct destination/dismissal. Check paused failure,
  long localized status, >=44-px actual row height and document overflow.
- Use existing `prCapture` for desktop and phone. Inspect the rendered structure
  against all three previews; a screenshot alone does not prove state transitions.
- Ensure run timestamps establish newest ordering before asserting transitions;
  `created_at DESC, id DESC` does not guarantee insertion order for timestamp
  ties. Wait for the exact seeded newest run ID in accepted responses rather
  than treating any summary response as causal evidence.

### Documentation coverage preflight

Run from the repository root. This evaluates the actual linked package plus its
planned runtime paths, independent of GitHub PR state; it is not a remote check.

```bash
node -e 'const fs=require("node:fs"),v=require("./.github/scripts/pr-docs.cjs");const docs=["docs/specs/office/requirements/automation-runs.md","docs/specs/office/system-design/automation-runs.md","docs/specs/office/system-design/automation-latest-run-indicator.md","docs/plans/automation-latest-run-indicator/plan.md","docs/plans/automation-latest-run-indicator/task-01-latest-run-indicator.md"];const runtime=["apps/web/components/runs/automation-run-presentation.ts","apps/web/components/runs/runs-list-page.tsx","apps/web/components/app-sidebar/sections/automations-section.tsx","apps/web/hooks/domains/sidebar/use-shortcut-activity.ts","apps/web/components/navigation/mobile-automation-row.tsx","apps/web/components/navigation/mobile-automations-section.tsx","apps/web/components/navigation/mobile-sidebar-layout-navigation.tsx"];const r=v.validateCoverage({changedFiles:[...docs,...runtime].map(filename=>({filename,status:"modified",additions:1,changes:1})),fileContents:Object.fromEntries(docs.map(p=>[p,fs.readFileSync(p,"utf8")]))});console.log(JSON.stringify(r,null,2));process.exitCode=r.ok?0:1'
```

## Files likely touched

Planned additions are marked new; all other source/test paths were located.

- `apps/web/components/runs/automation-run-presentation.ts` (new)
- `apps/web/components/runs/automation-run-presentation.test.ts` (new)
- `apps/web/components/app-sidebar/sections/automations-section.tsx`
- `apps/web/components/app-sidebar/sections/automations-section.test.tsx`
- `apps/web/components/runs/runs-list-page.tsx`
- `apps/web/components/runs/runs-list-page.test.tsx`
- `apps/web/hooks/domains/sidebar/use-shortcut-activity.ts`
- `apps/web/hooks/domains/sidebar/use-shortcut-activity.test.ts`
- `apps/web/components/navigation/mobile-automation-row.tsx` (new)
- `apps/web/components/navigation/mobile-automation-row.test.tsx` (new)
- `apps/web/components/navigation/mobile-sidebar-layout-navigation.tsx`
- `apps/web/components/navigation/mobile-automations-section.tsx`
- `apps/web/components/navigation/mobile-automations-section.test.tsx`
- `apps/web/components/navigation/app-nav-sheet.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/automations.json`
- `apps/web/e2e/tests/automations-sidebar.spec.ts`
- `apps/web/e2e/tests/layout/mobile-navigation-hierarchy.spec.ts`
- `docs/public/automation-and-mcp.md`, Read the output section
- This work order and `plan.md`, delivery results/status only

Reference-only: `automation-rows.ts`, `run-status.ts`, their existing tests,
`use-sidebar-layout-navigation.ts`, `shortcut-section-actions.tsx`, and
`shortcut-activity-indicator.tsx`. Do not change their unrelated semantics.

## Dependencies

None. One vertical slice, wave 1.

## Risks

- Tests limited to MobileAutomationRows would miss the shipped phone route.
- Adding failure to activity state would break running aggregation; adding a
  second phone summary hook would duplicate polling.
- Paused-gray is intentionally an exception to the issue's unqualified color
  language. Preserve visible failure metadata and record this in public docs.
- A newer skip/retry is the newest run even if an earlier run failed. Never scan
  backward to find a more alarming outcome or relabel the older run's age.
- Locale expansion and long names can squeeze suffixes. Verify actual rendering,
  not only CSS utility strings.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/office/requirements/automation-runs.md), AC-001.1,
  .9, .19 through .22.
- [Scoped design](../../specs/office/system-design/automation-latest-run-indicator.md),
  all sections; [base automation design](../../specs/office/system-design/automation-runs.md)
  for summary selection and unchanged refresh contracts.
- [Plan](plan.md), previews, behavioral test names and E2E matrix.
- Existing desktop AutomationsSection, active SavedMobileAutomationRows,
  useShortcutActivity, run-status mappings and sidebar/mobile E2E fixtures.
- Existing run-indicator and definition-refresh packages are historical context,
  not open dependencies or authorization to change their behavior.

## Results

Completed on 2026-11-05. All bounded work-order verification commands passed.
Runtime metadata confirms `openai-codex/gpt-6.1-sol`. No further agents or sessions
were created; source and documents remain unstaged and uncommitted.

### Acceptance delivered

- **AC-001.1/.19:** Sidebar and agenda consume one presentation helper and the
  authoritative newest run. Browser proof compares the same failed run ID and
  red status in history, then accepts a strictly newer success.
- **AC-001.9:** The mounted desktop sidebar recovers from failure without reload
  or row remount. The phone accepts the exact newer run while its existing drawer
  remains mounted and open; the page's `performance.timeOrigin` is unchanged.
- **AC-001.20:** Running wins over a newer skipped firing. Paused automation and
  paused schedule retain gray, with newest failure metadata still available.
  Never-run behavior and generic pinned-shortcut activity semantics are unchanged.
- **AC-001.21:** Desktop non-success text precedes the same run's age; success is
  age-only. Agenda keeps its next-firing datum and gets no last-run age. Seven
  complete locales and generated pseudo/Traditional Chinese catalogs are included.
- **AC-001.22:** The real saved-layout phone route exposes status with separate
  accessible link names/descriptions, preserves row-tap navigation and dismissal,
  wraps Portuguese retry-failure copy, and passes actual >=44px hit-target and
  document-overflow checks. The inactive fallback shares the domain row without
  activation or another poller.

The broad migrated requirement and base design remain draft. This delivery does
not promote unrelated automation contracts. Refresh owners, request/workspace
guards, backend status/order contracts and navigation preferences are unchanged.

### Exact verification and TDD evidence

Except for the initial install/RED commands, package commands used Node 24.22.1:
`env PATH=/home/clem/.npm/_npx/387698761821791d/node_modules/node/bin:$PATH ...`.
The exact scoped command lines are retained in Verification above.

- `cd apps && pnpm install --frozen-lockfile`: passed once, 1,162 packages.
  The initial Node 22 engine warning was resolved for subsequent checks by using
  Node 24; dependencies were not repeatedly installed.
- RED: `cd apps/web && pnpm exec vitest run components/app-sidebar/sections/automations-section.test.tsx components/runs/runs-list-page.test.tsx hooks/domains/sidebar/use-shortcut-activity.test.ts`:
  5 expected assertion failures, 48 passes (53 tests). Failures demonstrate the
  idle-green terminal failure, missing metadata and dropped newest run.
- RED: `cd apps/web && pnpm exec vitest run components/navigation/app-nav-sheet.test.tsx -t 'saved-layout automation row exposes'`:
  1 expected assertion failure, 32 skipped. The shipped composition exposed Idle
  rather than Failed. No missing-import/selector failure is counted as RED.
- The final nine-file `pnpm exec vitest run ...` command: **148 passed, 9 files**,
  after complete typed Automation/AutomationRun fixture refinement.
- `pnpm run typecheck`: passed, including `rich-output:check`.
- The exact fourteen-file `pnpm exec oxlint -c oxlint.config.mjs --deny-warnings ...`
  command: passed without warnings. Additional lint of both changed browser specs
  and `e2e/helpers/automation-summary-waits.ts` passed.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, `pnpm run i18n:check`: passed.
  All six translated locales are complete; 13,659 English messages and pseudo
  are synchronized. The 604 orphan-key warnings are informational.
- `make fmt`: passed. Its unrelated SDK-release JSON whitespace change was restored
  to the original content. Focused Prettier checks and final fixture/spec formatting
  passed.
- `scripts/run-quiet e2e --summary -- env CAPTURE_PR_ASSETS=1 PATH=/home/clem/.npm/_npx/387698761821791d/node_modules/node/bin:$PATH pnpm --dir apps/web e2e:run --project chromium tests/automations-sidebar.spec.ts -- --retries=0`:
  **6 passed (1.2m)**; `/tmp/kandev-run.e2e.Tr9CMTjm.log`.
- `scripts/run-quiet e2e --summary -- env CAPTURE_PR_ASSETS=1 PATH=/home/clem/.npm/_npx/387698761821791d/node_modules/node/bin:$PATH pnpm --dir apps/web e2e:run --project mobile-chrome tests/layout/mobile-navigation-hierarchy.spec.ts -- --retries=0`:
  **5 passed (1.1m)**; `/tmp/kandev-run.e2e.4NXGX9p9.log`.
  Both managed commands built fresh assets, ran sequentially with one worker and
  retries disabled, and exercised the complete specified files.
- `python3 scripts/list-docs.py validate`: 431 decisions and 1,921 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 55 pages validated.
- The exact local `validateCoverage` invocation above: `covered`, `ok: true`,
  no errors. `git diff --check -- apps/web docs/specs/office docs/plans/automation-latest-run-indicator docs/public/automation-and-mcp.md`:
  passed.

### Runtime and screenshot evidence

Inspected all three previews against actual rendering. No structural mismatch
remains: desktop status precedes age, agenda next firing stays primary, and phone
rows keep their disclosure, navigation and single scroller. Long names can
truncate while outcome text wraps; Paused and Failed occupy separate short lines
on the phone.

Final `prCapture` files, with desktop assets preserved across the sequential
managed runs and a combined manifest:

- UI-01: `apps/web/.pr-assets/automations-sidebar--automation-latest-failure-desktop.png`
- UI-02: `apps/web/.pr-assets/automations-sidebar--automation-latest-failure-agenda.png`
- UI-03: `apps/web/.pr-assets/mobile-navigation-hierarchy--automation-latest-failure-phone.png`
- UI-03, Portuguese: `apps/web/.pr-assets/mobile-navigation-hierarchy--automation-latest-failure-phone-pt.png`

Also launched the freshly built backend/SPA against disposable data under
`/tmp/automation-indicator-smoke.u7SJzY`, on port 48439. A managed
`browser.open({ app: { relay: false } })` tab exercised desktop, narrow agenda,
and the actual phone drawer. At 393x851, ordinary row height was 44 CSS px;
paused/long-label rows were 47.40625px, with Portuguese metadata 31.40625px high
and equal status `scrollWidth`/`clientWidth`. Document width stayed 393px.
The browser tab and owned backend service were closed. The public explanation
was updated only after runtime proof.

### Earlier attempts and extra broad-check limitations

- The initial browser RED attempt stopped before execution because the new ACP
  SDK was absent and the Go proxy refused connections
  (`/tmp/kandev-run.e2e.Qssd967u.log`). Direct Git transport retrieved the exact
  version; module/archive hashes match checked-in `go.sum`. No dependency files
  changed. Component assertions supplied behavioral RED.
- A cold managed desktop attempt reached the outer 1,200-second command deadline
  during its last scenario. The final fresh-build run above passed with no retries.
- Phone test-only corrections kept the reviewed contract: compare drawer identity
  rather than individual link nodes (existing definition-loading guards may
  replace rows), return to `/tasks` for its menu entry point, and scope the locale
  cookie to `/`. No production behavior or test timeout was changed. Capture
  framing scrolls the existing navigation scroller to show the whole group.
- Extra `make typecheck test lint` failed at classic TypeScript's default 4-GB
  heap ceiling. With `NODE_OPTIONS=--max-old-space-size=8192`, all-app typechecking
  passed, but backend tests failed in unchanged areas including routing probes,
  runtime installation, process lifecycle, configuration, launcher, updates and
  SQLite. Evidence is retained in `artifact://45`; test/lint are not claimed green
  for that broad command.
- Extra `pnpm --dir apps/web test`: **3,246 files passed, 47 failed (3,293 total);
  29,880 tests passed, 155 failed, 4 skipped (30,039 total)**. Failures are outside
  the changed automation files and include missing `unzip`, unavailable Docker
  gateway and dialog/integration timeouts (`artifact://72`). No unrelated source
  was changed to force a broad green result.
- Extra `env GOMAXPROCS=2 make lint` reached the 900-second outer command deadline
  during backend `golangci-lint run ./...` and was terminated. Its later targets
  did not run; the exact task-specific frontend lint gate passed independently.
  No whole-repository green audit is claimed.


### Opus implementation review remediation (round 1)

Two medium findings accepted on 2026-11-07:

- F1: `AgendaRow` lets outcome metadata consume the automation name's width on
  phones. Add real-browser geometry coverage for scheduled-retry and cancelled
  outcomes at 360, 393, 767 and 768 px. Keep the name readable while metadata
  wraps, without changing ordering or adding a last-run age.
- F2: generating the Traditional Chinese catalogs overwrote unrelated reviewed
  `pluginTools*` copy. Restore those values to HEAD in zh-hk and zh-tw while
  retaining the new `runStatusWithAge` frame.

The optional label-consolidation suggestion has no observed behavioral defect
and is deferred to avoid an unrelated refactor. Both actionable findings are
fixed and verified:

- RED: `scripts/run-quiet e2e --summary -- env PATH=/home/clem/.npm/_npx/387698761821791d/node_modules/node/bin:$PATH pnpm --dir apps/web e2e:run --project mobile-chrome tests/layout/mobile-navigation-hierarchy.spec.ts -- --grep 'phone agenda preserves automation names' --retries=0`
  failed at the intended geometry assertion: Cancelled at 360 px left the name
  17.171875 px wide, below the 64 px readable-width floor.
  Log: `/tmp/kandev-run.e2e.Zz68ygLX.log`.
- Fix: the agenda name uses `min-w-24` instead of a zero minimum. Outcome metadata
  wraps before consuming the name. No status, schedule, ordering or refresh
  behavior changed.
- GREEN: `scripts/run-quiet e2e --summary -- env CAPTURE_PR_ASSETS=1 PATH=/home/clem/.npm/_npx/387698761821791d/node_modules/node/bin:$PATH pnpm --dir apps/web e2e:run --project mobile-chrome tests/layout/mobile-navigation-hierarchy.spec.ts -- --retries=0`:
  **6 passed (1.1m)** against a fresh managed build, one worker, retries disabled.
  Log: `/tmp/kandev-run.e2e.21LVYoa4.log`. The new case covers scheduled-retry and
  cancelled outcomes at 360, 393, 767 and 768 px, readable names, visible metadata,
  non-overlap and no document overflow.
- Desktop controls: `scripts/run-quiet e2e --summary -- env CAPTURE_PR_ASSETS=1 PATH=/home/clem/.npm/_npx/387698761821791d/node_modules/node/bin:$PATH pnpm --dir apps/web e2e:run --project chromium tests/automations-sidebar.spec.ts -- --retries=0`:
  **6 passed (1.4m)** against a fresh managed build after the shared agenda CSS fix.
  Log: `/tmp/kandev-run.e2e.1YUMWXPh.log`. Current desktop and phone captures were
  combined in `apps/web/.pr-assets/manifest.json`.
- `pnpm exec vitest run components/runs/runs-list-page.test.tsx`: **13 passed**.
- `pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/runs/runs-list-page.tsx e2e/tests/layout/mobile-navigation-hierarchy.spec.ts`: passed after formatting
  the new test with Prettier.
- `pnpm run i18n:check`: passed after the copy restoration; all complete locales
  and pseudo synchronized. JSON comparison against HEAD confirms zh-hk and zh-tw
  have only the `runStatusWithAge` addition and no altered pre-existing values.
- Isolated built-app `browser.open` smoke on port 48451 confirmed the same
  layout: name widths were 98 px at 360 px and 131 px at 393 px for both outcomes;
  767/768 px also passed with no horizontal document overflow. Inspected the
  actual phone agenda and the regression capture:
  `apps/web/.pr-assets/mobile-navigation-hierarchy--automation-agenda-readable-names-phone.png`.
  Owned browser/backend were closed and disposable runtime data removed.

Commands prefixed with `pnpm` above ran from `apps/web` using the same Node 24
PATH as the managed commands. The existing broad-suite limitations remain;
no unrelated failing suite was rerun or claimed green.

### Contributor PR review correction (2026-11-16)

Removed the remaining reference from AC .22 to the unrelated AC .14.
AC .22 still specifies the existing activity indicators and accessible state text.
This documentation correction changes no runtime behavior or architectural boundary.
The review used source inspection only, as requested.
No builds, tests, linters, or commit hooks ran for this correction.
