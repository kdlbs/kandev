---
status: current
system: release
requirements:
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003
---

# Release Contributor Notifications System Design

## Purpose and boundaries

One reusable notification workflow supports manual runs and the existing Stable release pipeline.
It uses the published release notes as the candidate source and GitHub PR metadata as the recipient authority.
Publication success remains governed by the existing release contract.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001` | Workflow inputs and release resolution |
| `REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002` | Recipient selection, duplicate prevention, failure and recovery |
| `REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003` | Release integration |

## Components

- `.github/workflows/notify-release-contributors.yml`: Reusable workflow with manual `workflow_dispatch` and caller `workflow_call` entry points.
- `.github/scripts/notify-release-contributors.py`: Python standard-library helper. It uses `gh api` through argument arrays for authenticated GitHub operations.
- `cliff.toml`: Existing source of the maintainer list in the changelog template.
- `.github/workflows/release.yml`: Existing publication pipeline; adds the unchecked input and a dependent reusable-workflow job.

No new package dependency, personal token, or organization membership permission is necessary.
The helper reads the existing Tera `set maintainers` declaration from the TOML changelog body.
It requires one declaration containing a JSON-compatible string list and fails if that contract changes.
Tests bind attribution and notification filtering to this one source.

## Workflow inputs and release resolution

Both entry points expose `release_tag` as an optional string, default empty.
Both expose `dry_run` as a boolean, default false.
The manual form describes the empty value as latest published Stable release.

Trim input whitespace. An empty value calls `GET /repos/{owner}/{repo}/releases/latest` once.
A supplied value calls the release-by-tag endpoint with the tag encoded as a path segment.
Reject drafts, prereleases, missing `published_at`, and non-Stable tag identities before writes.
Use `vX.Y.Z`, with the existing historical `vX.Y` normalization if encountered.
Never resolve a missing explicit tag through the latest endpoint.
Bind release ID, tag, notes, URL, and publication time for the entire run.

Checkout executable helpers at `github.workflow_sha` with `persist-credentials: false` and complete Git history.
Fetch the validated release tag if necessary and resolve its commit separately.
An older release or backfill must not execute helper code from that release tag.
Log workflow definition identity and checkout identity separately, as required by `.github/AGENTS.md`.

## Recipient selection

1. Extract and deduplicate positive PR numbers from same-repository GitHub PR links in release notes.
2. Resolve each candidate through the PR API. A known non-PR reference is skipped with a reason.
3. Require a merged PR, matching base repository, and an available merge commit.
4. Prove its merge commit is an ancestor of the selected tag with `git merge-base --is-ancestor`.
5. Exclude bot accounts by API user type and login suffix. Exclude maintainers case-insensitively.
6. Read every page of conversation comments before deciding whether to post.

Do not treat every parenthesized number in release notes as proof of an included PR.
The v0.97.0 notes contain references that do not resolve to PRs; include this shape in fixtures.
An unexpected permission, transport, or Git ancestry error is a failure, not an empty recipient list.

## Comment identity and duplicate prevention

The visible comment is:

```text
This PR was included in [Kandev <tag>](<release URL>). Thanks for contributing!
```

Append the hidden marker `<!-- kandev-release-notice:<release ID> -->` on its own line.
Compare markers by exact release identity. Also recognize the exact visible comment without a marker.
This preserves the 40 manually posted v0.97.0 notices.
Count a marker or exact visible comment only when the API author is `github-actions[bot]` with type `Bot`, or a listed maintainer with type `User`.
This preserves trusted v0.97.0 notices and prevents an unrelated commenter from suppressing a notice.
Use exact comparisons rather than a broad search for the word release.

Use one repository-wide notification job concurrency group with `cancel-in-progress: false` and `queue: max`.
Place it on the called workflow's posting job so standalone and release-triggered runs share the same lock.
Do not reuse the existing release-wide concurrency group, which the caller holds.
Serialize POST operations and leave at least one second between them to reduce secondary rate limits.
Before each POST, check existing notices; after each success, verify the returned body and comment identity.
For an uncertain POST result, reread conversation comments before another attempt or a rerun.

## Release integration

Add `notify_contributors` as a boolean input with default false in `release.yml`.
Add a job calling the local reusable workflow after these dependencies:
`prepare`, `publish-release`, `publish-npm`, `update-homebrew-tap`, and `update-scoop-bucket`.

Its condition includes `!cancelled()`, manual Stable mode, the checked input,
and explicit success results for every dependency.
It also excludes dry runs and Desktop validation.
Pass `needs.prepare.outputs.tag` as `release_tag` and `dry_run: false`.
The same gate permits a successful Stable backfill and excludes Nightly.
Cancellation before this job starts skips notices. Cancellation after serial posting begins can leave a partial set, which the manual exact-tag workflow can safely complete.

Use a direct reusable-workflow call instead of `release: published`.
Releases created with `GITHUB_TOKEN` do not reliably start a separate release-event workflow.
The shared script supports manual recovery without another publication run.

## Security

Give the notification job `contents: read` and `pull-requests: write` only.
The caller declares the same permissions for the reusable-workflow job.
The built-in job token posts comments as `github-actions[bot]`.
Pass inputs through environment variables or structured arguments; never interpolate them into shell code.
Never checkout PR heads, execute release-note content, or send credentials to release-note URLs.
Construct GitHub API endpoints and comment URLs from the bound repository and release identity.

## Failure and recovery

Preflight release resolution, maintainer parsing, candidate metadata, and tag ancestry before the first write.
Preview performs the same reads and produces the same planned notices without POST requests.
Keep a result for each PR. Continue independent PRs after a confirmed individual failure, then fail the job.
Stop writes on global authentication failure or rate limiting, report remaining PRs as unprocessed, and fail the job.
Successful notices are the durable record; no separate database or artifact tracks completion.
A repeat run rereads comments and skips successful notices.
Cancellation after posting begins can leave partial results; recovery uses the same idempotent exact-tag path.

## Observability

The step summary contains the release link, comment preview, and per-PR result links or skip reasons.
It includes posted, already notified, excluded, failed, and unprocessed counts.
Dry runs distinguish planned notices from posted notices.
Escape metadata in summaries and never print tokens.

## References

- [GitHub release API](https://docs.github.com/en/rest/releases/releases#get-the-latest-release)
- [PR conversation comment API and permissions](https://docs.github.com/en/rest/issues/comments#create-an-issue-comment)
- [Reusable workflow inputs and permissions](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows)
- [Workflow event suppression](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow)
- [Stable release guide](../../../public/release-process.md)
