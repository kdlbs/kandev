---
id: "02-fusion-bindings"
title: "Qualify and normalize native Fusion bindings"
status: in_progress
wave: 2
depends_on:
  - "01-singleton-presentation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.4
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 02: Qualify and normalize native Fusion bindings

## Summary

Capture the observed native JSON shape and implement a pure, conservative join to the current ACP catalog.
Publish Lead/Sidekick bindings only for uniquely explained values and tuples.
This work order satisfies the evidence and pure-normalization portion of the original Task 04.

## In scope

- Update the existing `fusion-qualification.md` with sanitized captured evidence, provenance, source schema, supported command flags, and family identity.
- Capture `models list --format json` and grouped ACP choices without a prompt or inference.
- Use the same resolved executable, launch wrapper, environment, authentication, configuration, proxy, and executor as the ACP process.
- Keep only family identities/labels, variant identities/labels, relevant ACP leaves, and independent option snapshots in fixtures.
- Add a bounded typed parser and pure normalizer under `acpcompat`.
- Match both native expressions from the design, then intersect on the current ACP leaf value and label.
- Derive Lead from native family identity and Sidekick from exact variant identity.
- Reject duplicate native identities, multiple candidate pairs, and dimension-tuple collisions without a first-match choice.
- Preserve all unknown, unavailable, changed, fast, ambiguous, and native-only members as ordinary or absent according to ACP availability.
- Validate an enriched copy without mutating the original catalog or its flat compatibility projection.

## Out of scope

- Listing-process integration, cache changes, browser changes, or production credentials in CI.
- Pricing/context metadata, a static model list, a universal string grammar, or effort/speed inference.
- Claiming live Fusion delivery from a pure unit test.

## Acceptance

1. The qualification record and observed fixture establish the supported schema, family identities, exact join, launch context, and independent options.
2. `TestDevinCompositionBindings` fails before the normalizer and passes afterward using the captured shape and current ACP intersection.
3. All ambiguous members remain unbound, every accepted tuple resolves uniquely, and unknown members retain exact ordinary selection.

## Qualification gate

Issue #5053 identifies a credible native source and reports exact current-catalog coverage.
This workspace has no Devin executable, so the planning turn did not reproduce that capture.
Before enabling a production adapter, obtain a sanitized observed fixture from a configured, authorized executor.
Do not relabel synthetic rows or the parent issue's illustrative ID as captured provider evidence.

The native invocation is `<resolved Devin executable> models list --format json` through the matching launch context.
The grouped ACP capture uses the same context and the qualified `cognition.ai/groupedSessionConfigOptions` client capability.
Record CLI version, capture date, field names, group identity, exact accepted leaves, and rejected shapes.
Keep raw output outside the repository. Remove owned temporary data after extraction.
Do not copy credentials, reset the configuration directory, alter live profiles, or send a prompt.

If a capture is unavailable, leave this work order pending and record that limitation.
Task 01 can remain a completed independent correction. Synthetic tests alone do not satisfy this gate.

## Regression cases

- A verified native pair with an ACP value and label that both match exactly.
- A native-only pair, a withdrawn ACP choice, a label mismatch, and a changed source field.
- Duplicate native variant identities and more than one possible pair for one Fusion value.
- Two advertised values with the same `(lead family, sidekick variant)` tuple. Neither receives a binding.
- A mixed group containing accepted, rejected, and ordinary unknown members.
- A source family label that differs from its variant label. Lead uses the family label without suffix stripping.
- An effort-like model suffix alongside independently returned Thinking/Speed evidence.
- A valid single-binding composition and an empty surviving binding set.
- Malformed required fields and catalog bounds. Invalid enrichment retains the original catalog.

## Verification

Run from the repository root after the fixture gate.
Record capture qualification separately from deterministic fixture test results.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/common/modelcatalog ./internal/agentctl/acpcompat -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/acpcompat/devin_catalog.go apps/backend/internal/agentctl/acpcompat/devin_catalog_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add every further changed Go path to the lint command.
Keep reported capture counts as evidence, without assertions that every account must have those counts.

## Files likely touched

- New `apps/backend/internal/agentctl/acpcompat/devin_catalog.go`
- New `apps/backend/internal/agentctl/acpcompat/devin_catalog_test.go`
- New sanitized `apps/backend/internal/agentctl/acpcompat/testdata/devin-native-models.json`
- New sanitized `apps/backend/internal/agentctl/acpcompat/testdata/devin-acp-models.json`
- `docs/plans/first-draft-profile-discovery/fusion-qualification.md`

## Dependencies

Task 01 precedes this work order in the default execution sequence.
Task 03 requires this work order's actual qualification and pure normalizer.

## Risks

- A family-level Lead can collapse distinct native effort variants into an ambiguous tuple.
- A matcher can overstate coverage if it checks only the model ID or only the display label.
- The broader native catalog includes shapes outside the reported narrow rule.

## Parallelism

`sequential`

## Inputs

- [Composition qualification design](../../specs/agents/system-design/grouped-model-catalog.md#composition-qualification)
- [Current qualification record](../first-draft-profile-discovery/fusion-qualification.md)
- [Original Task 04](../first-draft-profile-discovery/task-04-fusion-qualification.md)
- Issue #5053's native-source evidence and existing `modelcatalog.Validate` tests

## Results

Pure normalization implemented 2026-11-04. The captured-fixture gate is still open.

- `apps/backend/internal/agentctl/acpcompat/devin_catalog.go` adds the bounded `families[]`/`variants[]` decoder, the exact identifier and label join, first-occurrence dimension ordering from the ACP group, ambiguity rejection, tuple-collision fallback, and non-mutating enrichment that validates a copy before publication.
  Unqualified, malformed, or colliding candidates produce no composition for that group, and a group that already carries a composition is skipped while other qualifying groups can still receive one.
  The original catalog and its flat projection are returned only when no composition is added or the final validated copy is rejected.
- `apps/backend/internal/agentctl/acpcompat/devin_catalog_test.go` covers parsing bounds and duplicates, exact bindings, current-catalog intersection, mixed unknown leaves, tuple collisions, ambiguous pairs, unexplained labels, repeated leaves, and original-catalog preservation.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/acpcompat ./internal/common/modelcatalog -count=1)`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/agentctl/acpcompat/devin_catalog.go apps/backend/internal/agentctl/acpcompat/devin_catalog_test.go`: 0 issues.

Qualification gate status:

- This workspace has no `devin` executable, authorized account, or sanitized captured fixture, so acceptance 1 is not met and no `testdata/devin-native-models.json` or `testdata/devin-acp-models.json` was committed.
- The tests use synthetic fixtures shaped by the design contract. They prove deterministic normalization only; they are not captured provider evidence and must not be described as such.
- Acceptance 2's RED test exists, but its captured-shape qualification and acceptance 3's real-source evidence cannot pass until a configured, authorized executor records the fixture.
- Task 03 stays blocked because it requires this work order's actual qualification. Keep this work order `in_progress`.
