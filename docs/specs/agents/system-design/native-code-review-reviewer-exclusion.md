---
status: draft
system: agents
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-002
---

# Native code review reviewer exclusion

## Purpose

A workflow-triggered native review uses a candidate distinct from the execution
that produced the change. The writer remains the decision owner. Manual reviews
without an origin requirement retain their existing selection behavior.

## Selection and ownership

The workflow callback captures the committed transition's writer session, or the
primary session for an attributable manual transition. The actual session
snapshot supplies the logical and concrete profile and, when available, the
agent/model pair. An unreadable required origin fails the review without blocking
the step or publishing findings.

The resolver excludes the writer's concrete profile and matching agent/model,
preserving the configured candidate order and the existing utility/default
precedence. Native dynamic profiles use a read-only preview before the durable
review run is created. After creation, the run owns actual route selection and
attribution. The exclusion is re-applied at the run boundary and on the detached
worker, including fallback, so a changed or unavailable candidate cannot turn
into an unverified self-review. Neither preview nor exclusion consumes a route or
provider probe. When ordered tiers are disabled, preview observes the legacy
candidate order and existing circuit state.

A candidate with no explicit model uses the existing same-agent utility
default. The review keeps that selected default through detached execution and
fallback. An unavailable or different-agent default cannot prove a distinct
model and is refused; a default matching the writer is excluded. A profile
dispatch otherwise runs the profile's own model, or the agent's probed default
when the profile has none, so under exclusion the review passes the checked
model to the session and host profile prompts, which apply it in place of the
profile's model. A session executor that cannot accept a model refuses the
candidate.

## Workflow input

The built-in YAML loader accepts the existing run_code_review on-enter action
with an optional string agent_profile_id. It rejects non-string values. No
default workflow is altered or automatically enabled by this change.

## Limits

This depends on the existing native route owner and ordered-candidate work. A
preview is an observation, not a reservation; final selection retains its
existing ownership and health checks. No provider call is made by preview.
