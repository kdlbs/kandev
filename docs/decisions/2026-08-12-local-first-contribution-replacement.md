# ADR-2026-08-12-local-first-contribution-replacement: Keep Remote Contribution Work Local-First

**Status:** accepted
**Date:** 2026-08-12
**Area:** backend, frontend, protocol, security, GitHub, GitLab

## Context

Kandev detects when a provider rewrites a contribution branch after task creation. The first drift
design preserved both histories and blocked Push, force-push, and Pull. That design protected data but
left the user without a recovery action. The user had to leave Kandev and select a Git strategy.

The task checkout is the user's active workspace. The provider branch is the published contribution.
Neither version can silently replace the other after divergence.

## Decision

Kandev keeps the task checkout as the primary working version after remote drift. Drift detection stays
non-destructive. The Changes panel shows a compact remote-change status and keeps the provider version
available through a secondary disclosure.

A user can select **Replace PR branch** to publish the task version. This action requires a destructive
confirmation and an exact expected provider head. The Git push uses an explicit force-with-lease value
for the contribution branch. A lease mismatch changes no remote refs.

A user can select **Use PR version** to adopt the provider version. Kandev requires a clean working tree.
It fetches and checks the expected provider head before local mutation. It creates a recovery branch at
the current task HEAD before it resets the task branch.

The managed replacement actions are session-scoped user actions. Kandev does not expose them through
agent MCP tools or automatic Git operations. Generic contribution force-push requests remain rejected.
Direct terminal commands remain outside this UI approval boundary.

Normal linear histories keep their current behavior. A provider-ahead branch offers Pull. A local-ahead
branch offers normal Push. Generic Pull stays unavailable for diverged histories because it requires a
merge strategy.

## Consequences

- Users can continue local work after a provider rewrite.
- Users select which version wins instead of selecting a Git strategy.
- Remote replacement can remove provider commits. The confirmation and exact lease make this effect
  explicit and protect changes that arrive after confirmation.
- Adopting the provider version adds a local recovery branch.
- Desktop and mobile require the same choices, safety conditions, and outcomes.
- The replacement flow applies to one selected repository. It never performs a destructive multi-repo
  fan-out.

## Alternatives Considered

### Keep all remote actions blocked

Rejected. This policy protects data but gives the user no way to complete the task in Kandev.

### Treat the provider version as automatically correct

Rejected. An automatic reset can erase task work and makes an external rewrite authoritative without
user intent.

### Rebase or merge automatically

Rejected. These actions select a conflict policy and can create new history. The user asked to choose
which version wins, not how Git combines them.

### Allow an unrestricted contribution force-push

Rejected. A plain force-push can overwrite a provider update that arrives after the confirmation. An
exact lease preserves the user's intent boundary.
