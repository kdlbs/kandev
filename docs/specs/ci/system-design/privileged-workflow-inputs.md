---
status: current
system: ci
requirements:
  - REQ-CI-ACTIONS-SECURITY-001
  - REQ-CI-ACTIONS-SECURITY-002
---

# Privileged workflow input design

## Ownership and current evidence

CI owns the boundary between workflow execution and external input. Release
continues to own channels, signing, publication, and backfill. This design
extends the [shared contributor trust contract](unified-contributor-pr-automation.md).
It changes input handling, not contributor authorization.

The [finding ledger](../../../plans/github-actions-security/findings.md) records
the source revision, scanner results, upstream action behavior, and open-PR check.
The implementation uses a shared `.github/scripts/prepare-pr-review.sh` helper
for history verification, exact diff preparation, and bounded context.

| Requirement | Design sections |
| --- | --- |
| REQ-CI-ACTIONS-SECURITY-001 | Trusted review workspace, Claude fork review, OpenCode review, Review failure behavior |
| REQ-CI-ACTIONS-SECURITY-002 | Publication data, Git authentication, Documentation preview, Preserved boundaries |

## Trusted review workspace

The Claude fork job and both OpenCode jobs check out `github.workflow_sha`
with `persist-credentials: false`. They assert the checkout SHA before any
privileged action. `github.workflow_ref`, `github.workflow_sha`, the checkout
SHA, the event base SHA, and the event head SHA remain distinct identities.

Fetch the event PR head as a Git object through the base repository's pull ref.
Verify its full SHA against the event before preparing context. Fetch enough
history for the original `BASE_SHA...HEAD_SHA` diff. Do not check out the head.
Use fixed Git arguments and validated numeric PR/full-SHA values.

Use the existing walkthrough history and bounded context implementations as
patterns. Reuse `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-context`
for review and walkthrough changed-file materialization.
Its inputs are the event base/head, and its output is beneath
`.opencode-review/head-context`. The helper accepts regular Git blobs and
enforces 512 KiB per file and 8 MiB total. Symlinks and gitlinks remain omitted.
The manifest distinguishes omitted files from empty files. Its version 2 entries
retain each original `path` and map materialized files through `context_path`,
relative to the manifest directory. Store every blob under the flat neutral
name `head/files/<Git-object-ID>.txt`. Identical blobs may share a physical file.
Do not preserve original directories, instruction names, or configuration names
in the prepared filesystem. Agents use the mapping for exact excerpts and keep
the original path for findings. All workflow consumers use this shared layout.

The runner never changes its working directory into prepared head data.
Materialized configuration and instruction files are text for review. Neutral
names prevent automatic discovery of `AGENTS.md`, `CLAUDE.md`, and `CONTEXT.md`
beside source files. Context creation occurs before model credentials
are introduced. Existing read-only local tools can inspect trusted surrounding
code. A missing file must not cause fallback to an untrusted checkout.

## Claude fork review

Retain the existing action pin, author gates, `allowed_non_write_users`, token
choice, tracking behavior, and comment tools. Keep the explicit `prompt` so
the pinned action selects agent mode, which does not switch the checkout.
Replace the instruction to use the PR-owned review skill with the trusted skill.

Use the manual-review pattern in `claude.yml`: explicit read tools, narrow
GitHub comment commands, the GET-only `.github/scripts/claude-read-pr-file.py` helper,
and `--permission-mode dontAsk`. Do not permit generic `gh api`, Python, or shell
execution. Keep the existing repository-mutation and arbitrary-network denials.

Add an optional `CLAUDE_REVIEW_HEAD_SHA` to the existing file helper. When set,
validate the SHA, require the current PR head to match, and retrieve the file
at that SHA. A mismatch fails closed. When absent, retain the manual caller's
current-head behavior. Capture the automatic diff before the agent starts.
Do not grant automatic review a live `gh pr diff` command that can replace this
input with a later head. Bind the automatic prompt and posting context to the
captured event head. The manual review caller retains its existing commands.

The pinned action restores sensitive `.claude` paths from the base branch
before CLI execution. This is a useful upstream mitigation, but it does not
restore `.agents/skills/code-review`. The design removes dependency on PR-owned
instructions instead of treating a hooks exploit as proven.

## OpenCode review

Keep the job IDs, protected environment on the same-repository job, model,
full-PR diff range, terminal review markers, and artifact names. Load the setup
action and posting helper from the trusted workflow checkout. The event base
still determines the diff, even if it differs from the workflow SHA.

Create the review agent from trusted workflow text. As in the walkthrough,
clear `.opencode`, `opencode.json`, and `opencode.jsonc` before creating that
agent. Do not load any root configuration from the PR. Keep permissions denied
by default, with only read/glob/grep needed for trusted and prepared context.
Deny shell, edits, subagents, external directories, network tools, and custom
tools. Do not change provider or model selection in this work.

The model step receives only its existing provider credential. Mint the
dedicated App token after the model exits. The protected same-repository job
keeps its current App identity check. The fork job keeps `github.token` advisory
posting. Preserve exact-head, run-ID, attempt-ID, malformed-output, and failed-model
handling in `scripts/opencode-code-review` and its tests.

## Review failure behavior

A head mismatch or unsafe context preparation fails before model execution.
It must not produce clean evidence. A model or parser failure keeps the existing
non-clean diagnostic path. Neither an omitted file nor a missing output becomes
a successful clean review. Tool denials supplement trusted input selection.
They do not prevent startup plugin execution on an untrusted checkout.

## Publication data

Replace the 20 reported `release.yml` expressions inside shell programs with
step environment variables and quoted shell expansions. Replace the weekly
date expression in `universal-rebuild.yml` the same way. Keep action `with`
expressions and job conditions intact.

Use `GITHUB_REPOSITORY` and `GITHUB_WORKFLOW_SHA` for helper API paths. Keep
helpers sourced from the workflow revision, even when application inputs use a
backfill tag. Preserve the diagnostics helper's current local-file/fallback
selection and optional failure behavior. Never substitute the PR head or moving
`main` for the workflow SHA.

Pass release versions, tags, and nightly versions through step environment.
Use quoted arguments, including `"${RELEASE_TAG}^{commit}"` when deriving the
updater date. Do not use `eval`, interpolate data into script bodies, or change
the validated SemVer/nightly producers. Stub publication commands in tests.
Hostile quotes, command substitutions, backticks, newlines, and spaces must stay
data or be rejected by existing validation. No test may contact publication APIs.

## Git authentication

Set `persist-credentials: false` on all eight flagged checkouts. Seven do not
need authenticated Git writes. `release.yml`'s `prepare` job needs branch delete,
branch push, and signed-tag pushes.

For each required push, use command-local Git configuration that clears inherited
credential helpers and selects the fixed `gh auth git-credential` helper. Supply
the ordinary `GH_TOKEN` only on the existing branch-write and signed-tag steps.
Do not put token values in command arguments, remote URLs, files, or logs.
Do not run `gh auth setup-git`, which would retain authentication across steps.
Public fetches remain anonymous. Keep tag retries, merge-commit checks, and
recovery instructions. The bypass token remains only on the administrator merge.

Checkout v6 stores its credential file under `RUNNER_TEMP`. Therefore these
findings do not prove token inclusion in `.git` or uploaded artifacts. Disabling
persistence reduces credential availability to later steps regardless of location.

## Documentation preview

Remove `cache: pnpm` and its cache dependency path from the preview Node setup.
Set `package-manager-cache: false` explicitly. Keep frozen landing dependency
installation, the same-repository PR gate, landing `main` checkout, scoped deploy
key, pinned Wrangler action, and separate comment job. Do not introduce a new
cache namespace or a replacement cache action.

The scanner identifies a cache restore in a publication job. It does not prove
that a fork can poison this cache. The current preview rejects fork heads.
GitHub cache scope also limits which refs can restore a cache. Removing this
unnecessary dependency is a narrow hardening measure.

## Preserved boundaries

Retain all six flagged `pull_request_target` triggers. The walkthrough and
reconciliation workflows keep trusted workflow checkouts and bounded data.
The preview keeps separate fork build, packaging, and trusted deployment jobs.
The merge-approval revoker remains metadata-only with no checkout.

Keep Windows CMD launchers. They call fixed PowerShell test paths without
expression interpolation. Keep the pinned `softprops/action-gh-release`
publication action. Its replacement recommendation is not a security defect.

Normal Stable, dry-run, desktop-validation, backfill, and scheduled/manual
Nightly cases must retain each job's current eligibility and dependencies.
GitHub release, npm, Homebrew, Scoop, containers, contributor notices, browser
demo notification, and Discord gates must each remain covered. No live dispatch
is needed to validate these changes.

Record reviewed residual findings without adding scanner suppressions in this
package. Expected residuals on the recorded baseline are six trigger warnings,
three CMD limitations, and one action-replacement suggestion. A changed scanner
count requires explanation, not an automatic suppression.

## Related decisions

- [Automatic review input isolation](../../../decisions/2026-10-25-automatic-review-input-isolation.md)
- [Manual review input isolation](../../../decisions/2026-07-31-isolate-manual-pr-review-content.md)
- [Current OpenCode evidence trust](../../../decisions/2026-07-23-opencode-review-evidence-trust.md)
- [Release backfill provenance](../../../decisions/0029-release-backfill-and-desktop-diagnostics.md)

## Implementation plans

- [GitHub Actions security findings](../../../plans/github-actions-security/plan.md)
