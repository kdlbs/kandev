---
id: "01-notification-helper"
title: "Implement release contributor notification helper"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002
acceptance_criteria:
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.2
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.3
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.4
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.5
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.1
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.2
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.3
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.4
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.5
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.6
  - AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.7
system_design:
  - ../../specs/release/system-design/contributor-notifications.md
---

# Task 01: Implement the Notification Helper

## Summary

Build the helper with mocked API and Git tests before workflow integration.
Use TDD to prove selection, duplicate prevention, and recoverable write failures.

## In scope

- Optional tag, bound latest-release resolution, Stable validation, and preview.
- Read the sole maintainer declaration from `cliff.toml` without duplicating its list.
- Validate candidate PRs, human authors, and merge-commit ancestry at the release tag.
- Read all conversation pages; recognize markers and approved legacy text.
- Serialize comments with one-second minimum spacing; report partial and uncertain failures.
- Produce Markdown summaries without credential exposure or metadata injection.

## Out of scope

Workflow files, publication changes, live comment tests, and dependency additions.

## Acceptance

1. Latest and explicit releases share one validated recipient-selection path; preview never makes a POST request.
2. Only eligible external human PRs receive the approved comment; paginated notices and duplicate references never create another notice.
3. Failure fixtures prove preserved successes, uncertain-write reconciliation, nonzero failure status, and complete result summaries.

## Verification

```bash
python3 .github/scripts/notify-release-contributors_test.py
git diff --check -- .github/scripts/notify-release-contributors.py .github/scripts/notify-release-contributors_test.py
```

Test missing latest release, missing explicit tag, draft/prerelease, malformed maintainer declaration,
API bot type, case-insensitive maintainer identity, issue-only references, unrelated merge commits,
existing notice beyond page one, empty eligible set, successful writes, failed reads, failed writes,
uncertain POST success, secondary rate limits, and exact comment rendering.
Use injected transport, Git runner, and sleep controls so tests do not access GitHub or wait in real time.

## Files likely touched

- `.github/scripts/notify-release-contributors.py`
- `.github/scripts/notify-release-contributors_test.py`

## Dependencies

None. `cliff.toml` remains the existing maintainer source.

## Risks

Do not infer inclusion from release-note links alone or silently treat API failures as excluded PRs.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/release/requirements/contributor-notifications.md)
- [System design](../../specs/release/system-design/contributor-notifications.md)
- `.github/AGENTS.md`, `cliff.toml`, and existing `.github/scripts/*_test.py` patterns.

## Results

- `python3 .github/scripts/notify-release-contributors_test.py`: 19 tests passed.
- `git diff --check -- .github/scripts/notify-release-contributors.py .github/scripts/notify-release-contributors_test.py`: passed.
