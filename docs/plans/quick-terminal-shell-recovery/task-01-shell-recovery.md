---
id: "01-shell-recovery"
title: "Add explicit same-tab shell recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-001
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-001.8
  - AC-UI-QUICK-TERMINAL-001.14
  - AC-UI-QUICK-TERMINAL-001.15
  - AC-UI-QUICK-TERMINAL-001.16
  - AC-UI-QUICK-TERMINAL-001.17
  - AC-UI-QUICK-TERMINAL-001.18
  - AC-UI-QUICK-TERMINAL-001.19
  - AC-UI-QUICK-TERMINAL-001.20
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
  - ../../specs/ui/system-design/quick-terminal-recovery.md
---

# Task 01: Add Explicit Same-Tab Shell Recovery

## Summary

Expose **Start new shell** on an exited or unavailable Quick Terminal.
Recover through the existing host-shell API while preserving descriptor identity.
Order lifecycle requests so an old shell cannot overwrite its replacement.

## In scope

- Add the recovery action to the existing lifecycle region.
- Extract the modal's terminal lifecycle owner into the domain hook named in the owned files.
  Keep its inputs narrow and share the same logic across viewports.
- Retire old mount callbacks. Drain prior PATCH requests.
  Verify the existing descriptor with a workspace list read.
- Reattach a live session discovered during that read, or start under the existing tab's stable key.
- Bypass new-tab descriptor creation for recovery and remount only the inner PTY generation.
- Retain pending ownership across presentation detach and existing explicit-close cleanup.
- Preserve the published generation and suppress descriptor/PTy startup while terminal DELETE is pending.
- Keep the lifecycle owner mounted across conversation-tab dismissal while terminal descriptors remain.
- Add translated action copy, touch sizing, progress announcements, and public recovery guidance.
- Add and run the plan's desktop and phone E2E scenario matrix with real worker-owned shells.
- Use TDD for the new interaction and request-ordering logic.

## Out of scope

- Backend production changes, schema changes, and new API contracts.
- Automatic restart, durable output, and agent-login changes.
- Rewriting historical delivery results or changing the tab order contract.

## Acceptance

1. The new view regression fails before implementation, then passes for both shell-loss states.
   Manual recovery keeps the same descriptor, sequence, position, and active selection.
2. Deferred-request tests prove duplicate suppression, failure/retry, old-write ordering, callback retirement, and pending close/dismiss behavior.
   One live sibling remains unaffected while the selected exited tab recovers.
3. Desktop and phone shell-loss scenarios follow UI-01 and UI-02 and produce executable output in the same tab.
   The action uses complete translations and preserves shared PTY behavior.
   All commands below pass before this work order becomes `done`.

## ASCII UI preview

Excerpt of [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview).
Applies to `001.15` through `001.20`.

```text
Desktop: Terminal exited.       [ Start new shell ]

Phone:   Terminal exited.
         [       Start new shell       ]

Pending: Connecting to terminal...
         [ Start new shell ] (disabled)
Failure: Terminal error: <current error>
         [ Start new shell ] (enabled)
```

Status precedes the action. Phone uses a full-width row inside the existing full-height dialog.
The action has a 44px minimum target on phones and coarse pointers.
Desktop uses a 28px button. The status region clears the on-screen keyboard.
The tab strip stays fixed and terminal output remains the scroll owner.
Copy and spacing are illustrative, and all rendered copy uses `t()`.

## Regression cases

- `quick-terminal-tab-view.test.tsx`: offers same-tab recovery for exited and unavailable terminals without starting automatically.
  Include an exited tab that still carries the previous session ID.
- `use-quick-terminal-lifecycle.test.ts`: reuses the same descriptor and adopts a session recovered by another client.
- `use-quick-terminal-lifecycle.test.ts`: coalesces repeated activation and permits explicit retry after failure.
- `use-quick-terminal-lifecycle.test.ts`: drains an old lifecycle write before replacement and ignores retired callbacks.
  Hold the request itself, not only its response handler.
- `use-quick-chat-modal-terminal-lifecycle.test.ts`: preserves a live sibling and unchanged tab selection/order during recovery.
- `use-quick-chat-modal-terminal-lifecycle.test.ts` and `quick-terminal-tab-view.test.tsx`: a held DELETE during preparation preserves the generation and cannot create a descriptor or mount/start a PTY.
- `use-quick-chat-modal-terminal-lifecycle.test.ts`: a failed DELETE retains its generation so a later shell exit is persisted.
- `quick-chat-provider-lifecycle.test.tsx`: an exited PATCH remains ordered across terminal-to-conversation switching, dismissal, reopen, and recovery.
- View and hook tests: close during preparation cannot recreate the descriptor or start a shell.
  Close during startup stops the late shell.
  Dismissal or a tab switch permits later reattachment.
- `quick-chat-sync.test.ts`: a stale unavailable read cannot replace the current connecting attempt.
- StrictMode coverage: effect replay keeps one effective PTY under the same client key.

## E2E implementation

Create `quick-terminal-recovery.spec.ts` and `mobile-quick-terminal-recovery.spec.ts` under `e2e/tests/terminal/`.
Use the plan's complete [scenario matrix](plan.md#e2e-tests).
Before production edits, run the new recovery regressions and record their expected failures.

Use `backend.restart()` from `fixtures/test-base`.
It preserves the disposable worker database while replacing backend memory.
One case reloads after restart. Another keeps the browser open, confirms the
changed backend generation, and uses the required reload action before recovery.
Do not create a replacement tab or invoke recovery from a wait helper.

For natural exit, send `exit` through the active shell and observe its persisted exit.
Before recovery, record its tab ID, sequence, and session ID.
After recovery, inspect the same row and assert a distinct running session.
Use `printf 'RECOVERED%s\\n' '_SHELL'` and require `RECOVERED_SHELL` in output.
The complete marker cannot appear in echoed command input.

For the mixed-state case, keep one sibling alive with an environment marker while another exits.
Recover only the exited tab and assert that the sibling keeps its session and marker.
Do not use a full backend restart for that case because it would kill both shells.

For failure/retry, intercept the targeted host-shell start once and return a controlled error.
Hold the request to exercise repeated activation, then release the failure.
Assert visible error and no automatic retry.
A later explicit activation must reach the real backend and produce usable output.

Use the configured mobile project and `.tap()` from the actual phone menu entry point.
Measure `boundingBox()` and hit-test the recovery control inside its dialog.
Exercise 767px and 768px fine-pointer viewports and `tabletTestPage`.
Reuse the existing mobile terminal suite's visual-viewport fixture for keyboard occlusion.
Capture the idle and recovered phone surfaces through `prCapture`.

Scope buffer readers to the selected terminal panel.
Arm causal HTTP waits before stimuli and poll the real API for persisted readiness.
Use no arbitrary sleeps.
Restore intercepted routes and close every surviving descriptor in `finally`.

## Verification

Run from the repository root. In a fresh worktree, install dependencies first:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

The dependency install is unnecessary if this worktree already completed it.
The implementation checks are:

```bash
(cd apps/web && pnpm exec vitest run components/quick-chat/quick-terminal-tab-view.test.tsx components/quick-chat/use-quick-chat-modal-terminal-lifecycle.test.ts components/quick-chat/use-quick-chat-modal.test.ts components/quick-chat/quick-chat-modal.test.tsx components/quick-chat/quick-chat-provider-lifecycle.test.tsx components/quick-chat/quick-chat-provider.test.ts components/quick-chat/quick-chat-provider-focus.test.tsx hooks/domains/session/use-quick-terminal-lifecycle.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/state/slices/ui/quick-chat-actions.test.ts components/settings/pty-terminal-view.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/quick-chat/quick-terminal-tab-view.tsx components/quick-chat/use-quick-chat-modal.ts components/quick-chat/quick-chat-modal.tsx hooks/domains/session/use-quick-terminal-lifecycle.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
node scripts/validate-public-docs.mjs
(cd apps/web && pnpm e2e:run --project chromium tests/terminal/quick-terminal-recovery.spec.ts tests/terminal/quick-terminal.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-quick-terminal-recovery.spec.ts tests/terminal/mobile-quick-terminal.spec.ts -- --retries=0)
(cd apps/backend && go test -trimpath ./internal/quickterminal/... ./internal/agent/loginpty/... -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/quick-terminal-shell-recovery
```

Run the named view regression alone before production edits to record the expected red result.
Run each new E2E recovery spec alone before production edits to record its red result.
Record the full successful targeted suite after implementation.
Managed E2E runs rebuild the frontend and backend and tear down their workers.
Run the desktop and mobile commands sequentially.
Record actual test counts, viewport sizes, screenshots, and descriptor cleanup.
The local documentation preflight uses this exact command:

```bash
node <<'NODE'
const fs = require('node:fs');
const validator = require('./.github/scripts/pr-docs.cjs');
const files = [
  'docs/plans/quick-terminal-shell-recovery/plan.md',
  'docs/plans/quick-terminal-shell-recovery/task-01-shell-recovery.md',
  'docs/specs/ui/requirements/quick-terminal.md',
  'docs/specs/ui/system-design/quick-terminal.md',
  'docs/specs/ui/system-design/quick-terminal-recovery.md',
];
const result = validator.validateCoverage({
  changedFiles: [
    { filename: 'apps/web/components/quick-chat/quick-terminal-tab-view.tsx', status: 'modified' },
    { filename: files[1], status: 'added', additions: 1, changes: 1 },
  ],
  fileContents: Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
process.exitCode = result.ok ? 0 : 1;
NODE
```

After all criteria pass, mark the work order done, the plan implemented, and the recovery design current.

## Files likely touched

Production and unit-test ownership:

- `apps/web/components/quick-chat/quick-terminal-tab-view.tsx`
- `apps/web/components/quick-chat/quick-terminal-tab-view.test.tsx`
- `apps/web/components/quick-chat/quick-chat-provider-lifecycle.test.tsx`
- `apps/web/components/quick-chat/use-quick-chat-modal.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.test.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal-terminal-lifecycle.test.ts` (new)
- `apps/web/components/quick-chat/quick-chat-modal.tsx`
- `apps/web/components/quick-chat/quick-chat-modal.test.tsx`
- `apps/web/hooks/domains/session/use-quick-terminal-lifecycle.ts` (new)
- `apps/web/hooks/domains/session/use-quick-terminal-lifecycle.test.ts` (new)
- `apps/web/lib/state/slices/ui/quick-chat-sync.test.ts`
- `apps/web/e2e/tests/terminal/quick-terminal-recovery.spec.ts` (new)
- `apps/web/e2e/tests/terminal/mobile-quick-terminal-recovery.spec.ts` (new)
- `apps/web/e2e/tests/terminal/quick-terminal-recovery-helpers.ts` (new, only shared setup and assertions)

Translation and public-document ownership:

- `apps/web/src/locales/en/sidebar.json`
- `apps/web/src/locales/pt-pt/sidebar.json`
- `apps/web/src/locales/zh-cn/sidebar.json`
- `apps/web/src/locales/zh-tw/sidebar.json` (generated)
- `apps/web/src/locales/zh-hk/sidebar.json` (generated)
- `apps/web/src/locales/ja/sidebar.json`
- `apps/web/src/locales/ko/sidebar.json`
- `apps/web/src/locales/pseudo/sidebar.json` (generated)
- `docs/public/developer-tools.md`, Quick Chat/Quick Terminal how-to section.

Existing code used as input, with no expected production edit:

- `apps/web/components/settings/pty-terminal-view.tsx`
- `apps/web/components/settings/pty-terminal-lifecycle.ts`
- `apps/web/components/quick-chat/use-quick-chat-close-actions.ts`
- `apps/web/lib/api/domains/quick-terminal-api.ts`
- `apps/web/lib/state/slices/ui/quick-terminal-actions.ts`
- `apps/web/lib/state/slices/ui/quick-chat-sync.ts`

## Dependencies

None. The existing descriptor and host-shell APIs already support the required binding.

## Risks

- A delayed PATCH can overwrite the server binding even if the client ignores its response.
- The old PTY mount retains its initial session ref unless recovery replaces that mount.
- Detached requests must retain tab ownership without stopping a sibling or recreating a closed tab.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/quick-terminal.md), `001.8`, `001.14` through `001.20`.
- [Recovery flow and attempt ownership](../../specs/ui/system-design/quick-terminal-recovery.md).
- [Main terminal design](../../specs/ui/system-design/quick-terminal.md).
- Scoped frontend guidance and the mobile-parity, TDD, and docs-maintainer skills.
- E2E skill, including causal waits and worker cleanup.
- `apps/web/e2e/tests/terminal/quick-terminal.spec.ts` and `mobile-quick-terminal.spec.ts`.
- `apps/web/e2e/tests/chat/quick-chat-restart-scroll.spec.ts`, existing restart fixture usage.
- `apps/web/e2e/tests/terminal/terminal-test-helpers.ts`, close semantics.

## Results

Implemented and validated on 2026-11-11.

### TDD evidence

- Before production edits, the permanent view regression failed for both an exited tab with its prior session ID and an unavailable tab with no session. Both failures were caused by the missing **Start new shell** action.
- The desktop recovery E2E baseline failed on the missing action across natural exit, backend restart, viewport, and retry scenarios. The phone baseline also failed at the missing recovery action.
- The new view and lifecycle regressions now pass. Lifecycle coverage includes duplicate activation, explicit retry, a held PATCH drained before descriptor read, stale callback retirement, close during preparation, adoption of another client's live shell, and persistence of later exit updates.
- Review-remediation coverage proves a failed DELETE preserves generation 0 for a later shell exit, a held DELETE fences descriptor creation and PTY mounting during preparation, and the provider keeps one lifecycle owner across terminal-to-conversation dismissal and recovery.

### Validation

- Targeted unit suite: **129 tests passed across 8 files**. This includes the Quick Terminal view, modal/provider, lifecycle owner, sync/actions, and shared PTY lifecycle suites.
- `pnpm run typecheck`: passed.
- `pnpm exec oxlint -c oxlint.config.mjs --deny-warnings ...` for the four production files listed above: passed.
- `pnpm run build:vite`: passed. Vite reported chunk-size and ineffective dynamic-import warnings.
- `pnpm run i18n:zh-hant` and `pnpm run i18n:pseudo`: passed; Traditional Chinese and pseudo entries were generated.
- `pnpm run i18n:check`: passed. All seven catalogs are complete, pseudo is in sync, and 4,424 guarded files were scanned.
- `pnpm run i18n:ratchet`: passed with 0 added and 5 modified source files clean; the guard allowlist remained intact.
- Desktop managed E2E: **9 passed**. Seven recovery cases covered natural exit, restart plus reload, restart with the page open, live-shell adoption, sibling isolation, target sizing, and failure/retry. Two existing Quick Terminal tests also passed.
- Desktop sizing used **767×900** and **768×900** fine-pointer viewports, a **1280×900** desktop viewport, and the **900×900** coarse-pointer tablet fixture.
- Phone managed E2E: **3 passed**, including the recovery case and both existing phone Quick Terminal tests. The recovery case ran on the Pixel 5 viewport (**393×727**) and tested keyboard clearance with a simulated 300px visual-viewport reduction.
- Capture-enabled phone E2E: **1 passed**. `prCapture` produced `.pr-assets/mobile-quick-terminal-recovery--quick-terminal-recovery-phone-idle.png` and `.pr-assets/mobile-quick-terminal-recovery--quick-terminal-recovery-phone-running.png`; both images were inspected.
- Phone recovery explicitly closed its tab and waited for the tab count to reach zero. Desktop `finally` cleanup closed all remaining Quick Terminal tabs; each managed worker was disposable and torn down after its suite.
- Backend tests: `go test -trimpath ./internal/quickterminal/... ./internal/agent/loginpty/... -count=1` passed.
- `node scripts/validate-public-docs.mjs`: 55 published documentation pages validated.
- `python3 scripts/list-docs.py validate`: 449 decisions and 1,985 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local public-document coverage preflight: `covered`, zero errors.
- `git diff --check`: passed.

### Consolidated PR CI: confirmed backend restart

Shard 11 failed the open-browser restart case in all four attempts. Its saved
page showed the required reload alert and an old-document request failure.
The isolated old case passed. It did not wait for the global restart check
before attempting terminal recovery.

The test now waits for the new backend identity, dismisses the modal, and uses
the visible reload action. It reopens the terminal through the sidebar without
another navigation. Existing tab ID, order, new session, and command-output
assertions remain. This follows the platform restart contract; product code
and published screenshot surfaces are unchanged.

The corrected case passed, all seven recovery scenarios passed, and the exact
42-case CI worker group passed. Each run disabled retries. Public documentation,
catalog validation, specification lint, and diff checks passed. Hosted results
are recorded on PR #5321 after the correction is published.

Public guidance now explains same-tab recovery, the lack of automatic restart on tab selection, the loss of the old shell state, and when to use **New Terminal**.

### Review remediation verification

The two P1 review findings were fixed and revalidated on 2026-11-11:

- Targeted frontend suite: **135 tests passed across 11 files**, including the provider lifecycle deferred-PATCH regression and close-fence cases.
- `pnpm run typecheck`: passed.
- Changed-file Oxlint for the Quick Terminal lifecycle, provider, modal, view, and close-action files: passed.
- `pnpm run build:vite`: passed. Vite retained its existing dynamic-import and chunk-size warnings.
- Desktop managed E2E: **9 passed**; the seven recovery and two Quick Terminal cases passed.
- Phone managed E2E: **3 passed**; recovery and both existing phone Quick Terminal cases passed.
- `python3 scripts/list-docs.py validate`: 449 decisions and 1,985 specifications validated; `python3 scripts/lint-spec-files.py --all`: passed.
- Review-comment sibling-isolation E2E rerun: **1 passed**. It now compares the sibling's original session ID and reads the environment marker set before recovery.
- PR documentation coverage preflight: `covered`, zero errors, including both changed work orders.
- `git diff --check`: passed.
