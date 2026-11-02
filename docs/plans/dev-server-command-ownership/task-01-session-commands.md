---
id: "01-session-commands"
title: "Own pending commands by session"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.17
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.18
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 01: Own pending commands by session

## Summary and acceptance

Both controls subscribe to the same outstanding command for the selected
session. Navigation, settlement and unmount retain originating ownership;
independent stores remain isolated. Preserve API returns, process freshness and
panel placement. Prove behavioral RED before production edits, then GREEN.

## In scope

- Real store/consumer/HTTP regressions and small transient command owner.
- Scoped lint, typecheck, managed browser integration and docs checks.

## Out of scope

Backend ordering, cancellation, new lifecycle authority, mobile header and
process status redesign.

## Files likely touched

- `apps/web/lib/state/dev-server-commands.ts`
- `apps/web/hooks/domains/session/use-dev-server-preview.ts` and `.test.ts`
- `apps/web/components/task/dev-server-preview-button.test.tsx`
- `apps/web/components/task/dev-server-preview-ownership.test.tsx`
- `apps/web/e2e/tests/session/dev-server-preview-toggle.spec.ts`

## Dependencies

None. Follow [the owning plan](plan.md) and its linked system design.

## Inputs

Owning navigation criteria `.5`, `.17`, `.18`, parent reproduction, current
hook/button tests and isolated dev-server E2E fixture.

## ASCII UI preview

UI-01: Existing desktop Dockview headers, selected session B while A starts.

```text
Chat header:      [Start preview: enabled for B]
Terminal header:  [Start preview: enabled for B]
After B starts:   [both disabled until B settles]
Return to A:      [both disabled while A is pending]
```

Only enabled/pending state changes. Existing icons, copy and panel grouping are
retained. Phone/tablet retain their focused chat/files/terminal composition;
the Dockview control is absent. The mobile-parity state/data exception applies.
Spacing is illustrative; no new layout or scrolling rules.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-dev-server-preview.test.ts components/task/dev-server-preview-button.test.tsx components/task/dev-server-preview-ownership.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/session/use-dev-server-preview.ts lib/state/dev-server-commands.ts hooks/domains/session/use-dev-server-preview.test.ts components/task/dev-server-preview-button.test.tsx components/task/dev-server-preview-ownership.test.tsx e2e/tests/session/dev-server-preview-toggle.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --host --project chromium e2e/tests/session/dev-server-preview-toggle.spec.ts --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use installed Node 24 through the shared build guard; all managed processes
must be terminal before the parent checkpoint. PR-documentation coverage is
checked against the committed head with the repository helper.

The full UI-01 preview is in [the plan](plan.md#ascii-ui-preview).

## Risks

Do not retain settled entries or couple independent provider stores.

## Parallelism

`sequential`

## Results

- Frozen workspace install: passed with pnpm 9.15.9.
- Initial real-consumer RED: 11 failed / 2 passed; expected ownership assertions,
  no fixture/import errors. A strengthened same-tick Start-to-Stop assertion
  separately failed (completion count 1 while the admitted request was held).
- Final focused Vitest command: 3 files, 29 passed (2026-10-21).
- Scoped ESLint: passed with zero warnings after rewriting three formatted test
  expressions flagged by `no-unexpected-multiline`.
- Node 24 typecheck with an 8 GiB heap: passed.
- Catalog and full specification lint: passed. The design extension was kept
  within the existing file-size limit; no baseline waiver was added.
- PR documentation coverage preflight: covered, no errors; exact-head recheck
  follows the normal hooked commit.
- Managed desktop browser command: passed, 2 Chromium tests / 1 worker,
  retries disabled, fresh Go/Vite/plugin builds. Real startup/output/stop and
  synthetic-HTTP two-visible-header navigation both passed (16.1 seconds).
  Log: `/tmp/kandev-run.e2e.hRq527hm.log`. The synthetic scenario proves
  rendered control/navigation ownership, not backend process startup; the
  separate real-process scenario proves startup, output and stopping.
- Mobile: state/data exception, no new mobile E2E. Actual phone/tablet alternate
  paths were inspected and do not consume this hook. Existing composition and
  saved-preview restoration are unchanged.
- Working-tree diff check and package status/untracked-file inspection: passed.
  Parent validation later identified committed trailing whitespace, corrected below.
- Initial normal hooked commit: see the parent handoff's exact receipt.

## Parent validation and delivery authorization

The parent inspected every changed path at the implementation head and
independently ran the same three focused suites: 29 tests passed (1.99 seconds),
terminal exit 0. No product blocker was found. Its full base-to-head whitespace
check found an extra blank line at the end of the work order; this documentation
cleanup removes it. A working-tree-only check had not detected the committed
whitespace. Subsequent diff checks use the complete base-to-head range.

The parent separately authorized push, template-based PR publication, exact-head
CI and substantive semantic review, scoped remediation, and ordinary protected
squash merge after the gates pass. No bypass, admin merge, status/provider or
workflow change is authorized. Delivery evidence remains external until its
exact final head and current-base merge result are verified. Parent retains
archive, accounting and next-selection ownership. No product suites were repeated
solely for this documentation correction; capture and merge-result validation
remain separate delivery checks.
