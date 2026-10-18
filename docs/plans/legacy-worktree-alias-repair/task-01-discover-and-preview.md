---
id: "01-discover-and-preview"
title: "Discover and preview duplicate claims"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-004
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.2
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.2
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.4
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.6
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 01: Discover and preview duplicate claims

## Summary

Add a bounded read-only discovery mode and a version-2 preview for exact duplicate
ownership selections. Preserve version-1 plans and reject incomplete or ambiguous
selections before application becomes available.

## In scope

- Add `--discover`, explicit installation paths, cursor, and limits to the existing
  source maintenance command. Database discovery performs no filesystem scan.
- Add typed version-2 selections and complete connected-claim observations, including
  selected canonical slots, aliases, shared environments, sessions, snapshots,
  workspace corrections, and negative dependency queries.
- Reuse Git and root evidence for selected previews. Test identical-looking foreign
  roots, two genuine checkouts, broken pointers, unsupported stores/executors,
  historical rows, mixed failed/live slots, caps, and changed observations.
- Add CLI tests that reject mixed modes, unknown fields, incompatible plan versions,
  and version-2 apply until the guarded implementation is available.

## Out of scope

SQL mutation, cleanup-job replacement, runtime-store changes, and live repair.

## Acceptance

Criteria 004.4 and 004.6 are shared with Task 02. This work order covers preview
refusal and blocker reporting. Task 02 covers apply-time refusal and proves that
the refused operation changes no data.

1. `TestAliasDiscovery` covers bounded complete groups, cursor continuation, oversized
   refusal, and byte-for-byte database/checkout preservation.
2. `TestAliasPreview` accepts both incident shapes only with complete canonical proof
   and reports every blocker without hiding an unsafe member.
3. Version-1 decoding and preview remain compatible, and version-2 mutation cannot
   bypass the unimplemented application gate in this work order.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/inventoryrepair ./cmd/worktree-inventory-repair -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/inventoryrepair/alias_discovery.go apps/backend/internal/task/inventoryrepair/alias_preview.go apps/backend/internal/task/inventoryrepair/alias_discovery_test.go apps/backend/internal/task/inventoryrepair/alias_preview_test.go apps/backend/internal/task/inventoryrepair/plan.go apps/backend/cmd/worktree-inventory-repair/main.go apps/backend/cmd/worktree-inventory-repair/main_test.go
git diff --check
```

Adjust the named new helper paths only if the implementation records its final
equivalents before running this block. Cover every changed file in scoped lint.

## Files likely touched

- `apps/backend/internal/task/inventoryrepair/{plan,read,inspect}.go`
- New `alias_discovery.go`, `alias_preview.go`, and corresponding tests in that package.
- `apps/backend/cmd/worktree-inventory-repair/{main.go,main_test.go}`

## Dependencies

None. Read the version-1 utility and completed companion plan first.

## Risks

Do not truncate one identity's claims into an apparently complete repair.
Discovery is not a trusted offline preview or authority to select a winner.

## Parallelism

`sequential`

## Inputs

- Linked requirement 004 and design sections on discovery and canonical proof.
- Existing `inspect_test.go`, `inspect_extra_test.go`, and native-path tests.

## Results

Completed 2026-10-16.

- RED: `TestAliasPreviewCanonicalProof` rejected the unknown `aliases` field.
  `TestAliasDiscoveryCommand` rejected `--discover`.
- Added bounded inventory discovery, version-2 decoding, canonical Git/root proof,
  shared-group/session/snapshot validation, dependency observations, and refusal tests.
- The oversized-input regression exposed trailing data beyond the decoder limit.
  The decoder now checks the complete bounded input.
- Full task race command passed: inventoryrepair 246.713s, command 1.182s.
- After the final refinements, `go test -trimpath -race
  ./internal/task/inventoryrepair ./cmd/worktree-inventory-repair
  -run 'TestAlias|TestDecode|TestCommand' -count=1` passed (41.946s, 1.093s).
- Scoped lint passed with 0 issues. The final package lint invocation was
  `bash scripts/lint-go-changed apps/backend/internal/task/inventoryrepair/alias_plan.go
  apps/backend/cmd/worktree-inventory-repair/main.go`. This helper checks both complete
  affected packages, including all new helpers and tests.
- `git diff --check` passed.

Additional implementation files are `alias_plan.go`, `alias_shared.go`,
`alias_dependencies.go`, and `inspect.go`. Version-2 command mutation remains
blocked until Task 02.

PR review follow-up (2026-10-18): discovery now measures the actual two-space
JSON encoding, trailing newline, and continuation cursor against the 1 MiB cap.
A byte-oversized group returns an oversized placeholder so later groups remain
reachable. Both regressions failed before the fix. `go test -race
./internal/task/inventoryrepair -run TestAliasDiscovery -count=1` passed.
