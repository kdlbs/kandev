# Orchestrator is the single central assistant product

Status: accepted

## Decision

Kandev presents one central product named **Orchestrator**. Workspace
coordination and the capabilities previously called Personal Assistant are
parts of that product, enabled by the single `features.orchestration` flag.

The Orchestrator may have workspace-scoped assignments and an owner-selected
home conversation, but those are views and scopes of one product. Objectives,
attention, memory, capability discovery, native input handling and managed work
belong to the Orchestrator runtime alongside task delegation and the central
workspace task view.

Office remains a separate legacy autonomous-agent fleet surface. Its individual
agent pages and settings are compatibility/admin surfaces, not another central
assistant. Existing Office data and routes remain available when Office is
explicitly enabled; Office is not required by Orchestrator.

## Rollout

`features.orchestration` is the only Orchestrator product gate. The former
`features.personalAssistant` configuration and UI gate are removed from the
feature contract. Internal runtime environment names may remain temporarily as
compatibility plumbing, but they are derived from the Orchestrator gate and do
not represent an independent user choice.

The live Coordinator pilot must be replaced by a qualified candidate carrying
this consolidation before the owner-level capabilities are enabled in
production. The current pilot candidate remains Coordinator-enabled with its
previous Assistant-off setting until that replacement is qualified.

## Consequences

- Users get one Orchestrator destination rather than Coordinator and Assistant
  being presented as competing products.
- Existing task, profile, executor, ownership and permission boundaries remain
  unchanged.
- Office compatibility does not leak into Orchestrator runtime or grant it
  additional authority.
- Tests and rollout receipts must verify the single gate and must not describe a
  separate Personal Assistant toggle.
