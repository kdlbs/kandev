---
id: "06-import-e2e-docs"
title: "Verify import end to end and update docs"
status: done
wave: 5
depends_on:
  - "01-source-adoption"
  - "02-session-isolation"
  - "03-import-engine"
  - "04-import-lifecycle"
  - "05-import-progress-ui"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-IMPORT-001
  - REQ-COSTS-USAGE-IMPORT-002
  - REQ-COSTS-USAGE-IMPORT-003
  - REQ-COSTS-USAGE-IMPORT-004
  - REQ-COSTS-USAGE-IMPORT-005
acceptance_criteria:
  - AC-COSTS-USAGE-IMPORT-001.1
  - AC-COSTS-USAGE-IMPORT-001.2
  - AC-COSTS-USAGE-IMPORT-002.2
  - AC-COSTS-USAGE-IMPORT-002.4
  - AC-COSTS-USAGE-IMPORT-003.1
  - AC-COSTS-USAGE-IMPORT-003.2
  - AC-COSTS-USAGE-IMPORT-003.5
  - AC-COSTS-USAGE-IMPORT-003.6
  - AC-COSTS-USAGE-IMPORT-004.1
  - AC-COSTS-USAGE-IMPORT-004.3
  - AC-COSTS-USAGE-IMPORT-005.1
  - AC-COSTS-USAGE-IMPORT-005.3
  - AC-COSTS-USAGE-IMPORT-005.5
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 06: Verify import end to end and update docs

## Summary

Prove the feature on an installed plugin against a real Host, with non-empty source data, on desktop and phone. The earlier import smoke used an empty source report, so successful import was never proven. Also measure real tokscale timing and update user documentation.

## In scope

- `test/historical-import-smoke.spec.mjs`: a session seeded without creation evidence is imported, shows progress, survives a plugin restart, and appears in Token Usage. Also covers the unsafe session and queue flows.
- One manual timing run of the real `tokscale` command on a real history, recorded in Results.
- `README.md` in the plugin, `docs/public/usage-history.md`, and a run of `/docs-maintainer`.

## Out of scope

- Remote executors.

## Acceptance

- The `chromium` and `mobile-chrome` projects pass with retries disabled, and the imported amounts match the fixture.
- Results record the session count, active days, and per-report duration from the real run, including whether any report approached the 120-second timeout.
- Public docs state what can and cannot be imported.

## Verification

```bash
(cd "$PLUGIN_ROOT" && make package-host)
(cd apps/web && KANDEV_HOST_ROOT="$PWD/../.." SESSION_COST_PACKAGE_PATH="$PLUGIN_ROOT/kandev-session-cost-0.5.0.tar.gz" SESSION_COST_EXPECT_ACTION=1 pnpm exec playwright test --config "$PLUGIN_ROOT/test/host-action-smoke.playwright.config.mjs" "$PLUGIN_ROOT/test/historical-import-smoke.spec.mjs" --retries=0)
git diff --check -- docs/public
```

Confirm the environment against the existing Host E2E section of the plugin `README.md` before running.

## Files likely touched

- `test/historical-import-smoke.spec.mjs` (new)
- `test/retained-usage-smoke-helper.mjs`
- `test/host-action-smoke.playwright.config.mjs`
- `README.md`
- `docs/public/usage-history.md`

## Dependencies

Tasks 01 to 05.

## Risks

- Seeding a pre-upgrade session must happen before adoption completes. The `/api/v1/e2e/usage-history` routes may need a fixture hook gated like the existing ones.
- If real tokscale reports approach the timeout, stop and raise it with the user. It may need a design change.

## Parallelism

`sequential`

## Inputs

- `test/retained-usage-smoke-helper.mjs` and `test/retained-usage-auth-smoke-helper.mjs`.
- `apps/backend/internal/backendapp/e2e_usage_history.go`.

## Results

The installed-Host historical-import flow passed in both `chromium` and `mobile-chrome` with retries disabled. It used the fixture tokscale command with a non-empty report and verified matching imported totals: 1,620 tokens and 4,200 cost subcents. The desktop flow also verified one imported and one skipped session in the primary workspace, one imported session in the queued workspace after backend restart, and the ineligible-session composer message.

The real tokscale timing sample covered 5,193 sessions over 129 days. The slowest sampled lifetime report took 72.01 seconds, the warm lifetime report took 24.56 seconds, and a bounded-day report took 18.58 seconds, all below the 120-second timeout. These are machine-specific measurements. The public usage-history documentation and plugin README were updated to describe import limits.

Verification passed: focused desktop and phone installed-Host E2E with `--retries=0`; plugin `make test` (Go tests, 71 Node tests, bundle syntax, package verification, release-version checks); `node scripts/validate-public-docs.mjs`; `node --test scripts/validate-public-docs.test.mjs`; `python3 scripts/list-docs.py validate`; and `python3 scripts/lint-spec-files.py --all`.
