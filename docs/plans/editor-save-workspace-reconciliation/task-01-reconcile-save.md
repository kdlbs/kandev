---
id: "01-reconcile-save"
title: "Reconcile save receipts with workspace observations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
acceptance_criteria:
  - AC-UI-FILE-EDITOR-MUTATION-001.3
  - AC-UI-FILE-EDITOR-MUTATION-001.10
system_design:
  - ../../specs/ui/system-design/file-editor-mutation-ownership.md
---

# Task 01: Reconcile save receipts

## Summary and scope

Preserve independent workspace observations while recording successful saves.
Own `use-file-save-delete.ts`, `file-editors-sync.ts`, `FileEditorState`, their
regression tests, one focused desktop E2E, and related documentation. No backend,
panel markup, schema, API, or general action ordering changes.

## Acceptance

- New actual hook/store regression fails before correction and passes afterward.
- Snapshot, baseline/hash, dirty, echo and ownership invariants pass deterministically.
- Desktop Reload and separate phone Files flow pass against a fresh production build.

## Verification

Run from repository root with Node 24 on PATH:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/use-file-editors.mutation-ownership.test.tsx hooks/use-file-editors.save-sync.test.tsx hooks/use-file-save-delete.test.ts hooks/file-editors-sync.test.ts hooks/file-editors-sync.ownership.test.tsx components/task/file-editor-panel.workspace-sync.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-file-save-delete.ts hooks/file-editors-sync.ts lib/state/dockview-store.ts hooks/use-file-editors.mutation-ownership.test.tsx hooks/use-file-editors.save-sync.test.tsx hooks/file-editor-mutation.test-helpers.tsx e2e/tests/task/file-editor-save-refresh.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/file-editor-save-refresh.spec.ts tests/task/file-tree-create.spec.ts -- --grep 'keeps Reload|New file at root creates' --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-file-viewer.spec.ts -- --grep 'tapping file in All-files tab opens viewer')
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/plugins/conversation-recovery.spec.ts tests/task/mobile-task-transient-read-recovery.spec.ts -- --project=mobile-chrome --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Dependencies and parallelism

None. Sequential; no delegates.

## Risks and inputs

Use the owning requirement/design and plan's surface inventory. Preserve no-op
refresh behavior and existing LSP disk-versus-live handling. Failed operations
must leave remote state and baseline unchanged.

## Results

Completed. See [plan verification results](plan.md#verification-results) for the
RED/GREEN chronology, exact browser outcomes and limitations. The listed unit,
increased-heap typecheck, zero-warning lint, desktop/mobile E2E, spec, public-doc
and diff checks passed. The original desktop command performed a fresh managed
build; after correcting only its Save selector, the passing rerun added
`--no-build`. Shared test fixtures were extracted to stay within file-size limits.
Documentation coverage preflight returned `covered` with no errors.

Review remediation extends the same AC .10 with the real-original-hash alias
regression and explicit Worktree executor isolation with source commit/status
assertions; see the plan remediation record. The first remediation targeted unit count was
89. Fresh-build regression/capture/neighbor E2E passed 3 tests with retries off.

The acceptance-provenance extension adds six focused cases (final total 95),
keeps original-hash alias/echo controls, and extends the same desktop E2E with
Reload acceptance followed by a later warning and observable dirty-baseline proof.
Acceptance is transient per buffer and remains separate from latest observation.


CI remediation is limited to the two reported test assumptions: place the
mobile synthetic online event in a new foreground episode using Date control,
and validate plugin continuation identities rather than an exact row count
that excludes independent live projection. Production behavior and owning
requirements/design are unchanged. Reproduce the CI-selected shard partition
with one worker and retries disabled; keep final CI and review gates external.
