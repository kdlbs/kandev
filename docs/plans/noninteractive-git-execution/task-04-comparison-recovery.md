---
id: "04-comparison-recovery"
title: "Prove desktop and phone task access"
status: done
wave: 4
depends_on: ["03-network-budgets"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.5
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.6
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.16
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.17
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.18
system_design:
  - ../../specs/platform/system-design/git-subprocess-execution.md
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 04: Prove desktop and phone task access

## Summary

Desktop and phone retain task access and local file changes during real fixture authentication failure, then recover comparison data.

## In scope

Reuse the existing fork comparison desktop and mobile tests and their shared seeding helper.
Add a local HTTP authentication-failure target to the isolated fixture; an unavailable boolean or missing local path alone does not prove authentication behavior.
Create a local uncommitted file, open the task, and prove session readiness, local Changes access, and the existing unavailable target notice.
On mobile, enter Changes through its visible touch button and use the mobile panel.
Assert comparison-derived numbers are not fabricated from origin/main.
Restore the fixture response and use the existing refresh trigger to prove comparison recovery without restarting the backend.
Backend fixture assertions must prove the HTTP auth request occurred and local file status survives.

Keep production UI composition, copy, and navigation unchanged. These are rendered regression tests, so no new UI preview is needed.
If an existing error surface cannot expose completion, update this package before adding localized UI.
PTY and HTTP Go tests remain the terminal suppression evidence; browser tests cannot substitute for them.

## Out of scope

Credential-authority changes, transport fallback, live-instance changes, and unrelated refactors.

## Acceptance

- Desktop and phone retain task access and local file changes during real fixture authentication failure, then recover comparison data.
- Run new behavioral tests before implementation and record the expected failure, then the passing result.
- Preserve the contracts and exclusions in the linked design.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
make build-backend build-web-e2e
(cd apps/backend && go test ./internal/agentctl/server/process -count=1 -timeout=5m)
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/git/fork-pr-comparison-target.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/git/mobile-fork-pr-comparison-target.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/git/fork-pr-comparison-target.spec.ts`
- `apps/web/e2e/tests/git/mobile-fork-pr-comparison-target.spec.ts`
- `apps/web/e2e/tests/git/fork-pr-comparison-target-helpers.ts`
- `apps/backend/internal/agentctl/server/process/workspace_comparison_target_test.go (new)`

## Dependencies

03-network-budgets.

## Risks

Credential helper and process-lifecycle compatibility require real subprocess evidence. Do not infer success from environment assertions alone.

## Parallelism

`sequential`

## Inputs

- [Execution design](../../specs/platform/system-design/git-subprocess-execution.md).
- [Plan evidence and caller inventory](plan.md).
- Scoped AGENTS.md and relevant fix, TDD, and E2E skills.

## Results

Implemented after Task 03 on the verified PR #3635 descendant `9cc146ee21c296d553ae684531219a5c711a4213`.

Results: the desktop and mobile fork comparison specs use a disposable local smart HTTP Git fixture that returns authentication failure before recovery. Both flows preserve session access and a local uncommitted file, show the existing unavailable comparison state, observe the failed request, then recover comparison data after the fixture becomes available and Changes is refreshed. Reset cleanup removes comparison refs from the seed repository and task environments.

Checks passed:

- `(cd apps/web && pnpm e2e:run --host --no-build --project=chromium e2e/tests/git/fork-pr-comparison-target.spec.ts)`
- `(cd apps/web && pnpm e2e:run --host --no-build --project=mobile-chrome e2e/tests/git/mobile-fork-pr-comparison-target.spec.ts)`
