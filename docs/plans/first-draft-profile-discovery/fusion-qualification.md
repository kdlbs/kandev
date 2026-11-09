# Fusion composition qualification

**Status:** native source identified in issue #5053. Pure normalization is implemented and deterministically tested. Sanitized observed fixtures, a production adapter, and context-bound acquisition remain pending.
**Date:** 2026-11-03
**Updated:** 2026-11-04
**Work order:** [Task 04](task-04-fusion-qualification.md)

## Current assessment

[Issue #5053](https://github.com/kdlbs/kandev/issues/5053) reports an actual `devin models list --format json` capture.
The native JSON contains families and exact variant IDs/labels.
CLI version: `3000.11.234 (92c83011d113)`. The ACP investigation used source `067632acaa29e657c45162c2b8eb375958497300`.

The diagnostic accepted a pair only when both exact relationships held:

```text
fusion.model_uid == "fusion-" + lead.model_uid + "-sidekick-" + sidekick.model_uid
fusion.label == "Fusion (" + lead.label + " + " + sidekick.label + ")"
```

The issue reports 61 unique matches for the current ACP Fusion group.
Lead family identity plus exact Sidekick variant identity produced 61 unique tuples, with seven Lead families and nine Sidekick variants.
The same rule matched only 395 of 590 native Fusion variants. Unsupported members, including some fast variants, require ordinary fallback.
These counts describe one capture and do not define a universal grammar.

The reported native output is approximately 437 KB. The host CLI runner's combined 64 KiB buffer cannot retain it.
The capture also needed the existing configuration environment. An empty configuration directory failed to connect.
Production acquisition must preserve the ACP launch's executable, wrapper, authentication, configuration, proxy, and executor context.
The native catalog must never add availability beyond the current ACP choices.

The current planning workspace has no Devin executable.
The preceding observations are attributed issue evidence, not a newly reproduced native capture or a checked-in fixture.
The local investigation reproduced the singleton presentation defect and confirmed that catalog publication supplies no compositions.

[The repair package](../model-catalog-selector-repair/plan.md) owns the remaining work.
[Task 02](../model-catalog-selector-repair/task-02-fusion-bindings.md) captures sanitized observed fixtures and implements the pure join.
[Task 03](../model-catalog-selector-repair/task-03-contextual-acquisition.md) supplies bounded acquisition and every live/probe publication path.
Keep this qualification gate open until both work orders pass.
The record below preserves the original investigation and its actual results.

## Implementation status

2026-11-04: the pure join landed in `apps/backend/internal/agentctl/acpcompat/devin_catalog.go`.
It decodes the bounded `families[]`/`variants[]` shape, applies the exact identifier and label expressions above, intersects with the current ACP group, rejects ambiguous pairs, and leaves colliding tuples ordinary.
Enrichment never mutates or extends the original catalog, and an invalid result keeps the ordinary selection path.

`TestDevinCompositionBindings` and its sibling negative tests exist, but they run on synthetic fixtures shaped by this design contract.
They prove deterministic normalization, not captured provider evidence.
No `testdata/devin-native-models.json`, `devin-acp-models.json`, production adapter, or context-bound acquisition was added, because this workspace still has no configured Devin executable.
The capture gate, Task 03, and the browser/production acceptance therefore remain open.

## Requirement

[AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3](../../specs/agents/requirements/dynamic-provider-options.md) requires Devin Fusion Lead/Sidekick controls to come from a verified relationship source or a narrowly qualified compatibility mapping. Ordinary grouped selection alone does not satisfy the criterion.

## Sources checked at the original handoff

1. **Official Fusion documentation** (`https://docs.devin.ai/cli/fusion`, fetched 2026-11-03). It documents the user-facing controls (Lead, Effort, Sidekick, Fast Mode) and states that a family of lead + sidekick pairings exists, with a default pairing of "Claude Opus 5.5 Medium" plus "SWE-2 Medium". It publishes **no machine-readable model catalogue, model IDs, or pairing schema**. The display labels cannot be mapped to exact advertised ACP values without guessing.
2. **ACP session config options specification** (referenced by the design). It defines native groups of exact choices but no composed-model binding type, so the wire protocol does not supply the relationship either.
3. **Installed repository state.** Grouped conversion is present (`internal/agentctl/acpcompat/catalog.go`), and the shared catalog shape supports optional `compositions`, but nothing populates them for Devin.
4. **Local provider availability.** `devin` is not installed in this environment and no `~/.local/share/devin/credentials.toml` exists, so no authenticated probe could capture grouped frames or the advertised model IDs.

## Missing evidence at the original handoff

- An authenticated Devin CLI handshake that advertises the grouped model catalog **and** the exact advertised model values that correspond to Lead/Sidekick pairings.
- A documented or emitted relationship contract (vendor metadata, documented catalogue endpoint, or a stable decoder grammar with observed versions) that maps a complete dimension tuple to exactly one advertised model.
- Sanitized real frames proving that Thinking/Speed/Effort remain independent provider config options for the selected pairing.

## Original consequence

- No speculative decoder, model-name splitting, or synthetic composition bindings were added.
- Grouped catalogs, search, and the shared picker work for any provider that supplies native groups.
- [Issue #5026](https://github.com/kdlbs/kandev/issues/5026) remains **incomplete**: its verified Fusion composition acceptance is not met.
- The Task 04 gate stays open. A grouped-only fallback is compatibility evidence, not Fusion acceptance.

## Original ACP capture procedure

```bash
test -x apps/backend/bin/acpdbg || make -C apps/backend build-acpdbg
apps/backend/bin/acpdbg probe \
  --client-capabilities '{"_meta":{"cognition.ai/groupedSessionConfigOptions":true}}' \
  --timeout 40s --out /tmp/kandev-5026-qualification devin-acp
```

1. Keep raw frames outside the repository.
2. Extract sanitized fixtures with provenance: CLI version, date, advertised group IDs/labels, exact choice values, and the option snapshot after selecting a pairing.
3. Record the observed binding rule in this file before enabling any composition decode.
4. Add `TestDevinCompositionBindings` with the sanitized fixture, including unknown/ambiguous/changed catalog cases and an effort-like model suffix that must not determine Thinking.
5. Only then mark this work order complete and #5026's Fusion criterion satisfied.

## Verification at the original handoff

- `devin` binary: absent (`which devin` returned nothing).
- Devin credential file: absent.
- Official documentation fetch: succeeded; no machine-readable pairing contract.
- No live provider process, prompt, or account was used.
