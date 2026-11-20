---
id: "03-recovery"
title: "Verify supported recovery"
status: completed
wave: 2
depends_on: ['01-dispatch']
plan: "plan.md"
requirements:
  - REQ-PLATFORM-UNDELIVERED-PROMPT-001
acceptance_criteria:
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.2
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.3
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.4
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.5
system_design:
  - ../../specs/platform/system-design/undelivered-prompt-recovery.md
---

# Task 03: Verify supported recovery

## Summary and scope

Exercise existing Resume against an isolated session with two absent SQL rows.
Verify desktop and phone behavior, blocked sends, reload, and a distinct follow-up.
Update public recovery guidance only after the implementation is verified.

## Out of scope

Provider connectivity repair, generic force-clear controls, and Office scheduling.

## Acceptance

- Resume preserves the native conversation and never dispatches the old payload.
- Desktop and phone retain usable busy/error states and at least 44-pixel phone targets.
- Reload and repeated actions do not duplicate delivery or clear independent blocks.

## Verification

Run from repository root. Write failing regression tests before implementation.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/session/undelivered-prompt-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-undelivered-prompt-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths after tests.
For repository changes, repeat the relevant tests with `KANDEV_TEST_POSTGRES_DSN`
pointing to a disposable database. Record unavailable infrastructure as unverified.

## Files likely touched

- `apps/web/e2e/tests/session/undelivered-prompt-recovery.spec.ts (new)`
- `apps/web/e2e/tests/session/mobile-undelivered-prompt-recovery.spec.ts (new)`
- `apps/web/e2e/tests/session/durable-stream-recovery.spec.ts (fixture reference)`
- `docs/public (existing recovery guidance)`

## Dependencies

01-dispatch and upstream PR #5312.

## Risks

Lost replies, delayed admission, replaced journals, partial commits, and stale owners.

## Parallelism

Sequential. No delegation is authorized.

## Inputs

Read the linked requirement and design, scoped AGENTS.md, and existing native
Resume tests. Read the relevant existing fixture before adding another fixture.

## Results

Main already implements missing-peer retirement. This work order verifies that
existing contract with two SQL-only rows and the new admission guard. Expected
historical outcomes remain unknown, and journal records must be retired.

Rebased desktop and phone runs passed on 2026-11-15 through the managed runner.
The fixtures preserve SQL uncertainty and verify retired journal records with no
payload. A send during the block creates no new SQL submission and preserves the
original block binding. Resume, reload, conversation identity, a distinct follow-up,
and phone layout assertions pass. Tests use isolated data; no production UI changes
are needed. SQL guard, store conformance, and documentation checks also passed.
