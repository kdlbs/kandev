---
spec: docs/specs/release/requirements/scoop-release-automation.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Automate Scoop Stable Releases

## Overview

Add an idempotent Scoop publisher before its Stable release job. Then extend
the release contract tests and maintainer documentation. The work uses the
published Windows checksum and a repository-scoped SSH deploy key.

The confirmed root cause has two parts. The Kandev release graph has no Scoop
publication job. The Scoop bucket contains `checkver` and `autoupdate` data,
but that repository has no automation that runs them.

## Release automation

### Scoop publisher

- Add `scripts/release/update-scoop-bucket.sh` with the interface
  `update-scoop-bucket.sh <version> <tag>`.
- Use `kdlbs/kandev` as the source repository and
  `kdlbs/scoop-kandev` as the default bucket repository.
- Download `kandev-windows-x64.tar.gz.sha256` from the exact GitHub Release.
- Accept only one lowercase 64-character SHA-256 value for the expected
  archive name.
- Clone the bucket into a temporary directory. Use
  `SCOOP_BUCKET_DEPLOY_KEY` for the CI SSH path.
- Use a temporary private-key file with mode `0600`. Pin the published GitHub
  SSH host keys and remove all temporary files on exit.
- Update only `version`, `architecture.64bit.url`, and
  `architecture.64bit.hash` in `bucket/kandev.json`. Keep all other manifest
  fields.
- Commit `kandev <version>` and push to `main`. Exit successfully without a
  commit when the manifest is already correct.
- Support a local authenticated fallback and repository override for the
  black-box test. The CI job still requires the deploy-key secret.
- Add `scripts/release/update-scoop-bucket.test.mjs`. Use temporary Git
  repositories and a fake `gh` executable. Cover a normal update, a no-op
  rerun, and a malformed checksum.
- Add the new test to the root `test-scripts` target.

### Stable workflow integration

- Add `update-scoop-bucket` to `.github/workflows/release.yml` as a sibling of
  `publish-npm` and `update-homebrew-tap`.
- Depend on `prepare` and `publish-release`. Require both jobs to succeed.
- Use the existing Stable-only condition. Exclude Nightly, dry runs, and
  Desktop validation runs. Keep `backfill_tag` eligible.
- Check out `github.workflow_sha` for this job. A backfill tag can predate the
  Scoop helper, so the job must use current release control logic.
- Pass the built-in `GITHUB_TOKEN` for release-asset reads. Pass the new
  `SCOOP_BUCKET_DEPLOY_KEY` repository secret for the cross-repository push.
- Fail with a clear error before publication when the deploy-key secret is
  absent.
- Add the helper and its test to the release-contract CI path filters and test
  command in `.github/workflows/lint-action-pinning.yml`.

## Release contract records

- Update `docs/public/release-process.md`. Add Scoop to the Stable target list,
  credential checklist, publication graph, verification list, and backfill
  guidance. Classify this page as a how-to guide.
- Update `.agents/skills/release/SKILL.md`. Add Scoop to version targets,
  sibling channels, Stable completion, and Nightly exclusions.
- Update the Release and Versioning section in `AGENTS.md` with the Scoop
  target and completion rule.
- Update `apps/cli/README_internal.md`. Describe Scoop as a third native bundle
  consumer and include its update command.
- Amend `docs/decisions/0029-release-backfill-and-desktop-diagnostics.md` so
  backfill reconciles the Scoop bucket with current workflow control logic.
- Amend `docs/decisions/2026-07-31-npm-nightly-release-channel.md` and
  `docs/specs/release/requirements/npm-nightly-channel.md` so Scoop remains Stable-only.
- Change the repair spec and its index entry to `shipped` after all tasks and
  checks are complete.

## Tests

- **What:** a valid Stable checksum produces the exact Scoop version, URL, and
  hash without changing unrelated fields.
  **File:** `scripts/release/update-scoop-bucket.test.mjs`.
  **How:** run the shell publisher against a temporary bucket and fake GitHub
  release download.
- **What:** a second publication for the same version creates no commit.
  **File:** `scripts/release/update-scoop-bucket.test.mjs`.
  **How:** run the publisher twice and compare the bucket branch commit.
- **What:** an absent or malformed checksum changes no bucket state.
  **File:** `scripts/release/update-scoop-bucket.test.mjs`.
  **How:** return invalid fixture data and assert a nonzero result with an
  unchanged remote commit.
- **What:** Scoop runs only after a successful Stable GitHub Release and stays
  eligible for backfill.
  **File:** `.github/scripts/release-workflow-contract_test.py`.
  **How:** add the job to Stable dependency, Nightly exclusion, and backfill
  contract tables.
- **What:** the workflow uses current control logic and a separate deploy key.
  **File:** `.github/scripts/release-workflow-contract_test.py`.
  **How:** assert `github.workflow_sha`, the secret name, its preflight, and the
  exact publisher arguments.
- **What:** release contract CI runs for Scoop helper changes.
  **File:** `.github/scripts/release-workflow-contract_test.py`.
  **How:** assert the path filters and Node test command in the lint workflow.

## E2E tests

No browser E2E test is required. This repair changes CI publication and
maintainer documentation. It does not change the Kandev UI or runtime.

## Post-merge rollout

1. Create a dedicated SSH key for `kdlbs/scoop-kandev`.
2. Add the public key to that repository as a write-enabled deploy key.
3. Store the private key as the `SCOOP_BUCKET_DEPLOY_KEY` Actions repository
   secret in `kdlbs/kandev`.
4. Reconcile the current Scoop manifest to the latest Stable release.
5. Verify the manifest version and hash. Then verify `scoop update kandev` on
   Windows.

Do not use the Homebrew deploy key for Scoop. Each deploy key has one target
repository.

## Verification results

Task 01:

- `node --test scripts/release/update-scoop-bucket.test.mjs` passed: 2 tests.
- `bash -n scripts/release/update-scoop-bucket.sh` passed.
- The task's `git diff --check` command passed.

Task 02:

- `python3 .github/scripts/release-workflow-contract_test.py` passed: 25 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` passed: 9 tests.
- The task's `git diff --check` command passed.

Task 03:

- `node --test scripts/validate-public-docs.test.mjs` passed: 58 tests.
- `node scripts/validate-public-docs.mjs` passed: 41 published pages.
- `python3 scripts/lint-harness-files.test.py` passed: 19 tests.
- `python3 .github/scripts/lint-harness-files.py --all` passed: 118 files.
- Focused harness pre-commit lint passed.

## Implementation waves and parallel candidates

Wave 1 (sequential):

- [x] [task-01-scoop-publisher](task-01-scoop-publisher.md)

Wave 2 (sequential, depends on Wave 1):

- [x] [task-02-release-workflow](task-02-release-workflow.md)

Wave 3 (sequential, depends on Wave 2):

- [x] [task-03-release-contract-docs](task-03-release-contract-docs.md)

Parallel delegation is not authorized by this plan. The work stays in the
primary session unless the user explicitly authorizes subagents.

## Open questions

None.
