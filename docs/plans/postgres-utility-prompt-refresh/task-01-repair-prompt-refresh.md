---
id: "01-repair-prompt-refresh"
title: "Repair the PostgreSQL prompt refresh"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-002
acceptance_criteria:
  - AC-AGENTS-UTILITY-AGENT-PROFILES-002.3
system_design:
  - ../../specs/agents/system-design/utility-agent-profiles.md
---

# Task 01: Repair the PostgreSQL prompt refresh

Status: implemented. The release rehearsal on main is pending.

During startup, refresh a known legacy built-in Enhance prompt on PostgreSQL
when the stored text matches the shipped revision. Preserve the user's custom
prompt text, profile binding, and other agent fields. Preserve SQLite behavior.
Run the PostgreSQL replay regression in the fixed PostgreSQL 16 CI cohort and
document its eligibility rule separately from saved-prompt refreshes.

- Requirements: AC-AGENTS-UTILITY-AGENT-PROFILES-002.3 and
  AC-PLATFORM-UPGRADE-REGRESSION-002.3.
- Designs: [utility agent profiles](../../specs/agents/system-design/utility-agent-profiles.md)
  and [release upgrade regression](../../specs/platform/system-design/release-upgrade-regression.md).
- Source: `apps/backend/internal/utility/store/builtins.go`.
- Tests: `apps/backend/internal/utility/store/builtins_refresh_test.go`.
- Exclusions: schema changes, SQL driver changes, unrelated startup fixes,
  viewer code, version changes, and publication.

## Implementation and acceptance

1. Add a PostgreSQL integration case using the existing isolated database
   helper. Initialize the schema, seed the shipped legacy text on its built-in
   row, and initialize the store again. Confirm it fails with the current
   PostgreSQL binding error.
2. Bind the integer built-in value in the guarded update. Keep the guard on
   the original prompt text. Verify the refreshed text, untouched binding
   and other fields, preservation of custom text, and idempotent replay.
3. Include ./internal/utility/store in the fixed PostgreSQL 16 CI cohort.
   The workflow contract test must verify that the package is passed to go test
   while KANDEV_TEST_POSTGRES_DSN is set by the job.
4. Document the Enhance utility refresh as hash-eligible with a conditional
   update over the original prompt text. Preserve both known-hash and
   created_at == updated_at eligibility checks for the changes-walkthrough and
   CI Auto-Fix saved-prompt refreshes.
5. Run the existing upgrade rehearsal from the published baseline and confirm
   candidate startup and preservation on PostgreSQL and SQLite.

## Verification

Use a disposable PostgreSQL DSN in `KANDEV_TEST_POSTGRES_DSN`. Confirm the
PostgreSQL test executes and passes rather than skipping.

```sh
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL DSN}"
(
  cd apps/backend
  go test -trimpath -tags fts5 -race -count=1 -v \
    ./internal/utility/store ./internal/prompts/store ./internal/persistence/storeconformance
)
python3 .github/scripts/backend-tests-workflow-contract_test.py
make -C apps/backend sqlguard
```

The full [Release upgrade regression workflow](../../specs/platform/system-design/release-upgrade-regression.md#ci-only-execution)
runs only on GitHub Actions on `main`. After the fix lands, verify PostgreSQL
desktop and SQLite desktop/mobile against the published baseline. Record the
run URL and candidate SHA. Capture candidate startup logs on failure.

Persistence guidance must state that the Enhance utility refresh uses a known
prompt hash and an original-prompt conditional update, while the changes-
walkthrough and CI Auto-Fix saved-prompt refreshes require both a known hash
and `created_at == updated_at`. Cross-check the saved-prompt guards in
`apps/backend/internal/prompts/store/sqlite.go` and the accepted CI Auto-Fix
outcomes decision.

## Results

The new test reproduced the PostgreSQL `int4` binding error before the fix.
The updated PostgreSQL 16.16 race suite passed with the shipped and customized
prompt cases across two schema replays. The test confirms profile bindings and
all other agent fields remain intact.
SQL guard, backend build, scoped Go lint, specification validation, and the
persistence conformance race suite passed. The workflow contract test passed all
26 tests and verifies that the PostgreSQL 16 package cohort invokes the utility
store with KANDEV_TEST_POSTGRES_DSN set.
Persistence guidance preserves both saved-prompt eligibility checks and the
conditional original-value update rules. The full release rehearsal remains
pending on `main`; record its run URL and candidate SHA after it runs.
