---
id: "02-wire-editor-availability"
title: "Wire active-session editor availability"
status: done
wave: 2
depends_on: ["01-add-session-capability"]
plan: "plan.md"
spec: "../../specs/ui/requirements/embedded-vscode-executor-availability.md"
---

# Task 02: Wire Active-Session Editor Availability

## Acceptance

- The desktop task topbar filters `internal_vscode` with the active session's backend capability;
  missing capability data fails closed without hiding unrelated editors.
- Saved-default fallback and active-session changes use the filtered editor set without mutating
  the stored preference or leaking the previous session's value.
- The obsolete boot `runtime.hostOS` field, parser/accessor, tests, and Playwright helper are
  removed with no remaining consumer.

## Verification

Use TDD: update the availability and prop-flow tests to describe the capability contract, observe
the failures, wire the active-session value, and remove host metadata only after the new path is
green. From `apps/` run:

```bash
pnpm --filter @kandev/web test -- src/boot-payload.test.ts components/task/editors-menu-availability.test.ts components/task/task-top-bar.test.tsx
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint
```

From `apps/backend/` run the affected boot package tests:

```bash
go test ./internal/backendapp ./internal/webapp
```

Finally search from the repository root:

```bash
rg -n "readBackendHostOS|hostOS|rewriteBackendHostOS" apps
```

The search must return no obsolete editor-availability or boot-contract references.

## Files likely touched

- `apps/web/hooks/domains/session/use-session-resumption.ts`
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/task-top-bar.tsx`
- `apps/web/components/task/task-top-bar.test.tsx`
- `apps/web/components/task/editors-menu.tsx`
- `apps/web/components/task/editors-menu-availability.ts`
- `apps/web/components/task/editors-menu-availability.test.ts`
- `apps/web/src/boot-payload.ts`
- `apps/web/src/boot-payload.test.ts`
- `apps/backend/internal/webapp/payload.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/web/e2e/helpers/boot-payload.ts` (remove)

## Dependencies

- Task 01 provides the session capability and enforcement contract.

## Parallelism

Sequential. The frontend contract and metadata removal must land together before E2E coverage.

## Inputs

- Spec sections: **What**, **API surface**, **Failure modes**, and scenarios 1–6 and 8.
- Plan sections: **Consume active-session capability**, **Filter and fallback**, **Retire
  host-wide boot metadata**, and **Mobile design contract**.
- Existing active-session source:
  `resumption.sessionStatus` in `apps/web/components/task/task-page-inner.tsx`.

## Risks

- Reset capability to false when `effectiveSessionId` changes instead of retaining asynchronous
  status from the previous session.
- Pass one capability value to both halves of the split editor control.
- Keep custom, hosted, Remote SSH, and installed built-in editors governed by their current rules.

## Output contract

Report the prop/data flow, removed host metadata, files changed, red and green results, and the
repository-wide search result. Update this task to `done` and its plan checkbox/status.

## Completion record

- The current session's `capabilities.embedded_vscode` flows from session resumption through
  `TaskPageInner`, `TaskTopBar`, `TopBarRight`, `TopbarToolsGroup`, and `EditorsMenu`. Missing
  data is false, while the hook's ID-keyed state prevents a previous session's status leaking into
  the next session.
- Availability filtering now accepts the capability boolean and retains existing enabled/installed
  checks. The filtered set continues to drive saved-default fallback without persisting a change.
- Removed boot `runtime.hostOS` from Go serialization and TypeScript parsing, plus the old E2E
  HTML-rewrite helper.
- Red: the updated availability test still used the old host-OS argument and failed before the
  filtering implementation changed. Green: focused Vitest tests passed (15 tests),
  `pnpm run typecheck` and `pnpm run lint` passed, and
  `go test ./internal/backendapp ./internal/webapp` passed.
- `rg -n "readBackendHostOS|rewriteBackendHostOS|hostOS" apps/web apps/backend/internal/webapp apps/backend/internal/backendapp -g '*.ts' -g '*.tsx' -g '*.go'` returned no matches.
