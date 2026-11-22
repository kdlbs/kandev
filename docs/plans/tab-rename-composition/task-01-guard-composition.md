---
id: TASK-TAB-RENAME-COMPOSITION-01
title: Keep composition keys inside the shared tab rename
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-EDIT-FOCUS-001
acceptance_criteria:
  - AC-UI-EDIT-FOCUS-001.3
  - AC-UI-EDIT-FOCUS-001.4
system_design:
  - ../../specs/ui/system-design/inline-edit-focus-continuity.md
---

# Guard tab rename composition

## Scope and files

Own `apps/web/components/task/tab-rename-input.tsx`, its test, extensions to
`use-session-rename.test.tsx`, the existing session/terminal browser rename
specs, and a browser helper if shared by those specs. Preserve other changes
and all foreign processes. Dependencies: frozen-lockfile install from `apps/`
and Node 24 on PATH before checks/hooks. No production test seams or mocked
coordination are needed.

## Acceptance

1. Composing Enter, legacy 229 Enter with `isComposing=false`, and composing
   Escape keep the edit open, do not persist/cancel, do not prevent default,
   and remain isolated from surrounding keyboard handlers.
2. After composition ends, a separate ordinary Enter commits exactly once.
   Ordinary Escape and internal blur, session Unicode boundaries and sibling
   isolation, and terminal default/reset behavior continue to work.
3. Real session and desktop/tablet terminal entry points pass browser checks;
   phone native picker compatibility remains covered by its unchanged suite.
   Report dispatched-event evidence separately from physical IME integration.

## Verification

From `apps/web` with Node 24 on PATH:

```bash
pnpm exec vitest run components/task/tab-rename-input.test.tsx components/task/use-session-rename.test.tsx components/task/terminal-tab.test.tsx components/task/session-tab-menu.test.tsx components/task/preview-session-tabs.test.tsx --maxWorkers=1
pnpm exec eslint --max-warnings 0 components/task/tab-rename-input.tsx components/task/tab-rename-input.test.tsx components/task/use-session-rename.test.tsx e2e/helpers/tab-rename-composition.ts e2e/tests/session/session-tab-rename.spec.ts e2e/tests/terminal/terminal-dockview-ui.spec.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

From `apps/`, managed runner, one worker/shard and no retries:

```bash
GOMAXPROCS=2 GOFLAGS=-p=2 ../scripts/run-quiet e2e --summary -- pnpm --dir web e2e:run --host --shards 1 --project chromium tests/session/session-tab-rename.spec.ts tests/terminal/terminal-dockview-ui.spec.ts --grep 'Session tab rename|Unicode names|composition|right-panel terminal context menu|right-click.*Rename swaps' --retries=0
GOMAXPROCS=2 GOFLAGS=-p=2 ../scripts/run-quiet e2e --summary -- pnpm --dir web e2e:run --host --shards 1 --no-build --project mobile-chrome tests/session/mobile-session-unicode-name.spec.ts --retries=0
cd ..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Evaluate `.github/scripts/pr-docs.cjs` `validateCoverage` using the exact final
changed-file records and file contents, without an override. Preserve a receipt.
Capture RED before production changes, then final GREEN against fresh assets.
Keep every returned process handle until its actual terminal result.

## Results

- RED: the two focused input/session suites had 7 intended failures and 24
  passing controls. Four IME cases prevented default; two Enter cases sent
  partial names; composing Escape cancelled. Both terminal browser consumers
  closed the input on composition Enter. The session browser observer was
  corrected to exclude non-gateway sockets, then reproduced the same input
  closure using the saved pre-fix assets.
- GREEN: all 86 tests across the five named Vitest files passed (5.27s).
  Fresh managed assets passed all 5 selected desktop/tablet browser checks
  (30.7s), including the complete 3-test session rename spec. The unchanged
  phone native-picker compatibility scenario passed (9.8s). All browser runs
  used one worker/shard and zero retries.
- Final ESLint, Node 24 typecheck with an 8192 MiB heap ceiling, i18n check
  and ratchet passed. Catalog validation, full spec lint, exact documentation
  coverage without override, and diff whitespace checks passed.
- Source hashes confirm the 11 traced consumer/phone compatibility sources
  match verified main exactly. Only the shared input's key guard changes
  production behavior. Phone screenshot inspected: native Sessions sheet,
  selected Unicode label, no inline rename input or document overflow.
- Receipt directory outside the checkout:
  `/home/jcfs/.kandev/tasks/keep-tab-renames-ope_8pc7mqcb/loop-receipts/attempt18/`.
  It contains causal logs, target PR patches/heads, source/asset hashes,
  package/full diff, checks, and normal commit hook/process receipts.

No rendered structure changes, so no ASCII layout preview is needed.
The final handoff requires a normal-hook Conventional Commit, clean head,
durable receipts outside the checkout, and `ready_for_parent_validation`.
Do not push or create a PR before separate parent authorization.
