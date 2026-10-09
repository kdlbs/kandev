---
status: current
system: release
requirements:
  - REQ-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001
---

# Stable Release Artifact Upload Recovery System Design

## Purpose and boundaries

The release system owns the GitHub Actions artifact handoff between build jobs
and publication jobs. This design covers required artifact uploads in
`build-web`, `build-bundles`, and `build-desktop`, plus the desktop matrix
failure policy. It uses the existing `backfill_tag` recovery contract for a
run that still fails after bounded retries.

Artifact downloads, GitHub Release asset uploads, GHCR mutations, npm
publication, Homebrew updates, and Scoop updates remain separate contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), and [Observability](#observability) |

## Components and responsibilities

- **`build-web` in `.github/workflows/release.yml`:** Uploads the shared web
  bundle with the bounded retry pattern.
- **`build-bundles` in `.github/workflows/release.yml`:** Uploads each named
  platform runtime bundle with the same pattern. The artifact name remains
  `bundle-<platform>` so existing consumers keep their contract.
- **`build-desktop` in `.github/workflows/release.yml`:** Runs all five desktop
  targets with `strategy.fail-fast: false` and uploads each validated desktop
  artifact with the bounded retry pattern.
- **Downstream release jobs:** Continue to require successful producer jobs.
  `publish-release` therefore remains the gate for npm, Homebrew, and Scoop.
- **`release-workflow-contract_test.py`:** Protects retry count, wait values,
  missing-file behavior, matrix isolation, and downstream success conditions.

The upload retry steps stay in the workflow file. This keeps the control logic
available from `github.workflow_sha` during a backfill even when the selected
application tag predates the resilience change.

## Data and contracts

Each required artifact keeps its current name and path contract. The first
upload attempt uses the pinned `actions/upload-artifact` action. Retry attempts
use the same name and path with `overwrite: true`, which permits a rerun to
replace a partially created artifact or an artifact left by an earlier attempt.

Required uploads set `if-no-files-found: error`. The diagnostics upload remains
best effort because it runs after a failed macOS build and is not consumed by
publication.

## Control flow

For each required artifact:

1. Run the upload action.
2. If its step outcome is `failure`, log the first retry and sleep for 30
   seconds.
3. Run the second upload attempt with `overwrite: true`.
4. If the second attempt fails, log the second retry and sleep for 60 seconds.
5. Run the third upload attempt with `overwrite: true`.
6. Fail the producing job when all three outcomes are failures.

`continue-on-error` is limited to the individual upload attempts. The final
failure gate converts three failed attempts back into a job failure. A
successful first or retry attempt skips later attempts.

The desktop matrix does not cancel sibling jobs after one target fails. The
publication job still checks `needs.build-desktop.result == 'success'`, so this
change preserves fail-closed publication while retaining evidence from every
target.

## Failure and recovery

A transient artifact-service or network error gets two bounded retry chances
inside the same job. A missing output, invalid path, or persistent service
failure remains a job failure after the retry budget is exhausted. Downstream
publication jobs remain skipped by their existing dependency checks.

After a signed tag exists, maintainers use the existing failed-job rerun or
`backfill_tag` procedure documented in
[`docs/decisions/0029-release-backfill-and-desktop-diagnostics.md`](../../../decisions/0029-release-backfill-and-desktop-diagnostics.md).
The resilience change does not create or move tags and does not bypass artifact
validation.

## Persistence

No repository, release, or package-manager state changes. GitHub Actions
artifacts remain ephemeral run-scoped handoff objects. A retry can replace an
artifact with the same name, but it does not alter the release tag or public
publication channels.

## Security

The change keeps the existing SHA-pinned upload action and job permissions. It
adds no credentials, network endpoints, or shell inputs. Retry logs contain
only attempt numbers, wait durations, and the existing artifact action output.

## Observability

Each retry emits a warning containing the artifact name, attempt number, and
wait duration. The final failure remains visible in the producing job, while
the desktop matrix records sibling failures independently for diagnosis.
The existing release workflow contract test verifies the static control flow.

## Related decisions

- [ADR 0029: Release Backfill and Desktop Diagnostics](../../../decisions/0029-release-backfill-and-desktop-diagnostics.md)
