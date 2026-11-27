---
status: active
system: release
created: 2026-10-05
owners:
  - kandev
---

# Release Contributor Notifications Requirements

## Overview

Maintainers can announce a published release on its external contributors' PRs.
They can run notifications separately or select them when they start a release.
The release system owns this capability because it owns publication and maintainer release operations.

## Terminology

- **Latest release:** The published Stable release that GitHub identifies as latest.
- **External contributor:** A human PR author outside the maintainer list used for release-note attribution.
- **Eligible PR:** A same-repository, merged PR linked in the release notes whose merge commit belongs to the release.

## Requirements

### REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001: Manual release notices

**Intent:** Maintainers can notify contributors without rebuilding or republishing a release.

#### Acceptance criteria

- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.1:** The notification workflow shall accept an optional release tag in the GitHub Actions interface.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.2:** When the tag is empty, the workflow shall resolve the latest release once and display its identity.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.3:** When a tag is supplied, the workflow shall use that published Stable release without substituting another release.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.4:** When the release is missing, unpublished, or a prerelease, the workflow shall fail before posting comments.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001.5:** A normal manual run shall post notices. An optional preview shall display the same selection and text without posting.

### REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002: Eligible recipients and repeat runs

**Intent:** Notices reach contributors once per PR and release, including after a partial failure.

#### Acceptance criteria

- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.1:** The workflow shall select eligible PRs by external human authors. It shall exclude maintainers, bots, open PRs, and unmerged PRs.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.2:** Duplicate references, unrelated references, and references to issues shall not cause extra or incorrect comments.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.3:** Each notice shall identify and link the selected release and thank the contributor.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.4:** A repeat or overlapping run shall not add a second notice for the same PR and release. Only comments from `github-actions[bot]` or a maintainer listed in `cliff.toml` shall count as prior notices.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.5:** The notices manually posted for v0.97.0 shall count as existing notices when a listed maintainer posted them.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.6:** When an API operation fails, the workflow shall report failure and preserve successful notices for a later repeat run.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002.7:** The run summary shall show the release, comment text, target PRs, and posted, skipped, and failed outcomes.

### REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003: Optional notices after publication

**Intent:** A maintainer can request notices as part of a release without a second manual run.

#### Acceptance criteria

- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.1:** The Release workflow shall expose a notification checkbox, unchecked by default.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.2:** When checked, notices shall run after GitHub, npm, Homebrew, and Scoop publication all succeed for a Stable release.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.3:** The release run shall pass its exact tag to notifications, even if GitHub's latest-release identity changes.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.4:** An unchecked box, Nightly, dry run, Desktop validation, cancellation before the notification job starts, or failed publication shall produce no automatic notices. Cancellation after posting starts may leave partial notices; a repeat run shall safely skip confirmed notices.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.5:** A successful Stable backfill with the checkbox checked shall notify for its existing tag without duplicate notices.
- **AC-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003.6:** A notification failure shall remain visible. Recovery shall use the separate workflow without repeating successful publication jobs.

## Out of scope

- Notifications for PRs absent from release notes, direct messages, or email.
- Changes to release versioning, package publication, or tag identity.
- Scheduled notifications or automatic notices without the checkbox.
- Guaranteed inbox delivery when a contributor mutes GitHub notifications.

## Implementation plan

See [the delivery plan](../../../plans/release-contributor-notifications/plan.md).
