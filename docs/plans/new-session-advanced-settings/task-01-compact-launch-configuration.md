---
id: "01-compact-launch-configuration"
title: "Compact new-session advanced settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NEW-SESSION-ADVANCED-001
  - REQ-UI-NEW-SESSION-ADVANCED-002
acceptance_criteria:
  - AC-UI-NEW-SESSION-ADVANCED-001.1
  - AC-UI-NEW-SESSION-ADVANCED-001.2
  - AC-UI-NEW-SESSION-ADVANCED-001.3
  - AC-UI-NEW-SESSION-ADVANCED-001.4
  - AC-UI-NEW-SESSION-ADVANCED-002.1
  - AC-UI-NEW-SESSION-ADVANCED-002.2
  - AC-UI-NEW-SESSION-ADVANCED-002.3
  - AC-UI-NEW-SESSION-ADVANCED-002.4
system_design:
  - ../../specs/ui/system-design/new-session-advanced-settings.md
---

# Task 01: Compact new-session advanced settings

## Summary

Implement the New Task-style disclosure and compact MCP picker in New Agent and
handoff. Preserve session launch data and verify desktop, phone and coarse tablet.

## In scope

Creation form composition, overlay ownership, accessible interactions, targeted
regressions, localization for any added text, and user-guide wording if affected.

## Out of scope

Backend/payload changes, existing-session exclusions/Customize, native packages,
profile editors, new persistent tasks, PR review/merge and unrelated UI cleanup.

## Acceptance

1. UI-02/03/04 match the reference hierarchy with bounded overlays and preserved drafts.
2. Actual creation sends the selected additions; no disclosure/picker action launches
   or persists existing-session state. Existing controls still work.
3. All targeted checks pass; desktop/phone captures and coarse-pointer results
   are recorded. Update paired specs to active/current only after completion.

## UI preview

[Full preview](plan.md#ui-preview): UI-02 is Profile, Context, Prompt,
Advanced settings, actions. UI-03 adds only MCP servers/count beneath that
label. UI-04 opens its selection list in a touch sheet. These views cover all
AC-001 criteria and AC-002.2/3; state/error controls cover AC-002.1/4.

## Verification

Run from repository root, sequentially. Install dependencies before package
commands if this checkout is fresh. New test paths below are implementation outputs.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/new-session-advanced-settings.test.tsx components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.test.ts components/task-create-dialog-advanced-settings.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host -- tests/session/new-session-advanced-settings.spec.ts tests/session/new-session-dialog.spec.ts tests/task/task-mcp-selection.spec.ts --project=chromium --workers=1 --retries=0)
(cd apps/web && pnpm e2e:run --host -- tests/session/mobile-new-session-advanced-settings.spec.ts tests/session/mobile-new-session-dialog.spec.ts tests/task/mobile-task-mcp-selection.spec.ts --project=mobile-chrome --workers=1 --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The new desktop suite also exercises coarse-tablet input; the mobile suite uses
real touch interactions. Add a bounded existing-session Customize regression
from the current MCP suite if the shared selection body changes. Use isolated
runner data, never the user's main instance. Record all failures and actual
counts. Run PR documentation coverage against final changed files before delivery.

## Files likely touched

- `apps/web/components/task/new-session-dialog.tsx`
- `apps/web/components/task/new-session-advanced-settings.tsx` (new)
- `apps/web/components/task/new-session-mcp-picker.tsx` (new; shared selector unchanged)
- `apps/web/components/task/new-session-advanced-settings.test.tsx` (new)
- The named E2E specs and `apps/web/e2e/pages/new-session-dialog-page.ts`
- Locale files only if existing translations cannot express session-specific copy
- Relevant public guide and this package's Results

## Dependencies

None beyond current main containing the existing MCP creation picker. No stack
rebase or sibling implementation is a prerequisite. Preserve any later shared
changes when integrating.

## Risks

Nested overlay focus/portals, accidental draft reset, and shared-content scope.

## Parallelism

`sequential`

## Inputs

Paired requirements/design; New Task advanced settings and MCP row; scoped web
AGENTS.md; mobile-parity, e2e and tdd skills before implementation.

## Results

Implementation uses the New Task ghost disclosure below the prompt and recovery
feedback. A compact MCP row opens a bounded popover on desktop and a Drawer on
phone or coarse tablet. The form retains prompt, attachment, profile, context and
explicit-selection state. The launch contract is unchanged. The public session
guide describes the new entry point.

The branch was fast-forwarded to main at
`6a1a2ae10003c6bccc754ab144c6035ac38158d1`. The shared selector also serves Task
Edit, so the new creation-only picker reuses its selection body without changing
that selector, New Task or existing-session Customize.

### Test evidence

The focused component tests first failed
because the Advanced settings control did not exist, then passed after the
presentation change. Browser checks exposed two additional defects: the dialog
clipped the popover at its edge, and zero safe-area padding let the touch list
extend below the Drawer surface. Pointer-hit and scroll-boundary assertions
failed before their respective fixes and now pass.

Browser coverage includes keyboard search/selection, Enter without submission,
Escape/focus return, collapse/reopen, cancel/reset, inherited origins, explicit
launch selections, attachments, handoff profile and Blank context, one scroll
owner, long names, 320px pseudo-locale, narrow fine-pointer phone and coarse tablet.
Loading and empty definitions use a deferred API fixture. Disabled choices,
busy controls and unavailable access are covered by component tests. A creation
browser fixture for unavailable access is not claimed: the existing creation
controller does not expose an error value to the selector.

Desktop, phone, 320px pseudo-locale and coarse-tablet captures were inspected. Local copies are retained under
`/tmp/new-session-advanced-captures/`; tests also write captures to their
Playwright output directories.

### Final checks

- Targeted Vitest command after PR review: **48 tests passed across 4 files**.
- Required desktop command: **20 passed**, one worker, zero retries. Log:
  `/tmp/kandev-run.e2e.BAhCbyIh.log`.
- Required mobile command: **13 passed**, one worker, zero retries. Log:
  `/tmp/kandev-run.e2e.r6YjVCi5.log`.
- Final web typecheck with `NODE_OPTIONS=--max-old-space-size=8192`: **passed**.
- Full web lint and `pnpm run i18n:check`: **passed**. Existing translations are reused.
- Catalog validation: **403 decisions and 1742 specifications passed**. Full
  specification lint: **passed**.
- Public-doc validation: **54 pages passed**; validator tests: **63 passed**.
- PR documentation coverage against the final changed files: **passed**, covering
  all three production components. `git diff --check` and whitespace checks for
  new files: **passed**.

The desktop set contains the new Advanced settings suite, the existing New
Session suite and Task MCP selection. The mobile set contains their mobile
counterparts. These suites include existing-session Customize and New Task
regression controls. All 33 browser tests passed against the implementation
build before the PR review corrections below.

Final browser commands used the verification paths above with
`KANDEV_SERVER_HOST=127.0.0.1`, `E2E_PORT_OFFSET=26` and a process-local
`LD_LIBRARY_PATH` for the installed Chromium binary. The mobile run rebuilt the
app; the following desktop run reused it with `--no-build`. Mobile project
selection was passed as `--project mobile-chrome` before the test arguments.

### Local runner setup and corrected failures

- Dependencies installed with `pnpm install --frozen-lockfile` from `apps/`.
- Default 4GB typecheck exhausted the Node heap. The 8GB rerun exposed one
  missing fixture field, which was corrected. The following 8GB check passed.
- Full lint exposed repeated test labels. Constants replaced those literals;
  the next full lint passed.
- Initial browser runs failed before application checks because the isolated
  backend inherited a non-loopback bind address, then Chromium lacked NixOS
  shared libraries. Per-command loopback binding and existing Nix library paths
  resolved these issues without changing the user's runtime configuration.
- One test setup hit a port collision. Subsequent runs use checked port offset 26.
- Early test assertions used the wrong Context role and a profile's absent agent
  ID. These fixtures now use the Context combobox and the discovered mock agent.
  The coarse-tablet test opens New Agent through the command panel because that
  layout does not contain the desktop tab-add control.
- Runs use one worker, zero retries and isolated runner data. Rebuilt artifacts
  are reused with `--no-build` only when production sources have not changed.

### PR review corrections

CodeRabbit's aggregate review identified two valid accessibility issues. The
trigger now includes the translated selected count in its accessible name.
The Enter guard allows native IME candidate confirmation for both `isComposing`
and key code 229. The visible layout and creation contract are unchanged.

The existing selection test now checks accessible names before and after a
selection. Two input regressions check IME confirmation, ordinary Enter
containment, and an explicit launch positive control. All three new assertions
failed before the fix. The targeted Vitest command above then passed all 48
tests across four files. The affected-file ESLint check, full web typecheck
(8GB Node heap), and full specification lint also passed. Remote CI and
automated review verification are pending.
