# Fusion composition qualification

**Status:** gate unresolved. No verified Lead/Sidekick binding source was established, so no composition adapter or fixture was implemented.
**Date:** 2026-11-03
**Work order:** [Task 04](task-04-fusion-qualification.md)

## Requirement

[AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3](../../specs/agents/requirements/dynamic-provider-options.md) requires Devin Fusion Lead/Sidekick controls to come from a verified relationship source or a narrowly qualified compatibility mapping. Ordinary grouped selection alone does not satisfy the criterion.

## Sources checked

1. **Official Fusion documentation** (`https://docs.devin.ai/cli/fusion`, fetched 2026-11-03). It documents the user-facing controls (Lead, Effort, Sidekick, Fast Mode) and states that a family of lead + sidekick pairings exists, with a default pairing of "Claude Opus 5.5 Medium" plus "SWE-2 Medium". It publishes **no machine-readable model catalogue, model IDs, or pairing schema**. The display labels cannot be mapped to exact advertised ACP values without guessing.
2. **ACP session config options specification** (referenced by the design). It defines native groups of exact choices but no composed-model binding type, so the wire protocol does not supply the relationship either.
3. **Installed repository state.** Grouped conversion is present (`internal/agentctl/acpcompat/catalog.go`), and the shared catalog shape supports optional `compositions`, but nothing populates them for Devin.
4. **Local provider availability.** `devin` is not installed in this environment and no `~/.local/share/devin/credentials.toml` exists, so no authenticated probe could capture grouped frames or the advertised model IDs.

## Missing evidence

- An authenticated Devin CLI handshake that advertises the grouped model catalog **and** the exact advertised model values that correspond to Lead/Sidekick pairings.
- A documented or emitted relationship contract (vendor metadata, documented catalogue endpoint, or a stable decoder grammar with observed versions) that maps a complete dimension tuple to exactly one advertised model.
- Sanitized real frames proving that Thinking/Speed/Effort remain independent provider config options for the selected pairing.

## Consequence

- No speculative decoder, model-name splitting, or synthetic composition bindings were added.
- Grouped catalogs, search, and the shared picker work for any provider that supplies native groups.
- [Issue #5026](https://github.com/kdlbs/kandev/issues/5026) remains **incomplete**: its verified Fusion composition acceptance is not met.
- The Task 04 gate stays open. A grouped-only fallback is compatibility evidence, not Fusion acceptance.

## Reproduction procedure when a qualified host is available

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

## Verification run in this session

- `devin` binary: absent (`which devin` returned nothing).
- Devin credential file: absent.
- Official documentation fetch: succeeded; no machine-readable pairing contract.
- No live provider process, prompt, or account was used.
