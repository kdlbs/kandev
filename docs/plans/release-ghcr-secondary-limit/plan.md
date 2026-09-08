---
spec: docs/specs/release/requirements/release-ghcr-secondary-limit.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Resilient GHCR release publishing

## Overview

Run #132 confirmed that the ARM64 `docker/build-push-action` can receive a 403 secondary-rate-limit response from GHCR after the image layers are built. The repair adds one tested local retry helper, uses it for GHCR image and manifest mutations, and serializes the two architecture publish stages so concurrent registry writes are reduced. The release recovery guide will document the safe rerun and backfill paths.

## Release workflow

- Add `scripts/release/retry-ghcr-command.sh`, a command wrapper with four total attempts, 60/120/240 second exponential waits, streamed output, transient-response matching, and preserved exit status.
- Add a shell regression test with a fake command that covers transient-then-success, persistent transient failure, and immediate non-transient failure. Override delays in the test so it never sleeps for production intervals.
- Replace the four per-architecture `docker/build-push-action` invocations in `.github/workflows/release.yml` with equivalent `docker buildx build` commands wrapped by the helper. Use BuildKit metadata output to preserve each job's `digest` output.
- Run the pinned GitHub runtime helper before each inline Buildx build so the existing `type=gha` cache settings receive their runtime token and URL.
- Wrap the base manifest `docker buildx imagetools create` and universal promotion command with the same helper.
- Make the base ARM64 job depend on the successful base AMD64 job, and make the universal ARM64 job depend on the successful universal AMD64 job. Update their conditions and the contract test's dependency map.
- Add the new helper and test to the release validation paths in `.github/workflows/lint-action-pinning.yml` and `Makefile`.

## Tests

- **What:** transient registry errors retry with the specified bounded backoff, permanent errors fail immediately, and final status is preserved.
  **File:** `scripts/release/retry-ghcr-command.test.sh`
  **How:** execute the real helper against deterministic fake commands with test-only delay overrides.
- **What:** all GHCR mutating workflow steps use the helper, retain digest outputs, serialize architecture stages, and keep downstream success gates.
  **File:** `.github/scripts/release-workflow-contract_test.py`
  **How:** extend the existing workflow contract tests with exact helper, output, dependency, and condition assertions.
- **Targeted checks:**
  ```bash
  bash scripts/release/retry-ghcr-command.test.sh
  bash -n scripts/release/retry-ghcr-command.sh
  python3 .github/scripts/release-workflow-contract_test.py
  python3 .github/scripts/lint-action-pinning.py
  ```

## Documentation

Update `docs/public/release-process.md` in the partial-release recovery section with the secondary-limit procedure: wait for the throttle, rerun failed jobs, avoid a new normal bump after the tag exists, and use `backfill_tag` only for the existing latest tag.

## Verification Results

- `bash scripts/release/retry-ghcr-command.test.sh` — passed.
- `bash -n scripts/release/retry-ghcr-command.sh scripts/release/retry-ghcr-command.test.sh` — passed.
- `python3 .github/scripts/release-workflow-contract_test.py` — passed, 28 tests.
- `python3 .github/scripts/lint-action-pinning.py` — passed, all 18 workflow files use SHA-pinned action refs.
- `python3 .github/scripts/lint-action-pinning_test.py` — passed, 9 tests.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 60 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published docs pages.
- `zizmor .github/workflows` — audit completed with pre-existing findings outside this repair; no unrelated workflow changes were made.

## Implementation Waves And Parallel Candidates

Execute sequentially in the primary session:

### Wave 1

- [x] [task-01-retry-helper](task-01-retry-helper.md)

### Wave 2

- [x] [task-02-workflow-wiring](task-02-workflow-wiring.md)

### Wave 3

- [x] [task-03-release-recovery-docs](task-03-release-recovery-docs.md)

## Open Questions

None.
