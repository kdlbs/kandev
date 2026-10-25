---
id: "02-workflow-wiring"
title: "Wire retry into releases"
status: done
wave: 2
depends_on: ["01-retry-helper"]
plan: "plan.md"
spec: "../../specs/release/requirements/release-ghcr-secondary-limit.md"
---

# Task 02: Wire retry into releases

## Acceptance

- Every base/universal GHCR image build and manifest mutation uses the retry helper and preserves the digest output consumed by downstream jobs.
- Base and universal architecture jobs are sequential, and all downstream manifest and publication conditions require successful dependencies.
- Contract, action-pinning, and script-test workflow paths validate the new helper.

## Verification

```bash
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning.py
bash scripts/release/retry-ghcr-command.test.sh
bash -n scripts/release/retry-ghcr-command.sh
```

## Files likely touched

- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`
- `.github/workflows/lint-action-pinning.yml`
- `Makefile`

## Dependencies

Task 01.

## Parallelism

sequential

## Inputs

- `docs/specs/release/requirements/release-ghcr-secondary-limit.md`, especially What and Scenarios.
- `docs/plans/release-ghcr-secondary-limit/plan.md`, Wave 2.
- Existing Docker jobs and dependency assertions in `.github/workflows/release.yml` and `.github/scripts/release-workflow-contract_test.py`.

## Output contract

Report the workflow and contract changes, files changed, exact test results, any blocker, and the updated task/plan status in the primary session.

## Results

Replaced the four GHCR build action calls with retry-wrapped Buildx CLI commands, preserved BuildKit digest outputs, wrapped manifest mutations, and serialized each architecture pair.
Added the pinned GitHub runtime helper before each inline build so the existing GHA cache backend receives its runtime credentials.

- `python3 .github/scripts/release-workflow-contract_test.py` — passed, 28 tests.
- `python3 .github/scripts/lint-action-pinning.py` — passed, all 18 workflow files use SHA-pinned action refs.
- `python3 .github/scripts/lint-action-pinning_test.py` — passed, 9 tests.
- `bash scripts/release/retry-ghcr-command.test.sh` — passed.
- `bash -n scripts/release/retry-ghcr-command.sh` — passed.
