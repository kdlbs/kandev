---
status: draft
system: ci
requirements:
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-002
  - REQ-CI-PR-DOCS-003
---

# Pull request documentation coverage system design

## Ownership and mapping

CI owns the contributor coverage policy and its trusted execution. Application systems continue to own their specifications.

| Requirement | Design sections |
| --- | --- |
| REQ-CI-PR-DOCS-001 | Classification; Artifact contract |
| REQ-CI-PR-DOCS-002 | Events and overrides |
| REQ-CI-PR-DOCS-003 | Reporting and consistency; Merge queue; Security |

## Components

Proposed files:

- `.github/workflows/pr-docs.yml`: trusted orchestration, PR and merge-group entry points.
- `.github/scripts/pr-docs.cjs`: pure classification and reference validation, plus a bounded GitHub API adapter.
- `.github/scripts/pr-docs.test.cjs`: Node built-in test runner, with fake GitHub responses.
- `.github/scripts/pr-docs-workflow-contract_test.py`: workflow event, permission, and registration contract tests.

Register tests in `.github/workflows/lint-action-pinning.yml`, following the existing PR size workflow contract test pattern.

## Classification

Use an explicit exemption list. A changed path outside it requires coverage, including unknown roots and runtime configuration.
Evaluate both old and new paths for renames; deleting runtime code still requires coverage.

Initial exemptions:

- `docs/**` and Markdown files, including `AGENTS.md` and `CONTRIBUTING.md`.
- Go `*_test.go`; JS/TS `*.test.*` and `*.spec.*` restricted to JS/TS source extensions; `apps/web/e2e/**`.
- `apps/web/src/locales/<locale>/<namespace>.json`.
- Exact lock basenames `pnpm-lock.yaml`, `package-lock.json`, `yarn.lock`, `go.sum`, and `Cargo.lock`.

Do not exempt all JSON, YAML, assets, scripts, package manifests, Rust files, workflows, generated directories, or files containing the word `test`.
These can change shipped behavior or repository contracts. Add exemptions later only with concrete fixtures.
This conservative policy creates false positives for small runtime fixes and refactors. The explicit label is their escape hatch.

## Artifact contract

Select added or modified `docs/plans/<initiative>/task-<NN>-<slug>.md` files from the complete PR diff.
A pure rename with no content change does not qualify. Require at least one qualifying work order.
Read selected work orders and references at the exact PR head through the content API, as bounded text data.

Validate the repository's existing frontmatter fields: `id`, `title`, `status`, `wave`, `depends_on`, `plan`, `requirements`, `acceptance_criteria`, and `system_design`.
Require nonempty requirement, acceptance, and design lists. Resolve `plan` to the sibling `plan.md`; require a link back to the selected work order.
The plan's requirement/design references must cover the selected work order's references.
Resolve each design under `docs/specs/<system>/system-design/`. Each design
owns the subset of work-order requirements that it declares, and the union of
the referenced designs must cover every work-order requirement. Find each
owned requirement and acceptance ID in that design system's requirements
directory.
Check that AC IDs belong to referenced REQ IDs and that the design declares those REQ IDs.
Use a bounded parser for the documented frontmatter subset, not executable YAML tags or PR-supplied parsing code.

Referenced plans and specifications may already exist on the base branch. Require the work order update to record the current delivery scope and results.
Reject empty documents and references to deleted files. Allow pending work during PR development; do not require a completed implementation before a draft PR can pass structural coverage.
Semantic review determines whether changed behavior requires specification updates and whether the selected work order actually covers all changed code.
A handful of unrelated documentation changes does not establish the reference chain.

No PR-body schema is needed. The existing repository frontmatter supplies machine-readable links.
Update the PR template and contributor guide with examples and the distinction between structural coverage and semantic review.

## Events and overrides

Use `pull_request_target` events `opened`, `reopened`, `synchronize`, `edited`, `labeled`, `unlabeled`, and `ready_for_review` without path filters.
Read current PR metadata and labels, not just the event snapshot. Drafts follow the same policy.
The exact current label `no-docs-allow` returns an override success before expensive file reads.
Failure to read current metadata or labels is still an infrastructure error.

GitHub repository permissions control who applies the label. No separate author allowlist or title-based exception exists.
The override is PR-wide and persists across pushes. This is distinct from the revision-specific `ready-to-merge` contract.
Document that label actions made with `GITHUB_TOKEN` may not trigger another workflow; such automation must explicitly arrange a reevaluation.
Provide a `workflow_dispatch` PR-number input for retries, read current metadata, and validate that the PR belongs to this repository.

## Reporting and consistency

Publish one commit status context, `PR documentation coverage`, on the current PR head using `statuses: write`.
Do not rely on the native `pull_request_target` job check, whose execution identity is the base revision.
Use a distinct job name to avoid a status/check-name collision.
Set pending before evaluation; publish success, failure for missing coverage, or error for incomplete data.
The status target URL points to the run summary, which lists reasons, paths, references, and remediation.
Also fail the workflow job on policy failure or infrastructure error. Do not post PR comments.

Serialize all events by the normalized target branch with `queue: max` and
`cancel-in-progress: false`. GitHub retains up to 100 pending runs in this
shared lock; additional runs can be canceled when that bound is full. The
lock is intentionally broader than a PR-number or group-SHA lock so queued
label reevaluations cannot race with a merge-group evaluation for the same
target branch. Every surviving run reads current state.
Before publishing, reread the head, base, and label set. If changed, reevaluate with a bounded retry, then fail if state remains unstable.
Do not report stale successes for a new head. Metadata publication is eventually consistent; GitHub does not provide an atomic label-read/status-write transaction.

Paginate file lists and compare the count to current `changed_files`. Reject results at GitHub's 3,000-file cap and mismatched counts.
Reject truncated responses, invalid encodings, oversized documents, unsupported frontmatter, symlinks, submodules, and ambiguous IDs.
Bound artifact reads to 100 documents, 256 KiB each, and 4 MiB total. Report limits explicitly; never silently discard documents.

## Merge queue

Handle `merge_group: checks_requested` and publish the same status on `merge_group.head_sha`.
Load only trusted base code. Never evaluate coverage against the combined diff, where one PR could supply another's artifacts.

Resolve members using the paginated GraphQL merge queue entries, including each entry's `baseCommit`, `headCommit`, and `pullRequest`.
Find the entry for the event head and trace the entry commit boundaries back to the event base.
Require a complete, unambiguous chain; unknown membership produces an error, not success.
Evaluate each member's own current PR diff, artifact chain, and labels. Confirm queue identities and member heads again before publication.
Use the target-branch concurrency key shared with queued label reevaluation and
never cancel active group evaluation. The group status itself remains on the
synthetic group SHA.

Integration must validate the entry-boundary mapping against real GitHub merge-group payloads before requiring this status.
If GitHub cannot supply the expected chain for a supported queue mode, revise the mapping and fixtures before rollout.
Label removal while queued must also reevaluate every active group prefix
containing that PR, through the same group evaluator and serialization key.
Do not dequeue, requeue, or automatically merge PRs.

## Security

Permissions are `contents: read`, `pull-requests: read`, and `statuses: write` only.
Use SHA-pinned actions, GitHub-hosted runners, a trusted base checkout, and `persist-credentials: false`.
For dispatch, restrict the workflow definition to the default branch. For queue runs, pin consumed scripts to the event base SHA.
Do not install PR dependencies, execute PR files, or interpolate PR text into shell or JavaScript source.
Normalize artifact paths inside their allowed roots, bind content requests to exact repository/head identities, and escape summary text.

## Rollout and persistence

GitHub stores labels, statuses, and run summaries. No application storage or feature flag is needed.
First merge the workflow and contributor instructions. Exercise pass, failure, override, removal, fork, and merge-group cases.
Then add the status context to the `main` ruleset as a separate administrator action, preserving all existing checks and bypass rules.
Until required, it is a failing PR check but does not itself prohibit merge. Do not report ruleset enforcement as deployed before verification.
Existing open PRs need a supported event or dispatch to receive their first result.

## Related decisions and references

- [Documentation coverage policy](../../../decisions/2026-09-10-pr-documentation-coverage.md)
- [Existing merge queue contract](../../../ci-merge-queue.md)
- [GitHub workflow events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)
- [GitHub required status checks](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks)
- [GitHub merge queue API fields](https://docs.github.com/en/graphql/reference/pulls)
