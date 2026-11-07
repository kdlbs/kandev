---
id: 01-ordered-tiers
title: Native ordered tier editor and durable selection
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-ORDERED-TIERS-001
acceptance_criteria:
  - AC-AGENTS-ORDERED-TIERS-001.1
  - AC-AGENTS-ORDERED-TIERS-001.2
  - AC-AGENTS-ORDERED-TIERS-001.3
  - AC-AGENTS-ORDERED-TIERS-001.4
system_design:
  - ../../specs/agents/system-design/ordered-profile-tiers.md
---

# Ordered tier slice

Implement the native flag, RulesJSON codec/partial preservation, ordered engine
chain and the existing editor's adjacent join/move/delete controls. Exclude
keep-model schema/behavior, other ranking modes, preview, admission limits and
operating configuration. Preserve their stored metadata without defaulting it.

## Acceptance

1. Saved joins work in the real dynamic runtime while OFF keeps flat behavior.
2. Retry/fallback/restart and current-success boundaries retain the chain rules.
3. Old clients and partial join edits preserve model/windows and unknown data.

## ASCII UI preview

UI-01 from [the plan](plan.md#ascii-ui-preview): tier number and existing row,
up/`=`/down/delete controls, existing enabled and failure-policy editors. The first
join is disabled. The same draft/mutations serve desktop and phone.

## Targeted verification

From `apps/backend`:

```text
go test -p 2 ./internal/runtimeflags ./internal/profiles
go test -p 2 ./internal/agent/runtime/dynamic ./internal/agent/runtime ./internal/agent/settings/controller -run 'Test.*Dynamic|TestAccountBlock|Test.*Unclassified'
```

After frozen pnpm install, use existing frontend feature/editor/normalization
checks, changed-file ESLint and typecheck. Disposable Go overlays/React checks
stay outside normal collection and are removed after readback. Run the existing
PR documentation coverage function against actual changed paths/documents.

## Results

The ordered source increment and its targeted checks are complete. Integration
and publication remain with the integration owner. No feature was enabled and
no operating database/configuration was changed.

- Frozen pnpm install completed. Existing frontend feature/editor/save/
  normalization checks passed: 5 files, 47 tests. Changed-file ESLint with
  `--max-warnings 0` passed. Standard typecheck (including its generators and
  rich-output check) passed with a process-local 8 GiB Node heap. The initial
  direct compiler call lacked the fresh checkout's generated changelog/release
  JSON; running the normal pretypecheck step resolved those missing inputs.
- Existing Go runtimeflags/profiles/dynamic/runtime/controller checks passed.
  Existing orchestrator/backendapp dynamic and unclassified checks passed.
- Disposable Go overlays proved ordered no-revisit across restart/wait, the
  legacy failure path, same-candidate unclassified carry, independent counter
  reset, stale-success fencing, OFF flat traversal and next-tier direction.
  Controller probes proved partial model merges, unknown nested/window metadata,
  legacy omission, originating policies on fragments and disabled join rejection.
- Disposable frontend checks passed 7 tests: join/split/move/delete ownership,
  stable IDs, opaque future mode/model/window round-trip, legacy omission, and
  the real enabled/disabled join control and callback.
- A task-bound Office success regression was reproduced with the old
  counter-only Office exclusion restored through a source overlay: the ordinary
  task passed while Office retained its old chain. Removing that unrelated
  exclusion from the chain-only success helper made both pass. The probe also
  marked the counter reset already complete: a verified success still ended the
  chain and a subsequent failure could choose an untried peer in its new chain.
  The final combined Go probes passed in dynamic/controller/orchestrator.
- Existing i18n checks passed, including all real catalogs and generated
  traditional/pseudo catalogs. Unrelated generated catalog changes were removed.
- Existing PR documentation coverage accepted the actual changed paths and four
  document bodies: `covered`, `ok: true`, `errors: []`. Diff whitespace check passed.

These are source/component checks with disposable fixtures. No real provider,
model execution, installed desktop/phone runtime or operating migration was
tested. Keep-model schema/behavior, other ranking modes, usage inputs and preview
remain original follow-up increments; their saved metadata is preserved rather
than defaulted. Publication and source integration belong to the integration
owner.

The final source lint covered the eight changed Go packages. Its only finding
was `prepareOrderedPlan` complexity 20 against the existing maximum 15. Extracting
the pure remaining-candidate filter preserved behavior; the relevant dynamic
package then passed lint with zero issues and its disposable chain probes passed
again. The other seven package results were reused. No lint threshold, bypass or
suppression was added. The final temporary Go/React probes, overlays and baseline
helper copy were removed after inspection; no permanent tests or runner remain.

Integration added the public agent-profile instructions for the independent
ordered-tier toggle, adjacency controls, durable fallback and preserved advanced
mode limitations. The existing public-docs validator passed all 48 pages.
The integration owner read the actual runtime chain/completion source and
closed the task-bound Office exclusion finding with the existing red/green
result, without changing the separate unclassified-counter policy.
