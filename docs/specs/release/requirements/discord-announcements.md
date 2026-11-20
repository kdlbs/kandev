---
status: active
system: release
created: 2026-10-23
owners:
  - kandev
---

# Discord release announcements

## Overview

Community members receive one compact release announcement in general, linked to the complete GitHub release notes.
Release owns this capability because it owns publication completion and release notes.

The user confirmed Stable-only delivery, whole-release category counts, contributor credits, emojis, and the exact GitHub release link.
The announcement excludes the website changelog link, a full Discord series, attachments, and the unsigned-installer notice.
Recovery uses explicit preview, create, and update modes. Automatic reruns and backfills defer to this recovery path.

## Requirements

### REQ-RELEASE-DISCORD-001: Release eligibility

**Intent:** Announce an available Stable release after its required publication channels succeed.

#### Acceptance criteria

- **AC-RELEASE-DISCORD-001.1:** A normal Stable release shall automatically send an announcement after GitHub Release, npm, Homebrew, Scoop, and public plugin SDK verification succeed and Stable completion passes.
- **AC-RELEASE-DISCORD-001.2:** Nightlies, drafts, prereleases, dry runs, validation-only runs, failed publication, and cancellation before notification starts shall send nothing.
- **AC-RELEASE-DISCORD-001.3:** Announcement failure shall report a separate delivery error without repeating or reverting package publication.
- **AC-RELEASE-DISCORD-001.4:** Release reruns and backfills shall require the recovery path before sending, so they do not automatically repeat an earlier announcement.

### REQ-RELEASE-DISCORD-002: Grouped changelog and attribution

**Intent:** Readers can identify changes, their contributors, and the complete release information.

#### Acceptance criteria

- **AC-RELEASE-DISCORD-002.1:** Announcements shall show category names and counts from the complete selected release, such as “Features (12)” or “Fixes (8)”, without PR-type prefixes. Counts shall include undisplayed entries and shall not depend on a page or excerpt. The message shall distinguish the total release count from the number of highlights shown. Empty groups shall be absent.
- **AC-RELEASE-DISCORD-002.2:** Each entry shall show linked GitHub contributor attribution, including maintainers. Unresolvable attribution shall display “Contributor unavailable” and produce an operator warning.
- **AC-RELEASE-DISCORD-002.3:** Announcements shall include the version, publication date, PR links where available, and the exact GitHub release link. They shall omit the website changelog link and links to a full Discord series.
- **AC-RELEASE-DISCORD-002.4:** Each release shall produce one bounded compact message. Oversized content shall reduce the displayed selection while preserving whole entries, credits, and whole-release counts. No attachment or additional message shall carry omitted entries.
- **AC-RELEASE-DISCORD-002.5:** Contributor credits and changelog text shall not cause Discord user, role, or everyone mentions.
- **AC-RELEASE-DISCORD-002.6:** One webhook shall deliver the compact announcement to general. No full-release webhook or separate Discord changelog shall be required or called.
- **AC-RELEASE-DISCORD-002.7:** The announcement shall use category emojis without PR-type prefixes. It shall omit attachment explanations and the unsigned macOS/Windows installer notice.

### REQ-RELEASE-DISCORD-003: Preview and recovery

**Intent:** Maintainers can inspect a message and recover delivery independently of publication.

#### Acceptance criteria

- **AC-RELEASE-DISCORD-003.1:** Preview shall show the exact compact message and whole-release counts for an explicit Stable tag without sending messages.
- **AC-RELEASE-DISCORD-003.2:** A confirmed send shall report its message ID and link. An uncertain send shall stop without automatic resubmission.
- **AC-RELEASE-DISCORD-003.3:** Recovery shall support updating a verified message from this sender for the selected release. A missing message shall not silently become a new post.
- **AC-RELEASE-DISCORD-003.4:** A manual new post shall require an explicit create action after the operator checks the channel for an existing announcement.

## Out of scope

- Nightly announcements, per-workspace Discord integrations, and Kandev application UI.
- AI summaries, native accordions, interactive bots, and automatic announcement-channel crossposting.
- Changes to historical release notes or the website changelog.
- Guaranteed exactly-once delivery and an independent durable delivery database.

## Implementation plans

See the [delivery plan](../../../plans/discord-release-announcements/plan.md).
