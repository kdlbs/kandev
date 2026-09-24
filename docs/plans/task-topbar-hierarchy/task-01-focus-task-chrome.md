---
id: 01-focus-task-chrome
title: Focus task chrome
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-TASK-TOPBAR-001
acceptance_criteria:
  - AC-UI-TASK-TOPBAR-001.1
  - AC-UI-TASK-TOPBAR-001.2
  - AC-UI-TASK-TOPBAR-001.3
  - AC-UI-TASK-TOPBAR-001.4
  - AC-UI-TASK-TOPBAR-001.5
system_design:
  - ../../specs/ui/system-design/task-topbar-hierarchy.md
---

# Task 01: Focus task chrome

## Summary and scope

Implement the desktop hierarchy and touch disclosures in the owning header,
tools, assignment, and metric components. Update direct tool-entry E2E paths and
public workbench guidance. Exclude backend behavior and unrelated page chrome.

## Acceptance

- Header identity remains readable; metrics and tools open complete disclosures.
- Existing assignment and workspace controls preserve their outcomes.
- Desktop, coarse-pointer, and phone checks pass with seeded screenshot proof.

## ASCII UI preview

UI-01 and UI-02 from the [plan](plan.md#ascii-ui-preview):

```text
Desktop: repo > Title [Workflow] [Metrics] [Plugins] [Avatar] [PR] [Panels] [Tools] [...]
Phone:   [Menu] [Task v] [Step v] [...]   -> existing native drawers
Tools:   Layout [picker] / Workspace [editor picker] [folder action]
```

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/task-top-bar.test.tsx components/task/task-assignee-control.test.tsx components/task/layout-preset-selector.test.tsx components/task/task-chrome-disclosure.test.tsx)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/layout/task-topbar-hierarchy.spec.ts tests/layout/task-topbar-long-title.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/task/mobile-task-topbar-long-title.spec.ts tests/settings/mobile-resource-metrics-display.spec.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also run each changed existing E2E file, focused lint on changed sources, and
record exact commands/results. Managed runs own isolated backend/data teardown.

## Files likely touched

- `apps/web/components/task/task-top-bar*.tsx`
- `apps/web/components/task/task-assignee-control.tsx`
- `apps/web/components/task/task-chrome-disclosure.tsx`
- `apps/web/components/task/layout-preset-selector.tsx`
- `apps/web/e2e/tests/{layout,task,settings}/`
- `docs/public/developer-tools.md`

## Dependencies and parallelism

None. Sequential execution.

## Risks

See the plan's nested-picker and touch-target risks.

## Results

- Focused Vitest command above: 4 files, 32 tests passed.
- Scoped ESLint, TypeScript, i18n, whitespace, specification validation, and
  public-documentation validation passed. Specification-validator tests: 36;
  public-doc-validator tests: 62.
- 39 distinct desktop/touch scenarios passed across the serial runs below and
  the touch review follow-up.
  The new regression first failed with inline layout controls still visible;
  another failed with Task tools remaining open after editor launch. Both now pass.
  Existing layout tests now reopen Task tools after applying a preset. The helper
  allows the parent disclosure to remain outside the accessibility tree while a
  nested modal picker is open.
- Four phone scenarios passed with the mobile command above: long-title layout,
  detailed Status metrics, menu fallback metrics, and simplified metrics.
- The existing full code-server startup scenario could not complete on this host:
  the cached `code-server-4.96.4-linux-amd64/bin/code-server --version` exits 139.
  The editor toolbar scenario passes with an explicit supported Local executor
  and embedded editor preference; its prior defaults selected an unsupported
  mock executor and the host's external editor. No production editor policy or
  code-server installation was changed.

Exact desktop runs (from `apps/web`, after `pnpm --filter @kandev/web build:e2e`
from `apps/`):

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium tests/layout/task-topbar-hierarchy.spec.ts tests/layout/task-topbar-long-title.spec.ts tests/task/open-task-folder.spec.ts tests/task/editors-menu-worktree-picker.spec.ts tests/task/executor-embedded-vscode-availability.spec.ts tests/layout/right-panel-visibility.spec.ts tests/layout/saved-layout-session-isolation.spec.ts tests/layout/changes-panel-focus.spec.ts tests/settings/vscode-open-panel.spec.ts tests/settings/layout-profiles.spec.ts tests/layout/task-topbar-workflow-stepper.spec.ts -- --grep-invert 'opens VS Code panel in the center group' --retries=0
pnpm e2e:run --host --shards 1 --no-build --project chromium tests/layout/task-topbar-hierarchy.spec.ts tests/layout/saved-layout-session-isolation.spec.ts tests/settings/vscode-open-panel.spec.ts -- --grep-invert 'opens VS Code panel in the center group' --retries=0
pnpm e2e:run --host --shards 1 --no-build --project chromium tests/layout/saved-layout-session-isolation.spec.ts -- --grep 'saved chat-only layouts' --retries=0
```

The first run passed 37 scenarios and identified the saved-layout test's need to
reopen Task tools. The second passed five and identified the helper's nested
modal accessibility lookup. The final focused run passed the remaining scenario.
The second run also covers saving a new layout through the nested dialog.

Public documentation checks from the repository root:

```bash
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
```

Six fresh screenshots passed visual inspection: desktop (1440 x 900), compact
desktop (1000 x 800), Task tools, System metrics, wide touch (1280 x 900), and
native Pixel 5 phone. The fictional Northstar workspace was seeded through real
APIs with a prepared disposable worktree, assigned user, coherent conversation,
and mock GitHub PR. The advertised mock-agent Opus (1m) model keeps all agent
execution isolated. Sidebar transitions were allowed to settle before capture.
The fixture removed its temp root and released its backend port. Raw images,
compressed delivery images, capture source, hashes, and provenance are retained
in `/tmp/kandev-topbar-evidence.pelfn5`; PR images publish only on an orphan media
ref, outside this branch's mergeable tree.

Temporary capture command from `apps/web` (harness removed after capture):

```bash
TOPBAR_CAPTURE_ROOT=/tmp/kandev-topbar-evidence.pelfn5 CAPTURE_PR_ASSETS=true pnpm e2e:run --host --shards 1 --no-build --project auth tests/auth/task-topbar-capture.spec.ts -- --retries=0
```


## Review verification

Added four real-primitive disclosure tests covering desktop/touch selection,
uncontrolled dismissal, controlled state ownership, and desktop focus entry and
restoration. The browser regression now opens both disclosures with Enter and
checks dialog focus, Tab into the layout selector, and Escape focus restoration.
Radix FocusScope supplies `tabIndex=-1` to the rendered popover content; the
reported missing-focus regression does not reproduce.

The focused Vitest command above passes all 32 tests. The exact browser command
is below and passes both scenarios. Changed-file ESLint and TypeScript pass.
The editor response guard now documents the hook's null-on-failure contract.
This first follow-up changed tests, documentation, and one production comment.

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium tests/layout/task-topbar-hierarchy.spec.ts -- --retries=0
```

The aggregate CodeRabbit review identified undersized portaled options on wide
touch screens. The new browser assertion reproduced the layout option failure.
Layout options, editor options, submenu triggers, and nested worktree choices
now have 44px minimum targets on coarse pointers. The editor group sizes to its
buttons instead of clipping them; the browser check verifies their bounds fit
inside its border. Fine-pointer sizing stays unchanged. No shared menu primitive
or unrelated menu was changed.

The two updated hierarchy scenarios and both desktop/touch multi-worktree
scenarios pass (four tests). Six layout-profile scenarios also passed. The
32 unit tests, TypeScript, and changed-file ESLint pass. The aggregate review's
generic docstring-coverage suggestion is optional: no additional comments are
needed for these self-describing controls or tests.

All six screenshots were captured again after the touch fix and visually
checked. Updated raw/delivery assets and teardown proof are retained in
`/tmp/kandev-topbar-evidence.pelfn5/touch-review`.

```bash
pnpm e2e:run --host --shards 1 --no-build --project chromium tests/layout/task-topbar-hierarchy.spec.ts tests/task/editors-menu-worktree-picker.spec.ts tests/settings/layout-profiles.spec.ts -- --retries=0
pnpm e2e:run --host --shards 1 --no-build --project chromium tests/layout/task-topbar-hierarchy.spec.ts tests/task/editors-menu-worktree-picker.spec.ts -- --retries=0
```
