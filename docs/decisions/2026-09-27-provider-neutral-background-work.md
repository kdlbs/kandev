# ADR-2026-09-27-provider-neutral-background-work: Provider-neutral agent background work

**Status:** accepted
**Date:** 2026-09-27
**Area:** protocol

## Context

Native Codex introduces background-command discovery and child-thread events.
ACP adapters already normalize some subagent and detached-shell observations,
with uneven control support. Future Claude-native support has similar product
needs. A Codex-specific terminal drawer would couple UI, persisted identities,
and controls to one wire protocol and confuse provider-owned processes with
Kandev terminal shells.

## Decision

The agents system owns one normalized background-work contract: workload/run
identity, lifecycle observations, content references, and granular capabilities.
Protocol adapters translate at the boundary. Shared services authorize and route
operations through the session's owning runtime; shared messages, transport,
stores, and UI consume provider-neutral fields. Future adapters implement this
contract rather than introducing parallel product models.

Codex app-server is the first native implementation. ACP initially projects its
existing proven observations and defaults unsupported controls off. Claude
native is a future adapter, not implementation scope for this package. A
provider-neutral conformance fixture proves portability without pretending a
second native implementation exists.

A persisted inspection projection is separate from the existing ephemeral
background-admission tracker and historical subagent-invocation rows. Only
adapter-attested evidence can affect existing accounting. A UI state, restored
row, or control acknowledgement cannot complete a root turn or authorize prompt
admission. Native process/thread identifiers stay in backend bindings; they are
not host PIDs, Kandev task IDs, or authorization tokens.

Capabilities are per connection and workload, fail closed, and are checked again
at dispatch. Generic input support does not imply that Codex thread-owned
background commands accept standalone command/exec input operations.

## Consequences

One UI can support protocols with different capabilities and incomplete data.
Maintaining identity, recovery, and negative capability tests costs more than a
Codex-only view, but prevents per-provider storage and control forks. Historical
records remain useful without resurrecting live process ownership. The shared
feature receives its own rollout gate; the existing native-Codex and Claude
prompt-handoff gates retain their meanings.

## Alternatives Considered

- Codex-specific UI/models: initially smaller, but subsequent protocols require
  duplicated state, history, security checks, and frontend branches.
- Least-common-denominator ACP-only surface: hides native capabilities that can
  be safely normalized and exercised independently.
- Treat every workload as a Kandev terminal or task: changes ownership, admission,
  billing, and permissions; provider IDs do not establish host-process authority.
- One new universal lifecycle tracker: unnecessarily replaces established
  workflow/admission and historical-invocation semantics during UI delivery.

## Related designs

- [Background work](../specs/agents/system-design/background-work.md)
- [Native Codex normalization](2026-09-24-native-codex-normalization.md)
- [Background liveness](../specs/platform/system-design/background-work-liveness.md)
