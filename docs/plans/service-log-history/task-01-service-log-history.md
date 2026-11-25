---
id: "01-service-log-history"
title: "Select service log history"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CLI-SERVICE-LOG-HISTORY-001
acceptance_criteria:
  - AC-CLI-SERVICE-LOG-HISTORY-001.1
  - AC-CLI-SERVICE-LOG-HISTORY-001.2
  - AC-CLI-SERVICE-LOG-HISTORY-001.3
  - AC-CLI-SERVICE-LOG-HISTORY-001.4
  - AC-CLI-SERVICE-LOG-HISTORY-001.5
system_design:
  - ../../specs/cli/system-design/service-log-history.md
---

# Task 01: Select service log history

## Summary

Add bounded, log-only history selection to the native service command. Prove
entrypoint parsing and both service-manager argument paths without invoking
external service tools, and document operator usage.

## In scope

- Implement the three flag forms and strict ASCII decimal range 0..1000000;
  retain 200 by default and the last valid repeated count.
- Preserve user/system, follow, custom home, output and existing error paths.
- Update CLI help and the public CLI reference and service how-to.

## Out of scope

New filters, formats, configuration, Makefile controls, platforms, UI, live
service operations and unrelated edits.

## Acceptance

- Public entrypoint tests cover default, forms, boundaries, repeats, malformed
  values and all non-log actions; invalid input returns usage failure before an
  executor call. The new entrypoint test must fail before production changes.
- Controlled executor tests assert actual Linux and macOS handler argv,
  including default, zero follow, user/system and custom home. Existing normal
  journal controls continue to pass.
- Help and public docs describe syntax, range, default, zero-follow and macOS
  per-file semantics without changing unrelated sections.

## Verification

After a separate implementation release, from the repository root, use the
following in order. Install workspace dependencies exactly once in this fresh
workspace before any pnpm command or active hook. Each invoked command receives
its own quiet directory; preserve all process handles and wait for actual exits.
The first focused Go invocation is the RED run after writing the meaningful
entrypoint test and before production changes; its failure must demonstrate the
missing behavior. Run the second after implementation. Replace the named RED
test only if its chosen test name differs, and record that name and result.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2
(cd apps && KANDEV_RUN_QUIET_DIR="$(mktemp -d)" ../scripts/run-quiet install -- pnpm install --frozen-lockfile)
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d)" GOMEMLIMIT=512MiB ../../scripts/run-quiet backend-test -- go test -race -trimpath -tags fts5 -p 1 ./internal/launcher -run '^TestServiceLogHistoryEntrypoint$' -count=1)
(cd apps/backend && KANDEV_RUN_QUIET_DIR="$(mktemp -d)" GOMEMLIMIT=512MiB ../../scripts/run-quiet backend-test -- go test -race -trimpath -tags fts5 -p 1 ./internal/launcher -count=1)
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" GOMEMLIMIT=3GiB scripts/run-quiet go-lint -- bash scripts/lint-go-changed apps/backend/internal/launcher/service.go apps/backend/internal/launcher/service_test.go apps/backend/internal/launcher/service_logs_test.go
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" python3 scripts/list-docs.py validate
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" python3 scripts/lint-spec-files.test.py
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" python3 scripts/lint-spec-files.py --all
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" node --test scripts/validate-public-docs.test.mjs
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" node scripts/validate-public-docs.mjs
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" git diff --check -- docs/specs/cli docs/plans/service-log-history docs/public/cli.md docs/public/run-as-a-service.md apps/backend/internal/launcher
KANDEV_RUN_QUIET_DIR="$(mktemp -d)" git status --short
```

Review the catalog and cross-references, run any applicable repository PR-doc
coverage preflight, and record actual outcomes. Normal active hooks must run at
commit; do not bypass them. Do not run full suites or clean unrelated processes,
temporary files, or caches. Publication requires a fresh complete overlap check.

## Files likely touched

- `apps/backend/internal/launcher/service.go`
- `apps/backend/internal/launcher/service_test.go` (only if required)
- `apps/backend/internal/launcher/service_logs_test.go`
- `docs/public/cli.md`
- `docs/public/run-as-a-service.md`
- This plan, work order, and paired specification lifecycle statuses.

## Dependencies

None within this package. Separate ROOT design acceptance and HEAVY release are
required before implementation or checks. Publication and merge remain subject
to ROOT's separate review and grants.

## Risks

Directly constructed service arguments must preserve the default explicitly;
zero must not be an unset sentinel. Tests must restore injected globals and not
run in parallel. Preserve the disjoint concurrent `cli.md` maintenance edit.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/cli/requirements/service-log-history.md)
- [System design](../../specs/cli/system-design/service-log-history.md)
- `apps/backend/internal/launcher/service.go` and existing service tests.

## Results

- The entrypoint RED run returned exit 1 because `--lines 42` produced usage
  exit 2; the same test passed after implementation.
- The complete focused launcher package passed with race, `fts5`, `-trimpath`,
  and one package worker after the last code edit. No live manager was invoked.
- Frozen pnpm install completed once; changed Go lint passed with the stated
  file list and 3 GiB memory target.
- Specification catalog validation, specification linter tests and lint,
  public documentation validator tests and validation, and `git diff --check`
  passed. The local PR documentation coverage preflight also passed. Hosted CI,
  review and merge are separate delivery gates.
