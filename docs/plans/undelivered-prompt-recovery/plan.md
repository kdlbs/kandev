---
created: 2026-11-12
status: completed
requirements:
  - REQ-PLATFORM-UNDELIVERED-PROMPT-001
  - REQ-PLATFORM-DELIVERY-RETENTION-001
system_design:
  - ../../specs/platform/system-design/undelivered-prompt-recovery.md
legacy_specs: []
---

# Implementation plan: Undelivered prompt recovery

## Problem and final scope

A dispatch failure before any transport write must not become an unknown delivery
outcome. A lost reply after a write must not trigger an automatic resend. An open
unknown-outcome block must prevent another submission from displacing its binding.

The supplied logs show provider DNS failure followed by dispatch failure and
repeated native Resume rejection. Journal absence came from the reporter; no
SQLite database or journal was attached for independent inspection.

## Upstream comparison and rebase

Rebased on main at `585824a5d` on 2026-11-14. PR #5312 already repairs eligible
SQL-only interrupted history using serialized missing-peer retirement. PR #5320
adds stale-history reconciliation. The original historical absence protocol was
removed from this change. Native Resume keeps uncertain outcomes unknown and
preserves its existing retirement evidence.

The remaining changes are typed transport non-delivery, conditional failed
settlement, retry safety, and the final unknown-block admission guard. No new
journal state, wire capability, endpoint, rendered UI, or localization is needed.

## Work orders

- [x] [Task 01: Classify rejection and fence dispatch](task-01-dispatch.md)
- [x] [Task 03: Verify supported recovery](task-03-recovery.md)

The former Task 02 implementation is superseded by upstream PR #5312. This PR
keeps integration coverage instead of adding another historical recovery protocol.

## Verification

Run Go checks from `apps/backend` with `-tags fts5`:

- Focused race tests for `UndeliveredPromptDispatch`, `BlockedPromptAdmission`,
  `PrepareAgentDeliverySubmission`, native Resume, queue/send-now recovery, and
  reconnect behavior across orchestrator, lifecycle, client, and SQLite packages.
- SQL guard and changed-package Go lint.
- Desktop `undelivered-prompt-recovery.spec.ts` and phone
  `mobile-undelivered-prompt-recovery.spec.ts` through the managed E2E runner.
- Documentation catalog, full specification lint, and `git diff --check`.

Rebased verification on 2026-11-15:

- The lost-reply regression fails on a clean checkout of main: main resends the
  request after the peer receives it and closes the connection.
- Focused race tests passed for orchestrator, lifecycle, and SQLite native Resume.
  Additional client stream, pending-request, prompt, and retirement tests passed.
- `go run ./cmd/sqlguard ./internal` and
  `go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1`
  passed. PostgreSQL remains unverified because no disposable database is configured.
- Managed desktop and phone E2E runs passed. They verify two SQL-only unknown rows,
  blocked admission, journal retirement, native conversation identity, reload,
  a distinct follow-up, and phone layout.
- Documentation catalog, full specification lint, and diff whitespace checks passed.
- Changed-package lint passed with the same scope and a ten-minute timeout.
  The five-minute attempts timed out during package loading or final analysis.
  Normal commit hooks remain required before publication.

Earlier broad process/SQLite suites reached their default ten-minute timeout.
Some host checks lacked `/bin/sleep`; these earlier runs do not certify this base.
The commit checks required two structural corrections: expose the error through
the runtime package and move the new browser assertions into focused helpers.
Architecture and web lint passed after these corrections. Affected orchestrator
race tests passed again. The first published head passed all CI checks. Review
then found missing initial settlement, a missing-client proof gap, and payload cleanup without terminal events.
The follow-up adds regressions for those cases and clarifies Resume eligibility.
Focused race tests passed for runtime, SQLite, and orchestrator, including initial
admission from both owners, disappearance of the client, changed ownership,
uncertain peer errors, and protected retention. SQL guard, store conformance, and
documentation checks passed. Changed-package lint and normal commit hooks remain
publication gates. The earlier CI result is historical; CI and review must run
again for the follow-up head.

## Risks

A false unsent classification could conceal accepted work. Keep the type limited
to paths before a write, and never carry it across a later ambiguous attempt.
SQL settlement must preserve ownership races. Existing historical recovery and
its refusal behavior remain owned by the upstream native Resume implementation.
