---
id: "02-agent-settings-e2e"
title: "Agent settings E2E coverage"
status: done
wave: 2
depends_on: ["01-agent-settings-layout"]
plan: "plan.md"
spec: "../../specs/agents/requirements/settings-profile-layout.md"
---

# Task 02: Agent settings E2E coverage

## Acceptance

- The desktop E2E test proves the configured card has the `New profile` action
  in its header, no profile-count/action row, and the installed-agent toolbar
  order is terminal, refresh/rescan, then rightmost Add TUI Agent.
- The mobile E2E test proves the same user value with the `mobile-chrome`
  project, uses touch for the creation action, verifies a 44px-or-larger hitbox,
  and detects no document-level horizontal overflow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-layout.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-profile-layout.spec.ts
```

The managed runner must rebuild the production Vite assets before the tests.
Do not use headed mode. If a focused test needs the exact fixture path or a
different project selector, record the final command actually run below.

## Files likely touched

- `apps/web/e2e/tests/settings/agent-profile-layout.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-layout.spec.ts`

## Dependencies

Task 01 must land first so the tests target final markup and selectors.

## Parallelism

Sequential. E2E selectors and layout assertions depend on task 01.

## Inputs

- Spec: `docs/specs/agents/requirements/settings-profile-layout.md`, all Scenarios.
- Plan: `plan.md`, E2E Tests and Mobile design contract sections.
- Existing patterns: `agent-profile-duplicate.spec.ts`,
  `mobile-agent-profile-duplicate.spec.ts`, and
  `mobile-agent-runtime-update.spec.ts`.

## Output contract

Report changed test files, exact managed-runner commands and test counts,
screenshots or failure artifacts if produced, cleanup evidence, blockers, and
synchronized task/plan status in the same conversation.

## Results

- `cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-layout.spec.ts` passed (2 tests).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-profile-layout.spec.ts` passed (1 test).
- Both managed runs rebuilt the backend and pseudo-locale Vite assets and started from the runner's cleaned E2E artifact directories. No failure artifacts remain.
