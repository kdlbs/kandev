# ADR-2026-11-07-isolated-managed-npm-cache-recovery: Recover incomplete npm execution trees without user maintenance

**Status:** accepted
**Date:** 2026-11-07
**Area:** backend, agentctl, protocol

## Context

An upgrade selected newer Claude ACP, Codex ACP, and Codex app-server versions.
All three installed package directories existed. Their npm execution trees
lacked root manifests and executable links. npm exited with `ENOENT` before
protocol initialization, so discovery failed and session resume fell back to
workspace inspection. The original cause of the missing files remains unknown.

An isolated reproduction failed with the same incomplete tree layout. A fresh
installation of each exact package completed protocol initialization.
An online metadata retry cannot restore files when npm treats the package as
already installed. Automatic deletion can disrupt a healthy sibling process.

The user requested automatic recovery after restart, including existing broken
installations. Manual update and fresh conversations do not meet that outcome.

## Decision

Extend the existing private-preparation boundary to verified incomplete npm
execution trees. Require a registered exact managed command, a matching
pre-initialization npm failure, and independent executor-local inventory.
Neither diagnostic paths nor package suggestions grant installation authority.

Prepare the same selected package in a private npm project and cache. Validate
its package identity and executable before the single replacement starts.
Preserve the shared tree, registry, authorized context, and conversation.
Apply this rule to managed ACP and managed Codex app-server discovery and tasks.

Reuse existing retry deadlines, cancellation, cleanup, and generation guards.
Prepared files belong to the local agentctl owner until all consumers stop.
Normal startup discovery and eligible session-open resume perform recovery.
No database migration, feature setting, startup-wide scan, or user action is
required. Existing restrictions on automatic session starts remain effective.

This extends the eligibility in the [bounded retry decision](2026-10-02-bounded-managed-npm-startup-retry.md).
It reuses the preparation policy in the [Codex dependency decision](2026-10-19-isolated-managed-dependency-recovery.md).
It does not enable arbitrary dependency repair or shared-tree mutation.
Both draft packages must share one implementation of private preparation.

## Consequences

Existing broken caches can recover after an upgrade without manual npm work.
Healthy runtimes retain their normal launch path. Preparation can still fail
under registry, policy, permission, disk-space, or deadline constraints.
Those failures remain visible through existing recovery controls.

Private installations consume disk space and can repeat after owner teardown.
This change does not promote a persistent repaired runtime or add a cache janitor.
An old agentctl peer cannot perform the new repair without capability support.

## Alternatives considered

- Delete or reinstall the shared tree automatically: can disrupt sibling processes.
- Recreate npm manifests and links manually: couples Kandev to npm install internals and does not validate the remaining installation.
- Retry the same npx command: reproduces the confirmed missing-manifest failure.
- Change the runtime version or use a global binary: changes the selected runtime without user intent.
- Require a repair button: does not meet the requested upgrade and restart behavior.
- Scan every cache at backend startup: adds unrelated work and cannot inspect remote caches correctly.
- Persist repaired runtime generations: introduces promotion and storage coordination beyond this bounded recovery.

## References

- [Incomplete-cache requirements](../specs/agents/requirements/managed-npm-cache-recovery.md)
- [Shared runtime recovery contract](../specs/agents/requirements/managed-npm-runtime-recovery.md)
- [Cache recovery design](../specs/agents/system-design/managed-npm-cache-recovery.md)
- [Implementation package](../plans/managed-npm-cache-recovery/plan.md)
