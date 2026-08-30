# Derive Provider-Scoped Task MCP Tools in the Backend

- **Status:** Accepted
- **Date:** 2026-08-03
- **Scope:** Backend, agentctl, protocol, GitHub, and GitLab integrations
- **Related spec:** [Provider-Aware Review Automation Runtime](../specs/integrations/requirements/provider-aware-review-automation.md)
- **Related PR:** [#2125](https://github.com/kdlbs/kandev/pull/2125)

## Context

Task-mode MCP currently registers both the GitHub PR automation tools and the
GitLab MR automation tools. That makes discovery independent of the providers
actually attached to a task. It consumes agent context and presents unusable
capabilities, particularly for single-provider and local-only tasks.

Provider identity already belongs to durable backend repository records. Tasks
can contain multiple repositories, including mixed providers, and workspace
sources can be attached while an agent is running. Agentctl has runtime and MCP
state but does not own the task repository model. Inferring providers from Git
remotes inside agentctl would duplicate repository parsing and would be wrong for
non-Git or not-yet-materialized sources.

The related GitLab lifecycle implementation also discards reviewer data returned
by its status read and reconstructs evaluation state through broader provider and
database queries. Choosing a provider boundary should therefore include the
internal data-flow rule that observations are captured once and carried to the
consumer that evaluates them.

## Decision

1. The backend/orchestrator owns provider capability derivation. It computes the
   union of normalized `Repository.Provider` values across every repository
   attached to the task; the primary repository does not override the union.
2. Provider-specific MCP discovery is fail closed. The initial allowlist contains
   `github` and `gitlab`; unknown, blank, local, and unsupported values add no
   review-automation tools.
3. The provider set is transported explicitly through launch, lifecycle, and all
   executor backends into agentctl. Agentctl enforces the supplied set and does
   not inspect filesystem remotes or query backend repository state while
   listing tools.
4. Agentctl supports replacing the provider set at runtime. Its MCP server
   rebuilds the registry while preserving mode and uses the protocol's tool-list-
   changed notification when the effective list changes.
5. Successful live source additions trigger one authoritative recomputation per
   completed batch. A failed refresh does not roll back the source; the old set
   remains a fail-closed subset for addition-only mutations, and the next launch
   or resume repairs runtime state.
6. Tool visibility remains discovery only. Backend handlers stay registered and
   enforce authorization, task ownership, provider compatibility, and credential
   checks for every invocation.
7. GitLab's existing MR status observation may carry reviewer data through the
   internal update event with an explicit validity marker. The observation is not
   persisted on the public `TaskMR` model. Evaluation uses a targeted options and
   checkpoint query plus the workspace configuration's persisted authenticated
   username; strict provider fallback remains for events without an observation.

## Alternatives considered

### Continue exposing every provider tool

Rejected because discovery would keep advertising impossible actions and would
grow with every added provider. Backend validation is necessary defense in depth
but does not make an inaccurate discovery surface useful.

### Infer provider identity inside agentctl

Rejected because agentctl does not own task repository identity, filesystem
remotes are incomplete evidence, and parsing them would duplicate backend rules.

### Query the backend on every MCP tools/list request

Rejected because tool discovery should be local and deterministic after runtime
configuration. A synchronous backend dependency adds latency and another failure
mode to a protocol operation that can be updated through explicit state changes.

### Replace PR and MR tools with generic review-automation tools

Rejected for this change because GitHub and GitLab do not expose the same
automation features. A generic name would either hide provider differences or
introduce a provider-dependent payload contract.

### Persist reviewer snapshots on TaskMR

Rejected because reviewer membership is evaluation input already available from
the poll response, not durable task-link state. Persisting it would add schema and
staleness semantics without helping the normal path.

## Consequences

- The internal launch/config transport gains an explicit provider field across
  several layers, but ownership is unambiguous and future providers extend one
  mapping rather than every agent runtime.
- Mixed-provider tasks work naturally by union; unknown providers fail closed.
- Live additions can update capable MCP clients without restart. Temporary
  refresh failure may leave newly applicable tools hidden until restart, never
  expose tools for an unsupported provider.
- GitLab evaluation can avoid repeated external calls and task-wide lifecycle
  scans while retaining compatibility for legacy/internal events.
- Backend handler checks and tests remain mandatory because discovery is not a
  security boundary.
