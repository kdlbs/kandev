---
status: current
system: platform
requirements:
  - REQ-PLATFORM-GO-PRE-COMMIT-LINT-001
---

# Go pre-commit lint system design

## Purpose and boundaries

The local Go lint hook selects package targets from pre-commit's filenames.
Platform owns this contributor feedback contract alongside
[CI performance](ci-performance.md). Hosted workflow authorization remains
with the CI system.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-GO-PRE-COMMIT-LINT-001` | [Selection and execution](#selection-and-execution), [Repository context and failures](#repository-context-and-failures), [Verification boundaries](#verification-boundaries) |

## Selection and execution

`.pre-commit-config.yaml` passes repository-relative filenames to
`bash scripts/lint-go-changed` and sets `require_serial: true`. The existing
`^apps/backend/.*\.go$` filter controls eligibility. Pre-commit continues to
own staging, exclusion of removed files, and temporary handling of unstaged edits.

The wrapper accepts backend Go paths, converts each to its containing directory
relative to `apps/backend`, and deduplicates the package argument array. Root
files select `.`; nested files select their exact `./internal/example` package,
without a recursive `/...` suffix. Quoted arrays preserve argument boundaries.
An empty package array exits successfully before resolving the comparison base.

Each non-empty batch invokes one `golangci-lint run` with those packages,
`--new-from-rev` set to the existing resolver result, and `--timeout=5m`.
Pre-commit may split exceptionally large filename lists into serial batches.
Dependencies can still be loaded for type information.

## Repository context and failures

Before Git discovery, the wrapper clears `GIT_DIR`, `GIT_WORK_TREE`,
`GIT_COMMON_DIR`, `GIT_INDEX_FILE`, `GIT_OBJECT_DIRECTORY`, and
`GIT_ALTERNATE_OBJECT_DIRECTORIES`. It locates the checkout from the wrapper's
own path, then runs `scripts/resolve-go-lint-base --comparison-base` there.
Authentication and transport environment settings remain available.

The resolver retains configured-base, canonical-upstream, merge-base, and
incoming-base-merge behavior. A resolution failure exits before lint. The
wrapper uses `exec` for the linter so its diagnostics and failure status reach
pre-commit directly. There is no fallback to a different comparison base or to
backend-wide analysis.

## Verification boundaries

`scripts/lint-go-changed.test.py` exercises real Git and pre-commit with a
recording linter at the external process boundary. It covers staged selection,
package deduplication, root and nested packages, empty selections, unstaged-edit
restoration, linter failures, invalid bases, and inherited Git overrides.
`scripts/resolve-go-lint-base.test.sh` retains the existing resolver coverage.

`.github/workflows/backend-tests.yml` and `apps/backend/Makefile` continue to
select `./...`. Cross-package effects remain subject to that broader validation;
the local optimization is not a replacement for CI.
