---
id: "01-correct-dedicated-tunnel-links"
title: "Correct dedicated tunnel links"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PORT-FORWARDING-ACTIVE-FIRST-001
acceptance_criteria:
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.3
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.6
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.9
system_design:
  - ../../specs/ui/system-design/port-forwarding-active-first.md
---

# Task 01: Correct dedicated tunnel links

## Summary

Use the dedicated listener's HTTP scheme independent of the backend dashboard
scheme, while retaining the backend hostname and returned listener port. Keep
proxy URL construction and existing actions unchanged.

## In scope

- Independently author a real rendered list/action regression and observe the
  HTTPS scenario fail before changing production code.
- Correct the shared URL construction and document the two URL types in the
  existing public how-to.
- Record actual targeted test and validation results; update delivery status.

## Out of scope

Listener, transport, TLS, lifecycle, authorization, IPv6 feature work, port
input validation, translations, and responsive presentation changes.

## Acceptance

- HTTPS and HTTP backend configurations produce the HTTP dedicated URL with
  the backend hostname and returned tunnel port in row text, Open, and Copy.
- Proxy text, Open, Copy, and the proxy-only Browser action retain the backend
  scheme, host, port, base path, session, and target port.
- The public how-to accurately distinguishes dedicated HTTP from the proxy URL;
  desktop and phone continue sharing the same URL data and actions.

## Verification

After the separate implementation grant and exclusive heavy-work lease, install
once in this fresh worktree before package commands. Run the first targeted
component test before and after the fix; record the actual red and green results.
Run commands from the repository root, sequentially, and retain each exit.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task/port-forward-list.test.tsx components/task/port-forward-dialog.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/port-forward-list.tsx components/task/port-forward-list.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use normal repository hooks for any later authorized commit. Do not run heavy
commands concurrently with other work. Mobile Playwright is not added for this
shared data-only correction; see the [plan](plan.md#e2e-tests). Use real
rendered action elements and copy interception in the independent component test.

## Files likely touched

- `apps/web/components/task/port-forward-list.tsx`
- `apps/web/components/task/port-forward-list.test.tsx`
- `docs/public/tasks-and-workflows.md`
- This plan, work order, and linked owning requirement/design for delivery status.

## Dependencies

None.

## Risks

The backend may itself use a non-default port or base path. Tests must distinguish
that port from the dedicated port and retain the proxy path exactly.

## Parallelism

Sequential.

## Inputs

- [Active-first requirements](../../specs/ui/requirements/port-forwarding-active-first.md)
- [Active-first design](../../specs/ui/system-design/port-forwarding-active-first.md)
- `apps/backend/internal/gateway/websocket/port_tunnel.go` and existing rendered
  port-forwarding test conventions.

## Results

2026-10-27, after the implementation release and exclusive lease:

- `(cd apps && pnpm install --frozen-lockfile)`: exit 0.
- `(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 components/task/port-forward-list.test.tsx)`: exit 1 before the fix, with HTTPS failing on the expected dedicated HTTP URL and the HTTP control passing.
- `(cd apps/web && pnpm exec vitest run --maxWorkers=1 components/task/port-forward-list.test.tsx components/task/port-forward-dialog.test.tsx)`: exit 0, 12 tests passed across two files.
- `(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/port-forward-list.tsx components/task/port-forward-list.test.tsx)`: exit 0.
- `(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)`: exit 0. The bounded allocation was explicitly authorized based on a prior default-heap failure in another worktree; this result does not claim a default-heap pass.
- `(cd apps/web && pnpm run i18n:check)`: exit 0; reported 557 existing orphan catalog entries.
- `python3 scripts/list-docs.py validate`: exit 0, 408 decisions and 1760 specifications.
- `python3 scripts/lint-spec-files.py --all`: exit 0.
- `git diff --check`: exit 0 before the final result was recorded; rerun at staging.
