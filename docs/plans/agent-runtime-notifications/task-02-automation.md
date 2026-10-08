---
id: "02-automation"
title: "Automatic policy and delivery"
status: done
wave: 2
depends_on: ['01-capabilities']
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
  - REQ-AGENTS-RUNTIME-NOTIFY-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-001.3
  - AC-AGENTS-RUNTIME-NOTIFY-002.1
  - AC-AGENTS-RUNTIME-NOTIFY-002.2
  - AC-AGENTS-RUNTIME-NOTIFY-002.3
  - AC-AGENTS-RUNTIME-NOTIFY-002.4
  - AC-AGENTS-RUNTIME-NOTIFY-002.5
  - AC-AGENTS-RUNTIME-NOTIFY-002.6
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
---
# Task 02: Automatic policy and delivery

## Summary and scope

Persist off-by-default policy and terminal outcomes with trusted identity. Reuse verified candidate activation with consent/selection guards and maintenance admission. Wire one cancellable scheduler and preference-aware durable notifications.

## Out of scope

No model discovery, worker delegation, or global developer CLI/login changes. Native unverified activation remains manual.

## Acceptance

- The linked acceptance criteria hold across multiple registered identities.
- Failures preserve authoritative state and expose truthful recovery.
- Exact verification below passes, with results recorded.

## Verification

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/managedruntime ./internal/notifications/service ./internal/notifications/providers ./internal/backendapp -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race -tags fts5 ./internal/persistence/storeconformance -count=1)
```

## Files likely touched

settings/controller/agent_update_job* and new policy/scheduler files; settings/handlers/; notifications/; backendapp/

## Dependencies and inputs

01-capabilities. Read the linked requirements/design and nearest source/tests.

## Risks

See the plan for native ownership, source failure, consent/selection races, and overlapping PRs.

## Parallelism

sequential

## Results

Passed the listed controller/handlers/managedruntime/notifications/backendapp race suites, SQL guard, and persistence store-conformance race suite. The final combined owning-package race suite also passed on integrated main `517249b5e609`.

Additional deterministic coverage verifies shared source catalogue reuse, consent withdrawal during validation, preparation/probe/persistence failures, retained outcomes across controller recreation, interrupted activation without an invented result, and read-only subscriber replay after worker disposal. A temporary runtime installation and a real isolated fixture process prove automatic activation, manual rollback and default reset leave the existing process/version live.

The complete lifecycle race suite and hostutility race suite passed. Its existing launch-deadline test failed twice because a 40ms deadline expired before the download stage started. The test now allows one second for launch setup against a ten-second download timeout, preserving its assertion that the launch deadline bounds transfer. Focused and full lifecycle race verification passed after this fixture correction. Backend lint reports zero issues. Additional red/green tests require a verified release catalogue before automatic consent and immediate retained notifications when catalogue validation fails before job creation.

PR review remediation covers shared-source caller cancellation, consent-preserving manual admission and durable-outcome release before refresh callbacks. Native-host managed fallback controls preserve package selection/recovery without global native mutation or false host capability publication. Red/green regression tests pass for cancellation, rejected manual requests, terminal admission, verified native fallback update/rollback/default, strict policy JSON, save contributor identity, original outcome identity, and bootstrap readiness. Backend controller/handler/registry/backendapp tests pass with -race -tags fts5; five directly changed frontend suites pass 39 tests, plus two card-destination snapshot tests. TypeScript, all seven shipped locales, 18 desktop and seven phone E2E cases pass. External exact-head CI/review/merge gates remain pending.
