---
status: active
system: ci
created: 2026-08-24
owners:
  - kandev
---

# Unified contributor pull request automation requirements

## Overview

Maintainers need one clear approval action for trusted contributor pull
requests. The approval must enable the existing AI review, preview deployment,
and pull request walkthrough workflows without asking maintainers to remember
which label controls each capability.

The approval applies to fork pull requests. Same-repository pull requests keep
their current workflow behavior.

## Terminology

- **Contributor pull request:** A pull request whose head repository differs
  from the base repository.
- **Approval label:** The maintainer-applied `safe-to-review` label.
- **Direct allowlist:** An existing repository variable that names trusted
  contributor logins. Direct allowlists remain independent authorization paths.
- **Privileged preview path:** The existing fork preview job that checks out the
  contributor head and runs `go run ./cmd/preview deploy` with its current
  deployment credentials.

## Requirements

### REQ-CI-PR-TRUST-001: One contributor approval label

**Intent:** Give maintainers one visible approval control for the three
contributor pull request automation capabilities.

#### Acceptance criteria

- **AC-CI-PR-TRUST-001.1:** When a non-draft contributor pull request has the
  `safe-to-review` label, the applicable base-controlled review, preview, and
  walkthrough jobs shall be eligible on their supported events.
- **AC-CI-PR-TRUST-001.2:** When a contributor pull request has only the
  `safe-to-test` label, no review, preview, or walkthrough job shall use that
  label as authorization.
- **AC-CI-PR-TRUST-001.3:** When a contributor pull request has neither
  `safe-to-review` nor an applicable direct allowlist authorization, the three
  privileged capabilities shall remain unavailable.
- **AC-CI-PR-TRUST-001.4:** When a maintainer removes `safe-to-review`, a later
  contributor push shall not regain label-based authorization.

### REQ-CI-PR-TRUST-002: Durable and allowlisted contributor trust

**Intent:** Preserve the existing trusted-contributor workflow while making
the label bridge reliable across pushes and GitHub event behavior.

#### Acceptance criteria

- **AC-CI-PR-TRUST-002.1:** When a contributor push updates an approved pull
  request, `safe-to-review` shall remain valid until a maintainer removes it.
- **AC-CI-PR-TRUST-002.2:** When a contributor is in
  `CLAUDE_REVIEW_ALLOWLIST`, the base-controlled label job shall add only
  `safe-to-review` on the supported opening event.
- **AC-CI-PR-TRUST-002.3:** When the label job writes a label with
  `GITHUB_TOKEN`, the review, preview, and walkthrough gates shall still have
  a direct allowlist path where needed. They shall not depend on a recursive
  `labeled` workflow run.
- **AC-CI-PR-TRUST-002.4:** Empty, malformed, or non-matching direct allowlists
  shall fail closed.

### REQ-CI-PR-WALK-003: Walkthroughs for approved contributor pull requests

**Intent:** Give maintainers a visual explanation of trusted contributor
changes, including the changes that previously skipped walkthrough generation.

#### Acceptance criteria

- **AC-CI-PR-WALK-003.1:** When walkthrough generation is enabled and a
  non-draft contributor pull request is authorized by `safe-to-review` or the
  existing contributor allowlist bridge, the walkthrough workflow shall be
  eligible on opened, ready-for-review, reopened, synchronize, and approval
  label events supported by the workflow.
- **AC-CI-PR-WALK-003.2:** A successful contributor run shall preserve the
  current walkthrough contract: non-empty JSON and HTML artifacts, validated
  publication to R2, and one owned callout in the pull request description.
- **AC-CI-PR-WALK-003.3:** Walkthrough generation shall use trusted workflow
  assets and bounded immutable contributor-head data. It shall not check out or
  execute contributor code in the secret-bearing generation worktree.
- **AC-CI-PR-WALK-003.4:** The workflow shall verify that fetched contributor
  data matches the exact event head SHA before preparing context or publishing
  a result.

### REQ-CI-PR-REVIEW-004: Existing review and preview capabilities

**Intent:** Extend the existing trusted fork paths without creating a second
review or preview implementation.

#### Acceptance criteria

- **AC-CI-PR-REVIEW-004.1:** When a contributor pull request is authorized by
  `safe-to-review`, the existing Claude and OpenCode fork review workflows shall
  retain their current provider-specific event and permission contracts.
- **AC-CI-PR-REVIEW-004.2:** When a contributor pull request is authorized by
  `safe-to-review`, the preview workflow shall reuse its existing privileged
  fork path, including its current head checkout and deployment credentials.
- **AC-CI-PR-REVIEW-004.3:** Existing direct review and preview allowlists shall
  remain valid as independent authorization sources.

### REQ-CI-PR-FAIL-005: Fail closed and preserve unrelated behavior

**Intent:** Prevent the label consolidation from widening unrelated workflow
permissions or changing same-repository automation.

#### Acceptance criteria

- **AC-CI-PR-FAIL-005.1:** When a workflow receives an unauthorized contributor
  event, its privileged job shall be skipped before any contributor code or
  secret-bearing action runs.
- **AC-CI-PR-FAIL-005.2:** Same-repository pull request behavior, the
  independent `PR_WALKTHROUGH_ENABLED` toggle, and the
  `generate-pr-walkthrough` same-repository rerun label shall remain unchanged.
- **AC-CI-PR-FAIL-005.3:** The workflow contract tests shall reject active
  authorization expressions that reference `safe-to-test`.

## Out of scope

- Redesigning the preview executor or isolating its existing privileged fork
  deployment path.
- Changing review provider prompts, models, or follow-up review policies.
- Removing `CLAUDE_REVIEW_ALLOWLIST`, `OPENCODE_REVIEW_ALLOWLIST`, or
  `PREVIEW_ENV_ALLOWLIST`.
- Adding a Kandev application walkthrough feature.
- Making `generate-pr-walkthrough` a contributor trust label. It remains an
  operational rerun label for same-repository pull requests.
