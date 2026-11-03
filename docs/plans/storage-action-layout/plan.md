---
created: 2026-11-02
status: implemented
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001
system_design:
  - ../../specs/system-page/system-design/storage-action-layout.md
legacy_specs: []
---

# Implementation plan: Storage action layout

## Overview

Keep Analyze and Run now readable and contained when the Settings sidebar or
a narrow screen reduces horizontal space. One work order adds a rendered
regression, changes the action layout, and checks desktop and phone behavior.

Implementation was authorized on 2026-11-03. Work order 01 owns the change
and its targeted verification in the primary session.

## Intent and ownership

Confirmed: the user requested a fix for the two Storage buttons at constrained
widths. Verified: viewport-based `sm` layout rules combine with fixed button
widths, and disabled tooltips introduce an intrinsic-width wrapper.

Agent design choice: use available section width to place the action group.
Use full-width stacked actions on phones. These are reversible presentation
choices within the requested scope. No material question blocks this package.

System-page owns the maintenance actions. The existing storage requirement
file is 20,386 bytes, close to its 20 KiB limit. This package isolates the
action-layout extension under the same owner instead of enlarging that file.

## Scope

### In scope

- Available-width composition for the top-level Analyze and Run now controls.
- Matching widths in enabled and disabled states, including tooltip wrappers.
- Wrapped localized state labels and existing desktop/touch control sizing.
- Focused rendered regressions and comparison with the previews below.

### Out of scope

- Cleanup policy, permission, API, confirmation, and job lifecycle changes.
- Other Storage cards, navigation, global styles, and shared size contracts.
- New copy, translations, animations, dependencies, and persistent preferences.

## Technical approach

Implement the [system design](../../specs/system-page/system-design/storage-action-layout.md)
in `StorageActions` and its existing page root. A named inline-size container
selects the combined or separate explanation/action rows. The 768px phone
boundary controls the single-column phone composition.

Add an opt-in `fullWidth` prop to `StorageActionButton` so its disabled-reason
wrapper fills the assigned cell. Keep other consumers at their current width.
Preserve all handlers and existing selectors. Add selectors for the action row,
explanation, and action group only if geometry checks need them.

Use existing icons, variants, and sizing helpers. Allow tall labels to increase
the button height while retaining standard minimums. Do not introduce new
motion or change focus and disabled feedback.

The test-only helper `apps/web/e2e/helpers/storage-actions-layout.ts` supplies
shared geometry checks and disposable job responses for both browser projects.

## ASCII UI preview

The previews show structure. Spacing is illustrative. Existing catalog copy
supplies every rendered label. These views map to criteria 001.1 through 001.11.

### UI-00: Current constrained desktop, idle

Entry: Settings > System > Storage > Host. Source and the supplied screenshot
show that the buttons can exceed their shrinking grid cells.

```text
Explanation text            [ Analyze ........][ Run now .......>
                            button exceeds cell / section edge
```

### UI-01: Proposed wide section, idle

Section width is at least 52rem. The explanation and action group share a row.

```text
Reclaim disk space safely          [ Analyze ]   [ Run now* ]
Analyze for a read-only snapshot,
or run the enabled cleanup rules...
```

### UI-02: Proposed constrained section, idle

Below 52rem, actions follow the explanation. Above the phone boundary, two
equal columns fit when the section is at least 24rem wide. Below 24rem they
become separate rows.

```text
Reclaim disk space safely
Analyze for a read-only snapshot,
or run the enabled cleanup rules...

[          Analyze          ]   [          Run now*          ]
```

### UI-03: Proposed phone, idle

Below 768px, each action has a separate full-width row and a 44px minimum target.

```text
Reclaim disk space safely
Analyze for a read-only snapshot,
or run the enabled cleanup rules...

[                Analyze                ]
[                Run now*               ]
```

### UI-04: Proposed phone, job and disabled states

```text
[             Analyzing...              ]
[                Run now*               ]

[           Analysis complete           ]
[           Cleanup complete*           ]

[       Longer localized status         ]
[       can wrap within this button     ]
```

The asterisk annotates the primary treatment and is not rendered copy.
Disabled wrappers must fill the same rows as enabled controls. Running, failed,
and member-disabled states keep this structure. The existing Settings page
scrolls vertically. No region becomes fixed or gains a separate scroller.

## Tests

Use Playwright geometry as the regression evidence for markup and CSS.
Do not add class-string tests. Existing component tests protect operation
semantics without duplicating the browser geometry checks.

| Acceptance | Evidence |
| --- | --- |
| 001.1, 001.2 | New desktop layout spec measures the section, copy, cells, controls, gap, and page overflow at narrow and wide widths. |
| 001.3 | State tests check readable localized labels through running, terminal, and permission-disabled states. |
| 001.4 | Mobile geometry checks measure phone and coarse-pointer targets at least 44px. |
| 001.5 | Screenshot inspection checks the primary Run now and outline Analyze treatments. |
| 001.6 | Label geometry checks measure unclipped text and sufficient control height at narrow widths. |
| 001.7 | Disabled-state geometry checks measure button and wrapper width against their allocated cell. |
| 001.8 | Desktop geometry checks measure single-line height at 28px. |
| 001.9 | The keyboard case checks action order, focus retention, and both mocked requests after resizing. |
| 001.10 | The phone case activates both mocked operations with taps after resizing. |
| 001.11 | The phone case measures separate full-width rows with Analyze first. |
| 001.12 | Desktop and phone state cases check stable translated button names. |
| 001.13 | The desktop and phone cases check one live region per action. A DOM marker proves that the same region survives job and viewport updates. |
| 001.14 | The state cases check announced failure and completion text outside stable button names. |

All shortened criterion numbers refer to
`REQ-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001`.

## E2E tests

- `apps/web/e2e/tests/system/storage-actions-layout.spec.ts`, project `chromium`:
  `fits actions inside a constrained Storage section`,
  `contains job labels and disabled wrappers`, and
  `retains keyboard actions after resizing`.
- `apps/web/e2e/tests/system/mobile-storage-actions-layout.spec.ts`, project
  `mobile-chrome`: `stacks actions with usable touch targets` and
  `retains action containment on a coarse-pointer tablet`.
- Retain and run the existing desktop and phone maintenance specs. They cover
  Analyze results, busy feedback, Run anyway, and resource cleanup behavior.

Use the configured Pixel 5 context for phone tests. A tablet case changes
viewport dimensions within that existing coarse-pointer context. A separate
chromium case checks phone-width behavior with a fine pointer. Do not override
device settings or add a Playwright project.

## Work orders

- [x] [Task 01: Make Storage actions fit their section](task-01-responsive-actions.md)

One sequential work order owns implementation and targeted verification.
No delegation is authorized.

## Verification

Run the exact implementation checks in the work order after implementation.
The managed E2E commands build current production assets and run one project
per invocation. Run desktop and mobile commands sequentially.

The design-package checks are:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/system-page docs/plans/storage-action-layout
git status --short -- docs/specs/system-page docs/plans/storage-action-layout
```

Run the local documentation coverage preflight from the repository root.
It uses the prospective component changes as triggering paths. It checks
cross-references without creating a PR or writing to GitHub.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/storage-action-layout/plan.md',
  'docs/plans/storage-action-layout/task-01-responsive-actions.md',
  'docs/specs/system-page/requirements/storage-action-layout.md',
  'docs/specs/system-page/system-design/storage-action-layout.md',
];
const fileContents = Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')]));
const changedFiles = [
  ...paths.map(filename => ({ filename, status: 'added', additions: 1, changes: 1 })),
  { filename: 'apps/web/components/settings/system/storage/storage-maintenance-settings.tsx', status: 'modified' },
  { filename: 'apps/web/components/settings/system/storage/storage-action-button.tsx', status: 'modified' },
];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Verification results

Design-package validation passed on 2026-11-02:

- Catalog validation accepted 425 decisions and 1,883 specifications. The two
  new system-page specifications appear in the filtered catalog.
- All 36 specification-linter tests passed. All specification files passed lint.
- The local documentation coverage preflight returned `covered` for work order
  01 with no reference errors.
- Scoped diff and new-file whitespace checks passed. Final status contains
  only the four new, unstaged design-package documents.

Implementation verification passed on 2026-11-03:

- All 127 Storage unit tests passed across 15 files. Lint, formatting,
  typecheck, and the i18n ratchet passed.
- All 13 desktop browser tests passed, including the three new layout,
  translated-state, and keyboard regressions.
- All nine phone/tablet browser tests passed, including the two new action
  layout cases. No retries were used in either project.
- Rendered narrow desktop, wide desktop, and phone views match UI-01 through
  UI-03. Completed and translated labels match UI-04. Images are saved under
  `.pr-screenshots/storage-action-layout/` for file previews.

The initial RED check measured about 65px of Analyze overflow at a 1024px
viewport. The same assertion passed after the production change. See the
[work-order results](task-01-responsive-actions.md#results) for commands,
environment details, and logs.

Final catalog validation, specification lint, documentation coverage preflight,
and diff checks passed. Work order 01 is done. Its requirement is active and
its system design is current.

PR review corrections split the acceptance criteria into individual behaviors
and aligned the verification commands with the passing local runs. The test
annotations and design mappings follow the expanded criteria. Documentation
validation and focused lint passed again. Production behavior did not change.

A later accessibility review found an English spinner label in translated
actions. The actions now retain stable translated names and expose translated
active statuses. Desktop and phone regressions passed, along with all 127
Storage unit tests, typecheck, lint, formatting, and language checks. The
work-order results record the failing assertion and passing commands. CI and
the requested review watch remain external delivery checks.

The final accessibility follow-up keeps the translated status in a persistent
region beside each button. Completion and failure remain available after the
spinner disappears. The Portuguese state loop checks the visible failure text
as well as the announcement. All 22 browser tests and 127 unit tests passed
again, with typecheck, lint, formatting, language, and documentation checks.
Only hidden announcements changed in this follow-up, so the published layouts
remain accurate. Remote CI and review completion are still pending.

A final test follow-up marks each idle live region before the first operation.
The browser cases verify that each marker survives job states and phone
resizes. All three desktop and two phone/tablet layout cases passed again.
The product code is unchanged from the terminal-status fix.

## Documentation impact

The docs-maintainer check found existing operator guidance in
`docs/public/operations.md` and a wide Storage screenshot in
`docs/screenshots.md`. The implemented layout preserves labels, navigation,
operation semantics, and wide composition. The rendered wide view retains
the documented information hierarchy. No public prose or screenshot refresh
is required for this focused layout correction. Internal specs and delivery
records contain the change.

## Risks

- Shared tooltip wrappers could widen other actions unless full width is opt-in.
- Responsive height variants can override a plain automatic-height class.
- Long translated status labels can require more height than idle labels.
