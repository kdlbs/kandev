---
status: current
system: agents
requirements:
  - REQ-AGENTS-CODEX-SUBAGENT-WIRE-001
---

# Codex Subagent Wire Compatibility System Design

## Purpose and boundaries

Restore the existing Codex dialect contract after the upstream 2.x renderer
removed legacy metadata. This is a wire compatibility repair within ADR-0044;
shared transport, persistence, and presentation contracts remain unchanged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-AGENTS-CODEX-SUBAGENT-WIRE-001 | [Wire interpretation](#wire-interpretation) |

## Wire interpretation

`dialect_codex.go` translates structured input into the same private metadata
view used by the legacy parser, without mutating the received frame. The presence
of `meta.codex` prevents fallback, including malformed and control metadata.

Activity input requires a recognized `activityKind`, a nonblank `agentThreadId`,
and an `agentPath` with a usable name. `interacted` is not a creation or terminal
signal. Collaboration requires exact title `spawnAgent`, a nonblank
`senderThreadId`, a typed `receiverThreadIds` list with valid string entries, and
an `agentsStates` map. An empty receiver list is valid while spawning.

Normalization and adapter correlation use this same interpretation. Update
enrichment preserves the reported title so a collaboration completion can add
the child identity and model. Existing session/tool/child correlation, terminal
tombstones, and emitted IDs continue to isolate siblings and suppress unmatched
terminal activity. No generic tool is promoted based on title alone.

Capacity evidence uses the same structured signal and preserves its conservative
background fence. Presentation-only Codex payloads do not acquire background-work
attestation or change prompt lifecycle authority.

## Persistence, failure, and validation

No schema, API, permissions, logging, or migration changes. Existing consumers
persist the restored normalized payload. Unsupported input remains generic;
existing malformed/ambiguous correlation behavior remains conservative.

Regression tests use sanitized JSON produced by the installed 2.2.2 reporters
and standard-client renderer. They cover creation, title-less activity updates,
spawn-result enrichment, terminal correlation, sibling isolation, nested parent
identity, legacy precedence, negative recognition, and capacity evidence.

## Related decisions

- [ACP compatibility dialects](../../../decisions/0044-acp-agent-compatibility-dialects.md)
