---
id: "01-darwin-identity"
title: "Repair Darwin process ownership verification"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 01: Repair Darwin process ownership verification

## Summary

Recognize owned macOS processes and prove termination of their complete isolated
session without kernel pointers. Preserve old persisted birth tokens and the
existing recovery admission rules.

## In scope

- Birth-token compatibility, live identity inspection, and absent-PID handling.
- Numeric-session descendant enumeration and existing containment behavior.
- Real macOS subprocess regression tests and internal design clarification.

## Out of scope

Recovery-block deletion, fabricated terminal outcomes, prompt replay, native
conversation replacement, live-instance mutation, UI changes, and remote or
other OS process implementations.

## Acceptance

1. A captured live identity is alive, including historical zero and nonzero
   token suffixes; changed birth time, process group, or session is rejected.
2. An exited isolated leader permits restoration only after all live descendants
   exit. Malformed identity and failed inspection remain blocked.
3. Containment terminates the owned descendants while an unrelated isolated
   process remains alive. Existing durable-recovery admission tests pass.

## Verification

Run from the repository root on macOS:

```bash
(cd apps/backend && go test -trimpath -race ./internal/common/processidentity -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(DeliveryRecord|WorkspaceOnlyAllocation|Darwin)' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run 'Test(RestartRecovery|AlreadyLoadedRestart|LegacyNativeRestart|WorkspaceRecoveryBlock)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the normal commit hooks and the local `pr-docs.cjs` `validateCoverage`
preflight against the changed files and referenced requirement/design contents.

## Files likely touched

- `apps/backend/internal/common/processidentity/processidentity_darwin.go`
- `apps/backend/internal/common/processidentity/processidentity_darwin_test.go`
- `docs/specs/platform/system-design/durable-agent-reattachment.md`

## Dependencies

None.

## Risks

A numeric PID is insufficient ownership evidence. Keep birth-time and isolated
session checks, and preserve unknown/error outcomes on ambiguous evidence.
macOS tests must execute on macOS; Linux CI cannot exercise these build-tagged
branches.

## Parallelism

`sequential`

## Inputs

- REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006, criterion .23.
- Durable reattachment design, automatic restart recovery section.
- Existing Linux ownership/termination tests and macOS process implementation.
- Read-only backend diagnostic bundle and exact-session retained journal evidence.

## Results

RED: all three new macOS subprocess tests failed on the original implementation:
zero token rejected, nonzero pointer misclassified, and exited PID returned EIO.
GREEN: all verification commands above passed on macOS arm64. The common
process-identity package passed with the race detector after the final test edit;
the targeted lifecycle and orchestrator recovery suites also passed with race
detection. The catalog validated 377 decisions and 1,538 specifications;
specification lint and `git diff --check` passed. The local `validateCoverage`
preflight returned `covered` with no errors.

A read-only check using the affected runtime's exact saved birth identity changed
from an inspection error to confirmed termination after the correction. Journal
inspection retained its one interrupted-unknown submission and unchanged 993
cursor. No live service state was modified. Temporary diagnostic code was removed.
Commit hook results belong in the PR validation record.
