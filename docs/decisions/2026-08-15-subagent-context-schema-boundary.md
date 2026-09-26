# ADR-2026-08-15-subagent-context-schema-boundary: Start with the execution-aware subagent schema

**Status:** accepted
**Date:** 2026-08-15
**Area:** backend

## Context

The subagent context feature stores `agent_execution_id` in its uniqueness key.
The feature branch also contained a dialect-specific rebuild for an earlier
two-column table shape. That earlier shape is not present on the supported base
branch, so the rebuild and its activation marker would add upgrade paths for a
database state that Kandev has never shipped.

## Decision

Create `task_session_subagents` directly in its final execution-aware shape from
`initSubagentContextSchema`. Keep `runMigrations()` responsible only for the
historical-message backfill and its two backfill activation keys. Do not add a
schema-shape probe, a legacy-table rebuild, or a separate execution-schema
activation key until a shipped predecessor creates a real compatibility need.

## Consequences

Startup has one schema definition and one backfill path for this feature. The
SQLite and PostgreSQL implementations do not need parallel cutover logic or
failpoint-only test infrastructure. A database created by an unreleased
intermediate build is outside the supported upgrade contract and would need a
separate, evidence-based migration if that build is ever shipped.

## Alternatives Considered

- **Keep the legacy rebuild and execution activation key.** Rejected because it
  preserves substantial dialect-specific code for an unshipped schema and makes
  the migration order harder to reason about.
- **Ship the two-column table first, then migrate it later.** Rejected because it
  creates avoidable production state and a compatibility obligation for a
  feature whose final key is already known.
