---
id: "02-workflows-and-guidance"
title: "Add notification workflows and maintainer guidance"
status: done
wave: 2
depends_on:
  - "01-notification-helper"
plan: "plan.md"
requirements:
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003
acceptance_criteria:
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.1
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.5
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.4
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.1
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.2
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.3
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.4
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.5
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.6
system_design:
  - ../../specs/release/system-design/contributor-notifications.md
---

# Task 02: Add Workflows and Maintainer Guidance

## Summary

Expose the helper through a manual and reusable workflow.
Add the release checkbox, success gates, CI coverage, and operator guidance.

## In scope

- Create the notification workflow with optional `release_tag` and default-false `dry_run` inputs for both entry points.
- Use a common non-cancelling posting lock, trusted workflow revision, and minimum job permissions.
- Add default-false `notify_contributors` to Release and a direct reusable-workflow call with the exact prepared tag.
- Cover every publication result and excluded mode through workflow contract tests.
- Add script and workflow tests to the existing action-pinning CI workflow.
- Update the public release guide, engineering guide, and release skill with manual and opt-in flows.
- Record final outcomes and promote paired specifications only when implementation satisfies all requirements.

## Out of scope

Additional release-event triggers, scheduled notifications, live comment tests, and changing publication prerequisites.

## Acceptance

1. Manual runs support latest fallback, explicit tags, and preview; both entry points execute Task 01's helper with the same lock and permissions.
2. Checked successful Stable releases and backfills pass their exact tag; every unchecked, skipped, cancelled, or failed publication path skips notices.
3. CI exercises the new contracts, and maintainer guidance describes posting, preview, repeat runs, bot identity, and recovery after notification failure.

## ASCII UI preview

UI-01 and UI-02 match [the plan previews](plan.md#ascii-ui-preview).

```text
Notify release contributors:
Release tag (optional) [               ]
[ ] Preview without posting comments
[ Run workflow ]

Release, after existing inputs:
[ ] Notify external contributors after a successful Stable release
[ Run workflow ]
```

GitHub owns desktop and phone rendering. No Kandev web components change.

## Verification

```bash
python3 .github/scripts/notify-release-contributors_test.py
python3 .github/scripts/notify-release-contributors-workflow-contract_test.py
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
node scripts/validate-public-docs.mjs
python3 .github/scripts/lint-harness-files.py --all
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
zizmor .github/workflows
git diff --check
```

Compare security findings with the pre-change baseline; do not mask new findings as existing debt.
Contract fixtures must cover optional tag defaults, permission propagation, helper checkout identity,
shared concurrency, all four success gates, backfill, and every excluded mode.
After deployment, use a read-only preview for v0.97.0 to confirm legacy notice recognition.
Do not deploy, commit, push, or post comments as part of local verification without applicable user authorization.

## Files likely touched

- `.github/workflows/notify-release-contributors.yml`
- `.github/workflows/release.yml`
- `.github/workflows/lint-action-pinning.yml`
- `.github/scripts/notify-release-contributors-workflow-contract_test.py`
- `.github/scripts/release-workflow-contract_test.py`
- `docs/public/release-process.md`
- `AGENTS.md`
- `.agents/skills/release/SKILL.md`
- This plan, both work orders, and the paired requirement and system design.

## Dependencies

Task 01.

## Risks

Implicit success conditions can skip dependent jobs when unrelated modes skip prerequisites.
Use `!cancelled()` and explicit dependency results. The reusable job must not acquire its caller's release lock.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/release/requirements/contributor-notifications.md)
- [System design](../../specs/release/system-design/contributor-notifications.md)
- `.github/AGENTS.md`, release skill, and docs-maintainer skill.
- Existing `release.yml`, release contract tests, and public release guide.

## Results

- `python3 .github/scripts/notify-release-contributors_test.py`: 19 tests passed.
- `python3 .github/scripts/notify-release-contributors-workflow-contract_test.py`: 4 tests passed.
- `python3 .github/scripts/release-workflow-contract_test.py`: 49 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: all 26 workflow files passed.
- `node scripts/validate-public-docs.mjs`: all 47 published docs pages passed. `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `python3 .github/scripts/lint-harness-files.py --all`: all 201 harness files passed.
- `python3 scripts/list-docs.py validate`: 348 decisions and 1343 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.
- `zizmor .github/workflows` exits 14 on existing repository findings. The Release workflow has 27 findings at both `HEAD` and the implementation revision, with zero route-level changes; the new notification workflow has zero findings.
