---
id: "02-all-tool-preparation"
title: "Prepare every tool report before acceptance"
status: complete
wave: 2
depends_on:
  - 01-limits-and-replay
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-001
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-002
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-003
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-004
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.1
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.2
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.3
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.4
  - AC-PLATFORM-BOUNDED-TOOL-DATA-001.5
  - AC-PLATFORM-BOUNDED-TOOL-DATA-002.1
  - AC-PLATFORM-BOUNDED-TOOL-DATA-002.2
  - AC-PLATFORM-BOUNDED-TOOL-DATA-002.3
  - AC-PLATFORM-BOUNDED-TOOL-DATA-002.4
  - AC-PLATFORM-BOUNDED-TOOL-DATA-003.1
  - AC-PLATFORM-BOUNDED-TOOL-DATA-003.3
  - AC-PLATFORM-BOUNDED-TOOL-DATA-004.2
system_design:
  - ../../specs/platform/system-design/bounded-tool-delivery.md
---

# Task 02: Prepare every tool report before acceptance

## Summary

Generalize bounded preparation across every tool category and supported content carrier.
Retain ordinary incident-size search output, reduce larger display data, and preserve the actual terminal and source result.

## In scope

- Replace generic-only admission with one shared operation at ACP, native Codex, and direct process emission boundaries.
- Apply explicit typed policies and an aggregate 1536 KiB display budget, including escaping and duplicate carriers.
- Preserve the existing stricter generic value policy and the supported incremental shell representation.
- Reserve envelope space, then apply the complete encoded-event guard before journal sequencing.
- Reuse existing typed truncation flags, generic preview values, and rich-content text markers.
- Preserve current renderer behavior without new message metadata or projection plumbing.
- Preserve immutable source snapshots, idempotent preparation, old-reader normalization, and replay hashes.
- Verify accepted replay bytes and current normalized hash compatibility.

## Out of scope

New notices, preview controls, renderer changes, and localization work are excluded.
This work does not change permission decisions, background provenance, shell epochs, actual file edits, or native tool responses.
No full-output store, new event field, SQL column, or hash version is authorized.

## Acceptance

1. The real forwarding path retains a synthetic 1,048,607-byte search result intact, subsequent text, and the actual terminal in order.
   Larger category/carrier fixtures retain a marked bounded copy without cancellation or poisoned delivery health.
2. Preparation preserves category, identity, status, trusted provenance, source immutability, explicit empties, and repeat-preparation bytes.
   Allocation and traversal remain bounded for huge, deep, escaped, multibyte, and duplicate inputs.
3. Existing omission representations survive replay without changing historical hashes or interface contracts.
   A truncated file or diff cannot appear as complete executable content. Shell patch and canonical tail semantics remain unchanged.

## TDD regression

Extend the real forwarder pattern in `bounded_tool_delivery_test.go`.
Create `TestAllToolDeliverySearchContinuesToTerminal` and `TestAllToolDeliveryCategoriesAndDuplicateCarriers`.
Use a reduced fixture event limit to prove the typed-carrier bypass independently of Task 01's larger default.
Include complete, failed, and cancelled tool statuses followed by the actual prompt terminal.

Create `TestAllToolDeliveryBoundedWorkAndSnapshots`, `TestAllToolDeliveryReplayHashCompatibility`, and `TestAllToolDeliveryExistingRepresentation`.
Cross search, read, edit, shell, HTTP, subagent, plan/todo, generic, and unknown-tool cases.
Include a fitting control/provenance object beside oversized display text.
A forged marker must not bypass accounting or create permission/background authority.

## Verification

Run from the repository root:

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race -p 1 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agentctl/server/process ./internal/task/repository/sqlite -run 'TestAllToolDelivery|Test.*BoundedTool|Test.*NormalizedPayloadSnapshot|Test.*IncrementalShell|Test.*ShellOutputPatch' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

No SQL query or projection change is planned. Existing persistence and hash regressions remain required.
Run changed-file Go lint over this work order's complete changed-file inventory.

Run this exact changed-file Go lint block from the repository root:

```bash
python3 - <<'LINT'
import pathlib, subprocess
tracked = subprocess.check_output(['git', 'diff', '--name-only', '--diff-filter=ACMR', 'HEAD', '--', 'apps/backend'], text=True)
untracked = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '--', 'apps/backend'], text=True)
files = sorted({p for p in (tracked + untracked).splitlines() if p.endswith('.go') and pathlib.Path(p).is_file()})
if files:
    subprocess.run(['bash', 'scripts/lint-go-changed', *files], check=True)
LINT
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/tool_display_bounds.go`, `tool_payload.go`, and their tests.
- Proposed focused helpers beside `tool_display_bounds.go`, split by typed policy when necessary.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go` and `bounded_tool_delivery_test.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/codexappserver/adapter.go` and related tests.
- `apps/backend/internal/agentctl/server/process/manager.go`, `incremental_shell_delivery.go`, and `bounded_tool_delivery_test.go`.
- Existing replay/hash fixture tests under `apps/backend/internal/task/repository/sqlite/`, with no new projection contract.

## Dependencies

Task 01 supplies authoritative limits and distinct size errors.
Task 03 verifies the current recovery interface end to end. This work order adds no frontend work.

## Risks

- Duplicate carriers can exceed the aggregate budget even when each field fits.
- Arbitrary clipping can turn a diff prefix into an apparently valid patch.
- New marker plumbing can add unnecessary projection and renderer complexity. Reuse current fields.
- A new unknown event field can break normalized hash compatibility.
- Shared-object mutation can change later snapshots or cumulative output.

## Parallelism

`sequential`

## Inputs

- [Typed policy and omission design](../../specs/platform/system-design/bounded-tool-delivery.md).
- Existing generic forwarder, ACP emission, native emission, shell patch, and normalized hash regression tests.
- The synthetic incident fixture in the package evidence.

## Results

Generalized generic-only preparation into `streams.PrepareToolEvent`, applied at ACP normal/locked emission, native Codex emission, and the process write boundary. Typed categories and duplicate carriers now share one aggregate display budget and reuse existing truncation representations.

Validation passed:

- `(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race -p 1 ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/codexappserver ./internal/agentctl/server/process ./internal/task/repository/sqlite -run 'TestAllToolDelivery|Test.*BoundedTool|Test.*NormalizedPayloadSnapshot|Test.*IncrementalShell|Test.*ShellOutputPatch' -count=1)`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

The real forwarder retained the synthetic 1,048,607-byte search result intact at the default limit and bounded the same fixture under a reduced limit without stopping the terminal. Category, duplicate-carrier, immutable-snapshot, replay-hash, and existing-representation regressions passed. Oversized diff, binary, todo, and wide-content fixtures now prove bounded preparation work and allocation before serialization.

Changed-file Go lint reported 0 issues.

`TestAllToolDeliveryBoundedSerializationCost` additionally proves that oversized diffs, binaries, todos, and wide content slices avoid full serialization and oversized slice reservations.
