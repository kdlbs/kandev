---
status: current
system: ci
requirements:
  - REQ-CI-PR-TRUST-001
  - REQ-CI-PR-TRUST-002
  - REQ-CI-PR-WALK-003
  - REQ-CI-PR-REVIEW-004
  - REQ-CI-PR-FAIL-005
---

# Unified contributor pull request automation system design

## Purpose and boundaries

This design defines the shared trust contract for base-controlled GitHub
Actions in the `kdlbs/kandev` repository. It covers fork pull request
authorization, event routing, and the boundaries between review, preview,
walkthrough generation, publication, and pull request linking.

The design does not change the review providers, the preview command, the
walkthrough skill, or the application. Those components keep their existing
contracts after the shared authorization gate changes.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-CI-PR-TRUST-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow) |
| `REQ-CI-PR-TRUST-002` | [Control flow](#control-flow), [Persistence and migration](#persistence-and-migration) |
| `REQ-CI-PR-WALK-003` | [Walkthrough flow](#walkthrough-flow), [Security](#security) |
| `REQ-CI-PR-REVIEW-004` | [Components and responsibilities](#components-and-responsibilities), [Security](#security) |
| `REQ-CI-PR-FAIL-005` | [Failure and recovery](#failure-and-recovery), [Security](#security) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `.github/workflows/claude-code-review.yml` | Adds the approval label for allowlisted contributors and runs the existing Claude fork review path. |
| `.github/workflows/opencode-code-review.yml` | Runs the existing OpenCode fork review path with durable `safe-to-review` authorization. |
| `.github/workflows/preview-env.yml` | Runs the existing privileged fork preview path. |
| `.github/workflows/pr-walkthrough.yml` | Generates, publishes, and links walkthroughs for authorized fork and same-repository pull requests. |
| `.github/scripts/*workflow_contract_test.py` | Protects job gates, labels, event filters, permissions, and trusted input provenance. |
| `scripts/opencode-code-review.test.sh` | Keeps the standalone OpenCode workflow test aligned with the persistent-label contract. |

## Data and contracts

### Approval and allowlists

`safe-to-review` is the only maintainer approval label for contributor review,
preview, and walkthrough automation. `safe-to-test` is removed from active
authorization expressions and from the automatic label bridge.

The label-based fork gate is:

```text
contains(github.event.pull_request.labels.*.name, 'safe-to-review')
```

The existing direct allowlists remain separate sources. The label bridge uses
`CLAUDE_REVIEW_ALLOWLIST`. The review and preview workflows retain their
existing `OPENCODE_REVIEW_ALLOWLIST`, `CLAUDE_REVIEW_ALLOWLIST`, and
`PREVIEW_ENV_ALLOWLIST` paths. The walkthrough uses
`CLAUDE_REVIEW_ALLOWLIST` directly so an allowlisted opening event remains
authorized even when a label written with `GITHUB_TOKEN` does not emit a new
workflow run.

### Event contract

- The walkthrough workflow keeps `pull_request_target` events for opened,
  ready-for-review, reopened, synchronize, and labeled actions.
- A `generate-pr-walkthrough` label still requests a same-repository manual
  rerun. It does not authorize a contributor pull request.
- The existing Claude workflow keeps its open and labeled fork review policy.
- The existing OpenCode and preview event sets remain unchanged except for the
  approval label expression.

### Walkthrough artifact contract

The walkthrough keeps the existing provider-neutral contract:

1. Generate `docs/pr-walkthrough/pr-<number>.json` and
   `docs/pr-walkthrough/pr-<number>.html` in the generation job.
2. Upload the files as a CI artifact.
3. Upload only HTML to the R2 bucket in the publication job.
4. Validate the public response and exact HTML bytes.
5. Add or replace the owned callout in the pull request body in the link job.

The public object key remains
`pr/<number>/<first-12-lowercase-head-sha>.html`.

## Control flow

### Trusted label path

1. A maintainer applies `safe-to-review` to a non-draft contributor pull
   request.
2. The base-controlled workflows receive the supported pull request event and
   evaluate the label at job level.
3. Review and preview use their existing fork paths. Walkthrough generation
   uses the trusted workflow checkout and the exact contributor head data.
4. Removing the label prevents later label-based runs. A contributor push does
   not remove the label automatically.

### Allowlisted contributor path

1. On an opened fork pull request, the Claude label job checks the author in
   `CLAUDE_REVIEW_ALLOWLIST` without checking out the pull request.
2. It adds only `safe-to-review`.
3. Each workflow keeps a direct allowlist expression where the resulting label
   event cannot be relied on. This includes the walkthrough opening and update
   path through `CLAUDE_REVIEW_ALLOWLIST`.
4. The direct path and the visible label converge on the same maintainer trust
   contract. No second preview label is needed.

### Walkthrough flow

1. The generation job checks out `github.workflow_sha` with credentials
   persisted as false and verifies the checkout is exactly that SHA.
2. It fetches `refs/pull/${PR_NUMBER}/head` into the Git object database and
   verifies that `FETCH_HEAD` resolves to the event `HEAD_SHA`. It does not
   check out that ref.
3. The trusted context helper computes the merge base, patch, manifest, and
   bounded regular UTF-8 files from the immutable head object.
4. The agent reads trusted instructions plus that bounded context, edits only
   `.pr-walkthrough/draft.json`, and invokes the fixed trusted renderer.
5. Publication and linking run in separate jobs with their existing minimal
   permissions and credentials.

## Failure and recovery

- Missing, malformed, or non-matching allowlists fail closed.
- A missing `safe-to-review` label skips contributor jobs. A stale
  `safe-to-test` label has no authorization effect after migration.
- A failed label write does not grant access and does not block direct
  allowlist paths.
- A missing or mismatched pull request ref fails walkthrough context
  preparation before the agent starts. The exact SHA check prevents a result
  from being associated with a different head.
- Existing walkthrough retry, artifact, R2 validation, and PR-body validation
  behavior remains unchanged.
- Contract tests fail when the old label appears in an active authorization
  expression or when a privileged job loses its trust or permission boundary.

## Persistence and migration

Approval labels remain until a maintainer removes them. The rollout changes
workflow expressions and the allowlist label bridge first. After the changed
workflows are active, operators remove existing `safe-to-test` labels from open
pull requests and remove the repository label definition when no other active
workflow references it. Existing `safe-to-test` labels are inert before that
cleanup.

The `PR_WALKTHROUGH_ENABLED` variable remains independent. The
`generate-pr-walkthrough` label remains an operational same-repository rerun
trigger.

## Security

- All workflow files, helper scripts, and walkthrough skill files used by the
  secret-bearing generation job come from `github.workflow_sha`.
- The walkthrough generation job fetches contributor data only as Git objects
  and materializes bounded files. It does not execute contributor scripts or
  use contributor-controlled workflow, skill, setup, or link helpers.
- Walkthrough generation receives the existing model credential but no R2
  credentials or pull request write permission.
- The review workflows retain their existing read-only agent policies and
  advisory posting boundaries.
- The preview workflow intentionally reuses the existing privileged fork path.
  That path checks out the contributor head and runs `go run ./cmd/preview
  deploy` with `SPRITES_API_TOKEN` and `GITHUB_TOKEN`. `safe-to-review` is an
  explicit maintainer trust decision for that execution.
- Publication receives only the bucket-scoped R2 credentials needed for its
  upload. Linking receives only the existing pull request write permission and
  trusted helper.
- The automatic label job has no checkout and only the issue-label write
  permission it needs.

## Observability

- Existing workflow run status, job summaries, artifact diagnostics, R2 URL
  validation, and PR-body marker behavior remain the operational signals.
- Contract tests provide local evidence for every authorization expression,
  event filter, trusted checkout, exact SHA check, and sensitive permission.
- Post-rollout verification must inspect a labeled fork run, an allowlisted
  fork run, an unauthorized fork run, a follow-up push, and a revoked-label
  run.

## Related decisions

- [Use one maintainer approval label for contributor PR automation](../../../decisions/2026-08-24-unified-fork-approval-label.md)
- [Persist Fork Approval Labels Across Pushes](../../../decisions/2026-08-22-persistent-fork-approval-labels.md)
- [Use the Claude Allowlist as a Trusted Preview Gate](../../../decisions/2026-08-07-claude-allowlist-label-bridge.md)
- [Use a Filesystem Contract for PR Walkthrough Runners](../../../decisions/2026-08-22-pr-walkthrough-filesystem-runner.md)
- [Use the workflow SHA for trusted PR walkthrough inputs](../../../decisions/2026-08-23-pr-walkthrough-workflow-provenance.md)
- [Host PR walkthrough HTML in Cloudflare R2](../../../decisions/2026-08-22-pr-walkthrough-r2-hosting.md)
