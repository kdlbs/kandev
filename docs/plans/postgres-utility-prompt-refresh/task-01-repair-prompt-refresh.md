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
3. Run the existing upgrade rehearsal from the published baseline and confirm
   candidate startup and preservation on PostgreSQL and SQLite.

## Verification

Use a disposable PostgreSQL DSN in `KANDEV_TEST_POSTGRES_DSN`. Confirm the
PostgreSQL test executes and passes rather than skipping.

```sh
cd apps/backend
go test ./internal/utility/store -run 'Test(EnhancePromptV1Hash|RefreshUneditedBuiltinPrompt)' -count=1 -v
```

The full [Release upgrade regression workflow](../../specs/platform/system-design/release-upgrade-regression.md#ci-only-execution)
runs only on GitHub Actions on `main`. After the fix lands, verify PostgreSQL
desktop and SQLite desktop/mobile against the published baseline. Capture
candidate startup logs on failure.

## Results

The new test reproduced the PostgreSQL `int4` binding error before the fix.
After the fix, the utility store race suite passed with PostgreSQL 17.11.
The test confirms the shipped prompt refreshes, customized text remains,
the profile and other fields stay intact, and a second replay succeeds.
SQL guard and the persistence conformance race suite passed with both database
engines enabled. The full release rehearsal remains pending on `main`.
