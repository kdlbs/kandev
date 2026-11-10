---
created: 2026-10-21
status: done
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
legacy_specs: []
---

# Implementation Plan: Dev-server command ownership

## Overview

Repair session-owned Start/Stop pending state across retained controls and
navigation. The parent reproduced three behavioral failures using real buttons,
store and transport boundaries on base `2dca446f48b5e3b7cb81baa8a1d8cbeb4600dd27`.
Local pending booleans leak across selection; the module start map is not reactive.

## Scope and technical approach

Implement the design's small app-store-scoped transient command store, consumed
by `useDevServerPreview`. Preserve process freshness, failure returns and panel
placement. No backend, dependency, lifecycle cancellation, process generation,
mobile layout, generic coordinator or persistence change. This is local
implementation of an existing navigation boundary; the design preserves its
rationale, so no ADR is needed.

`lib/state/dev-server-commands.ts` uses a WeakMap keyed by the existing app store
and a small transient Zustand store containing only outstanding session promises.
Admission publishes before execution or subscriber reentry; all admitted callers
wait for cleanup, including Stop invoked before Start has returned a process ID.
No command queue, completed marker, timer or new process authority is retained.

The source audit also includes `PreviewController`/`usePreviewPanel`, mounted by
the desktop and `SessionTabletLayout` for saved-preview restoration. Those
requests are a separate existing path; the two-header admission contract does
not claim backend/global idempotency. Tablet's `BrowserPanel` and phone's focused
file-preview/terminal/port surfaces retain their existing behavior. No affected
hook consumer is present in those alternate compositions.

Public docs need no change: `docs/public/sessions-and-review.md` already describes
the same Start/Stop actions and panel behavior. Labels, navigation instructions,
configuration and API contracts are unchanged; this repairs pending ownership.

## Tests

Real-consumer `dev-server-preview-ownership.test.tsx` is the primary regression
owner for `.5`, `.17`, `.18`. Exercise held fetches through real process API and
store publication, including two controls and navigation. Existing hook tests
retain process timestamp checks; button tests retain panel-opening checks.

## E2E tests

Run the desktop `dev-server-preview-toggle.spec.ts` with a fresh managed build.
Extend it for held requests, navigation and both visible header controls if
reachable through its existing isolated fixture. Mobile needs no new browser
scenario because the control is absent and the fix only changes shared data.

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

## Work orders

- [x] [Task 01: Own pending commands by session](task-01-session-commands.md)

## Verification results

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


## Risks

An opposite command invoked while busy joins the admitted request without
queuing work, preserving the disabled button policy. HTTP/WS status ordering
and backend command execution retain their current limits. Browser tests prove
rendered integration separately from transport-held component regressions.
