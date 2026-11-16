---
status: draft
system: ci
created: 2026-11-16
owners:
  - kandev
---

# UI screenshot requirements

## Overview

The PR skill asks contributors for screenshots. A required CI check makes missing screenshots visible and blocks merge.
The check applies equally to internal PRs and external forks.

## Requirements

### REQ-CI-UI-SCREENSHOTS-001: Screenshot presence

#### Acceptance criteria

- **AC-CI-UI-SCREENSHOTS-001.1:** Every open PR shall receive a screenshot-policy result, including drafts and forks.
- **AC-CI-UI-SCREENSHOTS-001.2:** A candidate UI change shall require at least one HTTPS image embed in its Screenshots section.
- **AC-CI-UI-SCREENSHOTS-001.3:** Contributors shall follow the existing affected-viewport capture guidance. Reviewers shall assess viewport coverage.
- **AC-CI-UI-SCREENSHOTS-001.4:** A checkbox, local path, plain link, recording, example, or badge outside the Screenshots section shall not satisfy the check.
- **AC-CI-UI-SCREENSHOTS-001.5:** Contributors and reviewers shall assess image visibility and relevance. The presence check shall not download or certify image content.
- **AC-CI-UI-SCREENSHOTS-001.6:** Changes outside candidate UI paths and recognized test, documentation, or tooling-only changes shall pass without screenshots. Renames shall evaluate both paths.
- **AC-CI-UI-SCREENSHOTS-001.7:** A missing-screenshot result shall name the triggering paths and explain how to add an image or request an exception.
- **AC-CI-UI-SCREENSHOTS-001.8:** A passing result shall establish screenshot presence only. It shall not claim capture freshness or visual correctness.

### REQ-CI-UI-SCREENSHOTS-002: Maintainer exception

#### Acceptance criteria

- **AC-CI-UI-SCREENSHOTS-002.1:** The exact repository-managed label `no-screenshots-allow` shall waive this check.
- **AC-CI-UI-SCREENSHOTS-002.2:** Body declarations and comment commands shall not waive this check. GitHub's existing label permissions shall control exception access.
- **AC-CI-UI-SCREENSHOTS-002.3:** Adding or removing the label shall reevaluate the current PR. The label shall remain effective until removed.
- **AC-CI-UI-SCREENSHOTS-002.4:** A waived result shall identify the label and remain distinct from screenshot presence. It shall affect no other check.

### REQ-CI-UI-SCREENSHOTS-003: Trusted enforcement

#### Acceptance criteria

- **AC-CI-UI-SCREENSHOTS-003.1:** The validator shall read contributor content as data and shall not execute fork code.
- **AC-CI-UI-SCREENSHOTS-003.2:** Body edits, head changes, base changes, and exception-label changes shall reevaluate the PR without an unrelated push.
- **AC-CI-UI-SCREENSHOTS-003.3:** Incomplete or unavailable evaluation input shall produce an error rather than a passing result.
- **AC-CI-UI-SCREENSHOTS-003.6:** The result shall belong to the evaluated PR head. A stale event shall not approve a newer head.
- **AC-CI-UI-SCREENSHOTS-003.7:** Merge enforcement shall be reported only after the live target ruleset requires the check and blocking behavior is demonstrated.

## Exclusions

- Image downloads, image decoding, capture attestations, or image-to-commit declarations.
- Custom exception commands, a new permission system, or an evidence registry.
- Automatic screenshot capture and visual approval.
- Merge-queue integration. Add it before enabling a queue on the protected branch.
- Changes to GitLab, Azure Repos, or unrelated repository rules.

## Implementation plan

- [Screenshot presence check](../../../plans/ui-screenshot-enforcement/plan.md)
