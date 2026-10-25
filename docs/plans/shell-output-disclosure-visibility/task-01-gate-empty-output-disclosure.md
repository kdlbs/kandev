---
id: "01-gate-empty-output-disclosure"
title: "Gate empty output disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/acp-shell-command-output.md"
---

# Task 01: Gate empty output disclosure

## Acceptance

- Shell command rows with `has_output: false` and zero stdout/stderr byte
  counts show the command, working directory, and status without a `Show
command output` control or an on-demand output request.
- Shell command rows with `has_output: true` or positive byte counts preserve
  the existing collapsed disclosure and fetch output only after expansion.
- Desktop and mobile E2E coverage proves the no-output path while retaining
  the existing real-output and viewport behavior.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/tool-execute-message.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run --project chromium tests/chat/tool-execute-output.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-tool-execute-output.spec.ts
```

## Files likely touched

- `apps/web/components/task/chat/messages/tool-execute-message.tsx`
- `apps/web/components/task/chat/messages/tool-execute-message.test.tsx`
- `apps/web/e2e/tests/chat/tool-execute-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-tool-execute-output.spec.ts`
- `docs/plans/shell-output-disclosure-visibility/plan.md`

## Dependencies

None.

## Parallelism

Sequential. Production code, component coverage, and desktop/mobile coverage
share the same summary predicate and must be verified together.

## Inputs

- `docs/specs/ui/requirements/acp-shell-command-output.md`, especially the disclosure and
  no-output scenarios.
- `docs/plans/shell-output-disclosure-visibility/plan.md`.
- Existing `ShellOutputDisclosure` lazy-fetch behavior and the current desktop
  and mobile shell-output fixtures.
- Mobile parity contract in the plan: reuse the shared command row and make no
  new drawer or responsive surface.

## Output contract

Report the root cause addressed, files changed, the failing-then-passing
component regression, exact desktop/mobile E2E and typecheck results, any
artifacts or blockers, and synchronized task/plan status.

## Results

Implemented the shared command-row guard in `ToolExecuteMessage`. The output
disclosure now mounts only when `has_output` is true or a projected stdout or
stderr byte count is positive. Zero-output commands retain their command,
working directory, and status while never mounting the lazy output hook.

The component regression was first run red against the unguarded row, then
passed after the guard and fixture updates (9 tests). The desktop E2E suite
passed all 4 tests, and the mobile E2E suite passed both tests. Web typecheck,
Prettier, and `git diff --check` also passed. Fresh synthetic desktop and
mobile screenshots were captured, inspected, compressed, and recorded in the
ignored PR asset manifest; no production artifacts or blockers remain.
