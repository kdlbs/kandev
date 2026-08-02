---
id: "04-toast-reporting"
title: "Toast reporting"
status: completed
wave: 3
depends_on: ["02-frontend-error-endpoint"]
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 04: Toast reporting

## Acceptance

- Sonner error toasts and legacy error creation/error transitions each send
  exactly one best-effort report with visible text and available rich browser
  context.
- Recognized task routes add their task ID to the report; non-task routes omit
  it without affecting the toast or report.
- Reporting success or failure does not change toast rendering, IDs, timing,
  ordering, dismissal, ARIA behavior, or responsive behavior.
- Toast rendering is handed to the original library before reporting is
  scheduled. The report is never awaited, uses a two-second deadline, and bounded
  context collection cannot create a noticeable synchronous serialization path.
- Application Sonner call sites use the reporting wrapper, and a focused
  source-scan test prevents new direct `sonner` toast imports.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- lib/api/domains/frontend-error-log-api.test.ts lib/toast/sonner.test.ts components/toast-provider.test.tsx
```

```bash
cd apps/web
pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/api/domains/frontend-error-log-api.ts`
- `apps/web/lib/api/domains/frontend-error-log-api.test.ts`
- `apps/web/lib/toast/sonner.ts`
- `apps/web/lib/toast/sonner.test.ts`
- `apps/web/components/toast-provider.tsx`
- `apps/web/components/toast-provider.test.tsx`
- Application files returned by
  `rg -l 'import \\{ toast \\} from "sonner"' apps/web --glob '*.{ts,tsx}'`
  (import-line-only migration to `@/lib/toast/sonner`)

## Dependencies

- Task 02 defines and registers the report endpoint.

## Parallelism

Sequential after Task 02 because the frontend client must implement the
approved backend contract.

## Inputs

- Spec: API surface, Failure modes, and toast-report Scenarios.
- Plan: Frontend sections.
- Frontend guidance: shared behavior only; no mobile composition change.
- Existing patterns: `fetchJson`, Sonner call sites, and `ToastProvider`.

## Risks

- Fire-and-forget handling must not produce unhandled promise rejections or
  recurse through console interception.
- Tests assert delegate-before-report call order for both toast systems.
- "Immediate" reporting must not become synchronous toast latency; slow or
  offline networking must leave the UI path unaffected.
- Rich context and generated stacks are sensitive and must remain bounded by
  the backend contract.
- Route parsing must accept only the documented task route shapes and must not
  mistake unrelated path segments for task IDs.
- Non-text React nodes must not crash serialization or suppress the original
  toast.

## Output contract

Report source coverage, preserved UI behavior, files changed, exact test
commands/results, blockers or risks, and update this task plus `plan.md` status
in the same conversation.
