---
id: "01-scoop-publisher"
title: "Add Scoop publisher"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/release/requirements/scoop-release-automation.md"
---

# Task 01: Add Scoop publisher

## Acceptance

- The publisher reads the exact Windows checksum and updates only the three
  version-dependent Scoop manifest fields.
- A repeated publication creates no commit. An absent or malformed checksum
  fails before the remote bucket changes.
- The CI SSH path protects and removes the deploy key, pins GitHub host keys,
  and pushes only to `kdlbs/scoop-kandev`.

## Verification

- `node --test scripts/release/update-scoop-bucket.test.mjs`
- `bash -n scripts/release/update-scoop-bucket.sh`
- `git diff --check -- scripts/release/update-scoop-bucket.sh scripts/release/update-scoop-bucket.test.mjs Makefile`

## Files likely touched

- `scripts/release/update-scoop-bucket.sh`
- `scripts/release/update-scoop-bucket.test.mjs`
- `Makefile`

## Dependencies

None.

## Parallelism

Sequential. The workflow task depends on the final publisher interface.

## Inputs

- The `What`, Permissions, Failure modes, and Scenarios sections in the repair
  spec.
- `scripts/release/update-homebrew-tap.sh` for the deploy-key, host-key, Git,
  and cleanup pattern.
- The current `bucket/kandev.json` contract in `kdlbs/scoop-kandev`.
- `scripts/release/nightly-release.test.mjs` for fake-command and temporary Git
  fixture patterns.

## Output contract

Report the changed files, Red and Green test results, cleanup evidence, and
security boundaries. Update this task and `plan.md` in the same conversation.

## Results

Red:

- `node --test scripts/release/update-scoop-bucket.test.mjs` failed before the
  helper existed, proving the fixture exercised the missing behavior.

Green:

- `node --test scripts/release/update-scoop-bucket.test.mjs` passed: 2 tests,
  2 passes.
- `bash -n scripts/release/update-scoop-bucket.sh` passed.
- `git diff --check -- scripts/release/update-scoop-bucket.sh
  scripts/release/update-scoop-bucket.test.mjs Makefile` passed.

The helper uses `SCOOP_BUCKET_DEPLOY_KEY` only for the cross-repository SSH
path. It writes the key and pinned GitHub host keys with mode `0600`, uses an
isolated `GIT_SSH_COMMAND`, and removes the key, known-hosts file, and temporary
clone directory from its `EXIT` cleanup trap. The local fallback uses the
authenticated `gh` CLI and the repository override is covered by the fixture.
