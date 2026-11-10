---
id: "04-fusion-qualification"
title: "Qualify Devin composition bindings"
status: pending
wave: 4
depends_on:
  - "03-catalog-contract"
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.1
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.2
  - AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
---

# Task 04: Qualify Devin composition bindings

## Summary

Establish a verified source for Fusion Lead/Sidekick relationships before implementing their normalization.
Then add a narrowly qualified backend adapter that resolves only exact current catalog entries.

## In scope

- Inspect official metadata/documentation and no-prompt ACP or documented model-catalogue responses.
- Capture sanitized fixtures with provenance and identify supported relationship shapes.
- Implement backend normalization only after its qualification record defines the contract.
- Prove unknown, ambiguous, missing, changed, and unsupported entries retain ordinary selection.

## Out of scope

- Guessing relationships from the issue's illustrative model ID or parsing names in React.
- Commercial inference, pricing estimation, broad string-splitting heuristics, or account enrollment.

## Acceptance

1. `fusion-qualification.md` identifies an actual relationship source, tested mapping, provenance, and unknown/ambiguous behavior.
2. Every normalized complete combination resolves to exactly one currently advertised value, with Thinking/Speed left as real provider options.
3. Unqualified entries remain ordinary models. No completion result claims Fusion support without the verified binding evidence.

## Qualification gate

The current issue proves groups, pairings, and dynamic options, but not a durable decoder grammar.
Official Fusion documentation describes controls without a machine-readable relationship contract.
First prefer explicit vendor metadata or a documented catalogue source.
If a narrow decoder is necessary, record observed versions/shapes and validate it against sanitized real frames.
Do not treat model-name tokens or effort suffixes as authoritative option values.

Use `/acp-debug` and existing credentials only where available and authorized.
No prompt is required. CI uses checked-in sanitized fixtures and does not require a paid account.
The initial probe command on a configured local host is:

```bash
test -x apps/backend/bin/acpdbg || make -C apps/backend build-acpdbg
apps/backend/bin/acpdbg probe --client-capabilities '{"_meta":{"cognition.ai/groupedSessionConfigOptions":true}}' --timeout 40s --out /tmp/kandev-5026-qualification devin-acp
```

Raw frames stay outside the repository. Remove owned temporary data after extracting sanitized evidence.
Do not change the user's live database, profiles, credentials, or session configuration.
If the source cannot establish bindings, record the exact missing evidence and leave this work order incomplete.
Do not implement speculative decoding or mark #5026 complete.

## TDD after qualification

Add `TestDevinCompositionBindings` using the qualified real-shape fixture.
The RED assertion is absent dimension bindings for that verified source.
Add cases for new unknown entries, ambiguous labels, unsupported shapes, missing pairs, and catalogs changing between requests.
Assert exact raw values and no synthetic model IDs.
Add `TestDevinCompositionKeepsProviderOptions` with an effort-like model suffix and independently returned Thinking/Speed values.

## Verification

Run from the repository root after the gate is satisfied.

```bash
(cd apps/backend && go test -trimpath ./internal/common/modelcatalog ./internal/agentctl/acpcompat -run 'Test.*(Composition|Fusion|Devin|Catalog)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/acpcompat/devin_catalog.go apps/backend/internal/agentctl/acpcompat/devin_catalog_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add every further changed Go path to the lint command.
Record provider qualification separately from fixture test results.

## Files likely touched

- New `docs/plans/first-draft-profile-discovery/fusion-qualification.md`
- New `apps/backend/internal/agentctl/acpcompat/devin_catalog.go` and `devin_catalog_test.go`
- Sanitized `apps/backend/internal/agentctl/acpcompat/testdata/devin-*.json`
- Shared catalog validation tests if qualification reveals another bounded shape

## Dependencies

Task 03. This is a hard gate for real Devin composition, not for ordinary flat compatibility.

## Risks

- A valid-looking decoder can bind the wrong exact pairing.
- A static list can become stale as account catalogs change.
- A grouped-only fallback does not satisfy the user's Fusion configuration outcome.

## Parallelism

`sequential`

## Inputs

- [Composition qualification design](../../specs/agents/system-design/grouped-model-catalog.md#composition-qualification)
- [Issue #5026](https://github.com/kdlbs/kandev/issues/5026)
- [Devin Fusion](https://docs.devin.ai/cli/fusion) and [ACP config options](https://agentclientprotocol.com/protocol/v1/session-config-options)

## Results

Gate remains open. The qualification record is [fusion-qualification.md](fusion-qualification.md).
The following results describe the original 2026-11-03 investigation.

- Official Fusion documentation was fetched: it describes Lead, Effort, Sidekick, and Fast Mode but publishes no machine-readable pairing schema or exact model IDs.
- ACP defines grouped choices only, and the repository has no other relationship source.
- This environment has no `devin` binary and no Devin credential file, so no authenticated grouped probe could run.
- No decoder, synthetic binding, or model-name parsing was added. Grouped selection works; verified Fusion composition and issue #5026's Fusion criterion remain incomplete.

## Follow-up execution for issue #5053

The issue now identifies native JSON model listing and reports an exact ID/label cross-check for the current ACP choices.
Observed fixture capture and the production adapter remain pending.
Execute the [binding work order](../model-catalog-selector-repair/task-02-fusion-bindings.md), then the [contextual acquisition work order](../model-catalog-selector-repair/task-03-contextual-acquisition.md).
They refine this work order's remaining scope, commands, and negative cases.
Keep this work order pending until both pass. Record real capture and integration results here without replacing the historical results.
