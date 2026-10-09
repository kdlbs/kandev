---
id: "01-browser-runtime"
title: "Browser runtime and release compatibility"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-BROWSER-DEMO-001
acceptance_criteria:
  - AC-UI-BROWSER-DEMO-001.1
  - AC-UI-BROWSER-DEMO-001.2
  - AC-UI-BROWSER-DEMO-001.3
  - AC-UI-BROWSER-DEMO-001.4
  - AC-UI-BROWSER-DEMO-001.5
  - AC-UI-BROWSER-DEMO-001.6
  - AC-UI-BROWSER-DEMO-001.7
  - AC-UI-BROWSER-DEMO-001.8
  - AC-UI-BROWSER-DEMO-001.9
system_design:
  - ../../specs/ui/system-design/browser-demo.md
---

# Task 01: Browser runtime and release compatibility

## Summary and scope

Retain the browser demo behavior and adapt its fixtures and release steps to current `main`.
Include transport installation, scenario data, workspace responses, workflow operations, system data, and release assets.
Exclude production backend changes, real agent execution, and external integration connections.

## Acceptance

- Demo fixtures satisfy current shared types and pass focused protocol tests.
- The release workflow retains upstream verification and publishes a size-checked demo archive with its checksum.
- Both associated PRs have no rebase conflicts and pass their available CI gates.

## ASCII UI preview

Use [UI-01 in the plan](plan.md#ui-01-demo-task-session) for the task session and its phone composition.
The existing task, plan, workspace, and terminal controls remain authoritative. Criteria .2, .3, and .7 apply.

## Files and dependencies

- `apps/web/lib/browser-demo/`: transport, seed data, and simulated runtime modules.
- `apps/web/src/main.tsx`: installation before boot.
- Task and settings components: ready icons and the development entry-point action.
- Locale catalogs: translated entry-point labels.
- `.github/workflows/release.yml` and `scripts/browser-demo/`: archive distribution.
- Landing PR #169: separate archive consumption and deployment.

## Verification

Run from the repository root:

```bash
pnpm --dir apps/web exec vitest run lib/browser-demo components/task/task-item-ready.test.tsx components/settings/system/feature-toggles-settings.test.tsx
pnpm --dir apps/web typecheck
pnpm --dir apps/web lint
pnpm --dir apps/web i18n:check
pnpm --dir apps/web i18n:ratchet
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning.py
./scripts/browser-demo/build-web-demo.sh /tmp/kandev-browser-demo
git diff --check
```

## Results

The October 5 rebase removed conflicts in both repositories.
Focused verification passed 54 tests across 10 files. Typecheck, translation checks, and 48 release-workflow tests passed.
The demo production build passed and produced a 13,576,609-byte compressed archive, below the 25 MiB limit.
Landing passed 117 tests and its combined production build. Its fresh Cloudflare Pages check passed.
Kandev full-suite CI and local lint remain pending at this revision. No local demo server started during this rebase.

The Pages preview audit found a missing demo bundle and a new Automations sidebar contract.
The worker now returns the expected arrays and envelopes for six automation read actions.
The standalone demo build now runs prebuild to generate release data in fresh checkouts.
Landing PR #169 adds a pinned source fallback and rejects exports without the demo assets.
The focused worker and installation tests passed 24 tests. Typecheck and focused lint passed.
Browser verification rendered the demo task board from the production export without a local server.
The paginated sidebar now uses the application's existing filter, grouping, and ordering evaluator.
Its responses include full task records for selection and status rendering.
Conversation subscriptions now acknowledge protocol v2 and publish ordered message changes to their scopes.
The audit approval fixture includes the request identity required by the current chat controls.
The scenario version increases so persisted demo sessions use the updated fixture.
Seeded messages use increasing timestamps so the chat renderer preserves conversation order.
The final runtime checks passed 54 tests across nine files, typecheck, and focused lint.
Headless Chrome verified task navigation and a live approval response without a local server.
Landing checks confirmed that hero links do not overlap the video at six desktop and phone widths.

The latest preview check found new Jira dashboard and repository discovery routes without demo responses.
The worker now serves ticket filters and transitions, discovery refreshes, and repository-set reads locally.
Follow-up chat sends retain history and simulate thinking, a file read, and a varied answer before idle review.
Direct and queued sends deduplicate retries and serialize turns. Reset cancels stale callbacks.
New incident-response tasks retain their own workflow steps during completion.
Focused tests passed 66 tests across 12 files. Typecheck, focused lint, and the translation ratchet passed.
Headless Chrome verified direct and queued replies, Jira ticket filtering, and both discovered repositories in the new-task picker.
The production demo build passed. No local server or real agent ran for these checks.

The October 9 PR fixup rebased onto current main without conflicts or a changed head.
CI artifacts showed an unbound terminal descriptor interrupting a local start.
Terminal mutations now invalidate older workspace snapshots, and reconnect reads preserve pending starts.
HTML preview tests now reset committed assets before and after each test.
The diff continuity test checks that only its two fixture files enter the viewer.
Focused unit checks passed 148 tests across 17 files, including demo and reconnect regressions.
The ordered browser checks passed five mobile tests and seven desktop tests without retries.
After the final frontend rebuild, both terminal browser tests also passed without retries.
Current-head CI and automated review remain pending until the remediation push is verified.

Fixup verification commands, run from the repository root:

```bash
pnpm --dir apps/web exec vitest run lib/browser-demo lib/state/slices/ui/quick-chat-sync.test.ts lib/state/slices/ui/quick-chat-actions.test.ts hooks/use-quick-chat-resync.test.ts
E2E_PORT_OFFSET=29 pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/workflow/mobile-queued-session-ownership.spec.ts e2e/tests/workflow/mobile-workflow-peer-resume.spec.ts e2e/tests/chat/mobile-inbox-failed-tab.spec.ts e2e/tests/chat/mobile-last-prompt-scroll.spec.ts --retries=0
E2E_PORT_OFFSET=29 pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/chat/html-preview.spec.ts e2e/tests/git/diff-refresh-continuity.spec.ts e2e/tests/terminal/quick-terminal.spec.ts --retries=0
E2E_PORT_OFFSET=29 MAKEFLAGS=GOFLAGS=-buildvcs=false pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/terminal/quick-terminal.spec.ts --retries=0
```
