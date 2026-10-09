---
status: active
system: release
created: 2026-09-19
owners:
  - kandev
---

# Stable Release Artifact Upload Recovery Requirements

## Overview

The release workflow transfers required build outputs between jobs through
GitHub Actions artifacts. A transient artifact service or network failure must
not immediately discard an otherwise valid platform build, while a persistent
or incomplete upload must still block publication.

The release system owns this contract because it owns the artifact set,
publication gates, and recovery path for Stable and shared runtime builds.

## Terminology

- **Required artifact:** A web bundle, platform runtime bundle, or desktop
  artifact consumed by a later release job.
- **Upload attempt:** One invocation of the pinned GitHub artifact upload action
  for one required artifact name.
- **Backfill:** A Stable workflow run that reuses the latest existing signed
  release tag to repair missing artifacts or publication outputs.

## Requirements

### REQ-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001: Recover transient required artifact upload failures

**Intent:** Preserve valid release builds when GitHub's artifact service has a
transient failure, while keeping incomplete releases from reaching publication.

#### Acceptance criteria

- **AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.1:** When a required artifact upload
  fails, the workflow shall make no more than three total upload attempts for
  that artifact, waiting 30 seconds before the second attempt and 60 seconds
  before the third attempt.
- **AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.2:** When a required artifact path
  contains no files, the upload step shall fail explicitly; retry behavior shall
  not turn a missing build output into a successful artifact.
- **AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.3:** When one Stable desktop matrix
  target fails, the other desktop targets shall continue to completion, while
  release publication shall still require every desktop target to succeed.
- **AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.4:** When all upload attempts for a
  required artifact fail, the producing job shall fail and GitHub Release, npm,
  Homebrew, and Scoop publication jobs shall not run from that incomplete run.
- **AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.5:** Each retry shall log its attempt
  and wait duration without exposing credentials, tokens, or signing material.

## Out of scope

- Retrying artifact downloads or external package-manager publication calls.
- Automatically starting a new workflow run or choosing a `backfill_tag`.
- Changing release versioning, tag signing, artifact contents, or publication
  ordering.
- Treating a partial release as complete when a required artifact remains
  unavailable.
