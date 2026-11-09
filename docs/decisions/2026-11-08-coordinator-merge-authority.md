# ADR-2026-11-08-coordinator-merge-authority: Repository-scoped coordinator merge authority

**Status:** accepted
**Date:** 2026-11-08
**Area:** workflow

## Context

Phase 2 reserved merging to people. Managers now need different levels of coordinator authority for different repositories, without extending the class eligibility mechanism to irreversible merges.

## Decision

Amend the phase 2 and phase 3 human-only merge boundary behind the coordinator merge flag. A manager chooses denied, approval or automatic per coordinator and repository; denied is the default. Execution checks the change and uses provider head protection, with no force, push or bypass. A merge cannot be undone by Kandev. Before enabling automatic, a repository whose own contributor rules reserve merging to a named maintainer must have its owner amend those rules. Kandev neither reads nor enforces those files.

## Consequences

Kandev rechecks immediately before dispatch and sends the expected head SHA, the only atomic guard supplied by the request. It reads and records which repository protections enforce pipeline, approvals and resolved threads; an unsupported or failed read is unknown. Provider rules remain authoritative. Conditions a provider does not enforce can change between the last check and acceptance; settings state this known window. A provider rejection or an unmerged reconciliation refuses the proposal and lowers automatic authority; preflight refusals do not. An automatic proposal waits for conditions such as a pending pipeline, rather than refusing at creation, and merge is the one kind the automatic path re-evaluates after creation. Change conditions on watched cards also wake the coordinator, which amends the phase 3 own-task boundary (see its 2026-11-08 amendment), as the phase 2 ADR is amended for merging. Automatic actions share admission and the daily budget; failures and confirmed reversals lower authority. This creates an irreversible action and provider reconciliation burden.

## Alternatives Considered

Keep merges human-only: rejects the requested repository-specific autonomy. Make merge another raisable class: conflates a repository decision with coordinator-wide evidence and undo assumptions.
