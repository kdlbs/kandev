---
id: "04-verify-recovery-flows"
title: "Verify desktop and mobile recovery"
status: done
wave: 4
depends_on:
  - "03-show-recovery-errors"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.6
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 04: Verify Desktop and Mobile Recovery

## Summary

Verify the complete user journey on desktop and mobile. Run the full backend,
frontend, formatting, complexity, and internationalization gates requested for
delivery.

## In scope

- Extend the session page object with error, read-only, new-branch, and warning
  locators.
- Add a deterministic E2E fixture for a session whose stored branch is absent
  locally and on its configured remote.
- Prove the first Resume click shows the descriptive error.
- Prove repeated Resume clicks retain visible feedback.
- Prove the branch action is explicit and resumes the same conversation on a
  new branch from the configured base.
- Prove the warning survives page reload and appears only once.
- Prove automatic read-only restore shows its notice and failure cause.
- Add an equivalent `mobile-*.spec.ts` path for the recovery surface.
- Assert mobile touch targets, keyboard reachability, and no horizontal
  overflow.
- Run all requested repository validation commands.

## Out of scope

- Screenshot documentation or public documentation changes.
- Performance benchmarks unrelated to recovery.
- Provider-specific remote branch deletion outside the deterministic Git
  fixture.
- Visual redesign of the stopped-session banner or status message.

## Acceptance

- Desktop E2E covers failure feedback, explicit continuation, conversation
  continuity, honest warning copy, idempotency after reload, and read-only
  notice behavior.
- Mobile E2E covers the same recovery capabilities in the existing chat
  surface.
- Mobile recovery actions have at least 44 by 44 CSS pixels and remain usable
  without document-level horizontal overflow.
- Keyboard focus reaches each visible recovery action and activation works.
- Backend, frontend, formatting, complexity, and internationalization gates
  pass from the current PR base.

## Verification

Install workspace dependencies once if this worktree does not have them. Run:

```bash
# From apps:
rtk pnpm install --frozen-lockfile

# From apps/web:
rtk pnpm e2e:run --host tests/session/session-resume-recovery.spec.ts
rtk pnpm e2e:run --host --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts
rtk pnpm run typecheck
rtk pnpm run i18n:check
rtk pnpm run i18n:ratchet

# From apps:
rtk pnpm --filter @kandev/web lint

# From the repository root:
rtk bash -lc 'unset KANDEV_INTERNAL_CONFIG_FILE KANDEV_INTERNAL_CONFIG_HOME_FILE KANDEV_SERVER_PORT KANDEV_BACKEND_PORT KANDEV_PORT
export HOME=/tmp/kandev-task-test-home
exec make -C apps/backend test'
rtk make -C apps/backend lint

# From apps/backend. Refresh the SHA if the branch is rebased:
rtk golangci-lint run ./... --new-from-rev="fd2c7dcdff0cd7a1292653c2199f4945d796873b" --timeout=5m

# From the repository root after staging Go files:
rtk bash -lc 'git diff --cached --name-only --diff-filter=ACMR | grep "\.go$" | xargs -r gofmt -l'
rtk git diff --check
```

The Go formatting command must produce no output. Run `gofmt -w` on each
reported file, stage it again, and rerun the check.

## Files likely touched

- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/session/session-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`
- E2E API helpers or fixtures needed for deterministic branch deletion
- `docs/plans/session-resume-recovery/plan.md`
- Completed work orders in this plan directory

## Dependencies

- Task 03 completes the visible desktop and mobile recovery surface.

## Risks

- A mock that returns only a frontend error does not prove the worktree path.
  Seed real local Git state and remove both recoverable refs.
- A desktop test that only narrows the viewport does not select the mobile
  shell. Use the `mobile-chrome` project and a `mobile-*.spec.ts` file.
- Reload can race message history. Wait for persisted session history before
  asserting the single warning count.
- The recorded base SHA can become stale after rebase. Recompute the PR
  merge-base before the changed-file complexity gate.
- Full backend tests can reveal unrelated failures. Record exact evidence and
  separate it from focused recovery regressions.

## Parallelism

`sequential`

## Inputs

- Completed Tasks 01 through 03.
- Existing `session-recovery.spec.ts` and `SessionPage` recovery locators.
- Existing mobile model-warning and recovery E2E patterns.
- PR base SHA `fd2c7dcdff0cd7a1292653c2199f4945d796873b` at plan creation.

## Results

- GREEN: desktop branch-recovery E2E, 1 passed in 20.1s. It covers visible
  failure feedback, explicit new-branch continuation, session continuity,
  persisted warning metadata, reload idempotency, and automatic restore
  feedback.
- GREEN: mobile branch-recovery E2E, 1 passed in 20.1s. It covers the same
  actions with mobile shell behavior, keyboard focus, touch targets, and
  horizontal-overflow checks.
- Fresh PR screenshots were captured and reviewed at
  `apps/web/.pr-assets/session-resume-recovery--session-resume-recovery-desktop.png`
  and
  `apps/web/.pr-assets/mobile-session-resume-recovery--session-resume-recovery-mobile.png`.
  The manifest validates both files.
- GREEN: isolated full backend tests, backend lint, changed-file
  `golangci-lint`, frontend lint, typecheck, i18n checks, production Vite
  build, and `python3 scripts/lint-spec-files.py --all`.
