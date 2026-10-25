---
id: "01-retry-helper"
title: "Add GHCR retry helper"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/release/requirements/release-ghcr-secondary-limit.md"
---

# Task 01: Add GHCR retry helper

## Acceptance

- The helper retries only recognized secondary-rate-limit or equivalent throttling output, using four total attempts and 60/120/240 second waits by default.
- Non-throttling command failures return immediately with the wrapped command's status; retry logs include attempt and delay information without secrets.
- The shell regression test proves transient recovery, persistent failure, and non-transient failure without real waits.

## Verification

```bash
bash scripts/release/retry-ghcr-command.test.sh
bash -n scripts/release/retry-ghcr-command.sh
```

## Files likely touched

- `scripts/release/retry-ghcr-command.sh`
- `scripts/release/retry-ghcr-command.test.sh`

## Dependencies

None.

## Parallelism

sequential

## Inputs

- `docs/specs/release/requirements/release-ghcr-secondary-limit.md`, especially What, Failure modes, and Scenarios.
- `docs/plans/release-ghcr-secondary-limit/plan.md`, Wave 1.
- Existing retry conventions in `scripts/release/npm-view-version.sh` and its test.

## Output contract

Report the helper behavior, files changed, exact test results, any blocker, and the updated task/plan status in the primary session.

## Results

Implemented `scripts/release/retry-ghcr-command.sh` and its regression test.

- `bash scripts/release/retry-ghcr-command.test.sh` — passed.
- `bash -n scripts/release/retry-ghcr-command.sh scripts/release/retry-ghcr-command.test.sh` — passed.
- No external side effects; the test uses disposable commands and a temporary directory.
