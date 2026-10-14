---
id: "01-expand-image-cards"
title: "Expand image cards by default"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001
acceptance_criteria:
  - AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.1
  - AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.2
  - AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.3
  - AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.4
  - AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.5
  - AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.6
system_design:
  - ../../specs/agents/system-design/image-preview-defaults.md
---

# Task 01: Expand image cards by default

## Summary

Apply the existing image classifier as the native card's disclosure default.
Preserve retained choices and target isolation, prove desktop/phone behavior,
and align the previously lazy-only contract and user documentation.

## In scope

- TDD regressions using real rich-output Renderer and only transport mocks.
- One initial-value change in `FilePreview` and existing image/read behavior.
- Shared deterministic image fixture and desktop/mobile browser scenarios.
- Parent specification, ADR and public guidance reconciliation at delivery.

## Out of scope

Other image surfaces, rendering/format changes, new settings or schema fields,
transport changes, and adjacent visual cleanup.

## Acceptance

1. Fresh image cards auto-expand/load in v1/v2; text remains lazy. A stored false
   choice survives metadata changes and retained remounts, with independent cards.
2. Image/text replacement isolates bytes and pending settlements; errors retry
   only after explicit reopening and successes reuse mounted content.
3. Desktop and phone render decoded images before a disclosure click, support
   hide/reopen and Open file, and preserve mobile containment and touch targets.

## ASCII UI preview

UI-01: Fresh image in the transcript, excerpt from the
[full preview](plan.md#ascii-ui-preview). Covers AC .1-.6.

```text
Desktop: [Image] shot.png  [Hide preview] [Open file]
         [            bounded image             ]
Phone:   [Image] shot.png
             [Hide preview] [Open file]
         [       bounded image       ]
Hidden:  shot.png [Preview] [Open file]
```

Reuse existing localized loading/error states inside the expanded region.
Phone actions wrap below 420px and retain at least 44px actual targets. The
transcript owns scrolling. These structures are required; spacing is illustrative.

## Verification

From repository root, run these commands sequentially. Managed browser runs
build fresh production assets. Do not overlap runs or override worker budgets.

```sh
(cd apps/web && VITEST_MAX_WORKERS=2 pnpm exec vitest run components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-view-state.test.tsx hooks/domains/session/use-workspace-file-preview.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/messages/kandev/rich-output/file-preview-block.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-view-state.test.tsx e2e/tests/chat/rich-output.spec.ts e2e/tests/chat/mobile-rich-output.spec.ts e2e/tests/chat/rich-output-helpers.ts)
PATH=/usr/local/go/bin:$PATH GOCACHE=/tmp/kandev-image-preview-go-cache GOMAXPROCS=2 GOFLAGS='-p=1 -gcflags=github.com/kandev/kandev/internal/task/repository/sqlite=-c=1' scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium tests/chat/rich-output.spec.ts
PATH=/usr/local/go/bin:$PATH GOCACHE=/tmp/kandev-image-preview-go-cache GOMAXPROCS=2 GOFLAGS='-p=1 -gcflags=github.com/kandev/kandev/internal/task/repository/sqlite=-c=1' scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/chat/mobile-rich-output.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The local environment requires Go on PATH and an isolated Go build cache after
a shared-cache linker crash. A package-specific compiler concurrency override
avoids a subsequent SQLite-package DWARF generation crash. The typecheck heap is raised within the 8 GiB
cgroup limit; run it separately from memory-heavy builds. Unit workers are
bounded to two after a default-worker crash.

If executing in a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)` once. Record behavioral RED before
production changes; capture the expanded phone state and compare with UI-01.

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/rich-output/file-preview-block.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-view-state.test.tsx`
- `apps/web/e2e/tests/chat/{rich-output,mobile-rich-output}.spec.ts`
- `apps/web/e2e/tests/chat/rich-output-helpers.ts`
- `apps/web/e2e/fixtures/rich-output-image.png`
- `docs/specs/agents/{requirements,system-design}/agent-rich-output.md`
- `docs/specs/agents/{requirements,system-design}/image-preview-defaults.md`
- `docs/decisions/2026-08-14-kandev-native-agent-rich-output.md`
- `docs/public/automation-and-mcp.md` (existing reference section)
- This plan and work order for results/status updates.

Reconcile parent AC 001.9/.14 and 002.4/.6/.11 to allow default image reads
without relaxing CSV snapshots, authorization, non-image laziness or callback
restrictions. Keep the parent design below its size limit by replacing obsolete
wording with a link to this design. Promote this draft pair only after checks pass.

## Dependencies

None. Read the paired requirement/design and shared rich-output baseline first.

## Risks

Do not add a repeated effect that defeats user collapse, change report/store
identity, or key an entire presentation by target. Keep existing text regressions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/image-preview-defaults.md)
- [System design](../../specs/agents/system-design/image-preview-defaults.md)
- Existing file-target and retained-view-state tests; `seedRichOutputTask`.

## Results

Completed on 2026-10-13.

- Behavioral RED: v1/v2 renderer tests failed with expected expanded true versus
  actual false. The fresh production-browser RED likewise failed on the actual
  card's `aria-expanded=false` before restoring the production default.
- The exact targeted Vitest command above passed 3 files / 32 tests after the
  final production edit. Only external workspace transport is mocked.
- `pnpm run typecheck` with the documented 6 GiB heap passed, including the
  generated rich-output schema check. The exact focused ESLint command passed
  without warnings after final test edits.
- Managed desktop command: 5 passed (33.6s), log
  `/tmp/kandev-run.e2e.LoC4DlXT.log`.
- Managed phone command: 3 passed (15.8s), log
  `/tmp/kandev-run.e2e.LCJarmvR.log`. The final run adds on-screen hide/open
  assertions and the reviewed `automatic-image-phone.png` screenshot. It matches
  UI-01: wrapped actions, expanded contained image, and collapsed text card.
- The final phone run intentionally used `--no-build`: the immediately preceding
  managed run freshly built backend and Vite assets, then failed while repacking
  the unrelated plugin fixture because a shared Go-module cache file was corrupt.
  The existing fixture archive and identity passed global-setup verification.
  Backend, plugin and frontend production sources did not change between these
  runs. An earlier fresh managed phone run also passed all 3 tests.
- Specification catalog and lint passed; public documentation validator tests
  passed 63 tests and public-page validation passed 48 pages. Documentation
  coverage preflight and whitespace checks passed.

Local tooling recovery: initial default-worker Vitest and 4 GiB typecheck
processes crashed; two unit workers and a 6 GiB heap completed. Go's initial
shared-cache linker crash and later compiler DWARF crash were bypassed with an
isolated cache and the package-specific serial compiler flag. The unreadable
hardlinked pnpm tree was moved to the ignored
`.cache/kandev-image-preview-corrupt/node_modules` directory; dependencies were
reinstalled from a fresh `/tmp` store using `--package-import-method=copy` with
no lockfile or dependency-version changes. No application workaround was added.

REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001 and AC .1-.6 are covered. The paired specs
are active/current. Parent lazy-only criteria, ADR and public reference were
reconciled. No unresolved implementation or design issue remains.

## PR integration follow-up

On 2026-10-14, merge-conflict remediation preserved both this package's link
and the current base's CSV-header-BOM repair link. The image-default production
change and regression tests remain unchanged. The targeted 32 tests, catalog,
specification lint, 19 harness-validator tests, full harness lint, and 36
specification-validator tests passed on the merged tree. Normal merge hooks
passed without bypass. Current-head CI and review completion are tracked
externally while the PR checks run.

Claude's aggregate review noted inconsistent deferred-read cleanup in the
image/text replacement test. Its success/failure variants now settle the old
request in `finally`, including when an earlier assertion fails. The exact
targeted Vitest command above passed 3 files / 32 tests, and targeted ESLint for
`rich-output-renderer-file-targets.test.tsx` passed. No production behavior or
durable contract changed, so no specification or public-doc amendment is needed.
Current-head remote checks and reviews remain pending after this test-only fix.

The follow-up aggregate review classified the repeated settlement as
informational and confirmed no change is needed. Its test-name suggestion was
addressed by renaming the existing text-target replacement case to
`collapses a text preview when its $field changes`. The exact targeted Vitest
command again passed 3 files / 32 tests, and targeted test-file ESLint passed.
Assertions and product behavior are unchanged; remote validation remains pending.

The subsequent review's image-target naming suggestion was addressed as
`loads only the new image target when its $field changes`. This describes the
path/repository/session identity represented by the parameterized cases. The
exact targeted command again passed 3 files / 32 tests, and targeted ESLint
passed. Only test wording changed; final remote validation remains pending.
