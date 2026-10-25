# ADR-2026-10-25-agent-profile-change-token: Track Every Profile Mutation in the Database

**Status:** accepted
**Date:** 2026-10-25
**Area:** backend, persistence

## Context

Exact task-profile assignments must reject a profile that changed after assignment, even when its visible values later return to their earlier state. The `updated_at` field is written by several services and is not a reliable change token: writers can omit it, reuse a timestamp, or restore earlier values. Settings and Office both write `agent_profiles`.

## Decision

The database owns a persistent, monotonic revision for each profile ID. Insert and update triggers advance a separate `agent_profile_revisions` row, including writes from services outside the settings store. An update advances the revision when an identity or configuration column changes, including a change that later returns to an earlier value. Operational-only updates to status, run identifiers, timestamps, sort order, failure counters, or pause reason do not advance it. The row survives profile deletion so reusing an ID cannot reuse a revision. Profile reads obtain the revision in the same query as the profile fields. A missing revision is a failed read, not an unchanged profile.

SQLite and PostgreSQL install equivalent triggers after legacy table-recreation migrations. Existing profiles start at revision one only when no revision row exists. Consumers of an exact assignment compare this token with the assignment's captured token and must reject missing or mismatched state before launch. A future migration changing this token contract must invalidate or transform existing assignments explicitly.

## Consequences

All profile writers advance the token for identity and configuration changes without remembering a per-service convention. Operational-only updates leave it unchanged. The token represents relevant mutation history, not a content hash or wall-clock time. Exact assignment code must use the persisted revision from the resolved profile snapshot and keep assignment and launch checks consistent.

## Alternatives Considered

- Compare `updated_at`: misses writers that do not update it and does not prevent timestamp reuse or ABA.
- Hash profile fields: returns to the same value after an intervening change, so an old assignment can appear current.
- Increment a column in every application writer: fragile while settings, Office, migrations, and direct SQL share the table.
