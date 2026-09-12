---
status: draft
system: ci
created: 2026-09-10
owners:
  - kandev
---

# Pull request documentation coverage requirements

## Overview

Contributors must include the delivery record for changes that need design context.
The CI system owns the coverage check, automatic exemptions, and label override.
Contributors can write the artifacts manually; use of the repository harness is optional.

## Requirements

### REQ-CI-PR-DOCS-001: Predictable artifact coverage

**Intent:** Require reviewable context without requiring new specifications for every correction.

#### Acceptance criteria

- **AC-CI-PR-DOCS-001.1:** Every open pull request shall receive a documentation coverage result, including drafts and forks.
- **AC-CI-PR-DOCS-001.2:** A pull request containing only recognized documentation, tests, translation catalogs, or dependency lock changes shall pass without a delivery package.
- **AC-CI-PR-DOCS-001.3:** Other changes shall require an added or modified work order, its plan, and linked requirements and system designs. Changing the title to `fix`, `chore`, or `refactor` shall not exempt the change.
- **AC-CI-PR-DOCS-001.4:** Existing plans and specifications shall qualify when the work order references them and they exist in the proposed revision. Contributors shall update contracts when behavior changes, but shall not need meaningless edits to unchanged contracts.
- **AC-CI-PR-DOCS-001.5:** A deleted artifact, empty file, unresolved reference, unrelated unlinked document, or work order without requirement and acceptance references shall not satisfy coverage.
- **AC-CI-PR-DOCS-001.6:** The result shall identify triggering paths, accepted references, missing artifacts, and corrective steps. Structural coverage shall not claim semantic completeness or prove that planning preceded coding.

### REQ-CI-PR-DOCS-002: Explicit documentation exception

**Intent:** Let repository label managers waive unnecessary documentation work.

#### Acceptance criteria

- **AC-CI-PR-DOCS-002.1:** When the exact label `no-docs-allow` is present, the coverage policy shall pass and identify the override in its result.
- **AC-CI-PR-DOCS-002.2:** Adding or removing the label shall reevaluate the current pull request without requiring a new commit. Removal shall restore normal evaluation.
- **AC-CI-PR-DOCS-002.3:** The label shall remain effective across pushes until removed. The workflow shall not add, remove, or grant permission to apply it.
- **AC-CI-PR-DOCS-002.4:** The exception shall affect only documentation coverage. Other validation and merge requirements shall remain independent.

### REQ-CI-PR-DOCS-003: Reliable enforcement

**Intent:** Make the result usable as a required check without trusting contributor execution.

#### Acceptance criteria

- **AC-CI-PR-DOCS-003.1:** The result shall belong to the evaluated revision. A stale event shall not publish a success for a newer revision.
- **AC-CI-PR-DOCS-003.2:** Missing or incomplete evaluation data shall produce an error, never an automatic exemption.
- **AC-CI-PR-DOCS-003.3:** The check shall evaluate contributor content as data and shall not execute contributor code with repository write credentials.
- **AC-CI-PR-DOCS-003.4:** A merge group shall pass only when every included pull request meets its own coverage policy or has its own override. One pull request's artifacts or label shall not exempt another.
- **AC-CI-PR-DOCS-003.5:** Required-check rollout shall include evidence that ordinary PRs, label changes, forks, and merge groups report the expected result.

## Out of scope

- AI classification of feature intent or documentation quality.
- Enforcing the historical order of planning and implementation commits.
- Requiring public user documentation for every code change. Existing public-docs review obligations continue.
- Automatically changing repository rulesets, labels, merge queue membership, or PR #3137.

## Implementation plans

- [PR documentation coverage](../../../plans/pr-documentation-coverage/plan.md)
