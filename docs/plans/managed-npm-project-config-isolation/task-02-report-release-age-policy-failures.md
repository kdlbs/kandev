---
id: "02-report-release-age-policy-failures"
title: "Report release-age policy failures"
status: done
wave: 2
depends_on:
  - "01-isolate-managed-npm-commands"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.4
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 02: Report release-age policy failures

## Summary

Classify npm's date-qualified exact-package `ETARGET` as a policy failure and
skip stale-cache recovery for it. Present one actionable, localized explanation
in task and Office chat while preserving sanitized technical details.

## In scope

- Start with a failing `TestManagedRuntimeReleaseAgePolicySkipsCacheRepair`
  regression, plus a safe-stderr projection test for the issue's npm line.
- Extend the exact-package matcher and probe/lifecycle failure contract with a
  distinct code. Check only the trusted top-level package and reject generic
  disconnects, transitive failures, and malformed suffixes.
- Persist the code and safe details in the existing recovery entry. Show the
  policy-specific copy and current retry action in Kanban and Office. Translate
  new copy in all required locales and document troubleshooting.

## Out of scope

- Changing the selected runtime version, npm settings, or package registry.
- A second automatic retry or global npm cache cleanup.

## Acceptance

- Date-qualified `ETARGET` for the exact package produces the stable policy
  error without cache invalidation or online-preferred retry.
- The desktop and phone recovery cards explain `min-release-age`/`before`, do
  not claim cache refresh, and keep technical details collapsed and sanitized.
- Other npm errors retain their current classification and recovery behavior.

## ASCII UI preview

`UI-01: Managed runtime startup error` applies to the inline Kanban and Office
chat entry. See the [full preview](plan.md#ascii-ui-preview). The text is
illustrative; copy is localized. AC-AGENTS-MANAGED-RUNTIME-RECOVERY-003.4
requires the policy explanation and the existing recovery path.

```text
Desktop: [!] npm blocked the selected runtime version
         Check min-release-age or before. Wait or select an older version.
         Technical details >      [Retry runtime]

Phone:   [!] npm blocked this runtime
         Check min-release-age or before.
         Wait or select an older version.
         Technical details >
         [ Retry runtime (at least 44 px) ]
```

The existing transcript remains the only scroll owner. Details start closed.
No new overlay or navigation is introduced.

## Verification

```bash
(cd apps/backend && go test ./internal/common/npmresolution ./internal/agent/runtime/routingerr ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agentctl/server/process ./internal/agentctl/server/utility ./internal/orchestrator -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/action-message.test.tsx components/task/simple/components/run-error-entry.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/managed-runtime-npm-recovery.spec.ts tests/office/managed-runtime-npm-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-managed-runtime-npm-recovery.spec.ts)
```

## Files likely touched

- `apps/backend/internal/common/npmresolution/matcher.go`
- `apps/backend/internal/agentctl/server/process/managed_npm_stderr.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`
- `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/simple/components/managed-runtime-npm-run-error.tsx`
- `apps/web/src/locales/*/chat.json`
- `apps/web/e2e/tests/session/managed-runtime-npm-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-managed-runtime-npm-recovery.spec.ts`
- `apps/web/e2e/tests/office/managed-runtime-npm-recovery.spec.ts`
- `docs/public/agents-and-profiles.md`
- Targeted adjacent backend and component tests.

## Dependencies

Task 01. Classification and probe tests use its trusted command shape.

## Risks

- The stderr sanitizer must retain only a safe marker, not npm log paths or
  raw provider stderr.
- The policy-specific code must not select the stale-metadata card, whose
  existing copy says Kandev already repaired the cache.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-003`, the paired system design, and
  existing Kanban, Office, and mobile recovery examples.

## Results

Implemented strict release-date-qualified classification for the exact managed
package, retained only a canonical date marker in sanitized details, and skips
cache repair and online retry for a policy failure. Task and Office recovery
surfaces show localized `min-release-age` / `before` guidance with one retry;
the public runtime recovery guide documents project `.npmrc` isolation and
remaining user/global policy.

Review correction: the parser accepts npm 11.16's observed
`M/D/YYYY, h:mm:ss AM/PM` date as well as RFC3339-shaped dates for
compatibility. Lifecycle classification checks the raw bounded diagnostic
against the trusted exact package before generic redaction, then persists a
fixed policy excerpt with the canonical date marker. Regression tests include
the exact issue line at parser, sanitizer, lifecycle, routing-classifier, and
agentctl probe boundaries, plus malformed-date and mismatched-package cases.

The targeted Go suites passed, including lifecycle, host probe, agentctl, npm
matcher, routing, orchestrator, agent registration, and settings packages.
`make -C apps/backend build` passed. Focused web tests, typecheck, changed-file
ESLint, i18n check, production build, desktop/Office and mobile E2E, public-doc
validators, spec lint, and `git diff --check` passed after the review correction.
The direct `build:vite` script does not run generated-file prebuild hooks; use
`pnpm --filter @kandev/web build`.
