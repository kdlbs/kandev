# ADR-2026-09-03-task-environment-rehome-generations: Rehome missing task workspaces with atomic binding epochs

**Status:** accepted
**Date:** 2026-09-03
**Area:** backend, frontend, workflow

## Context

A task can retain a ready environment binding after an SSH or Coder task
directory disappears. Reusing that binding fails closed, but leaving the task
there strands workflow transitions. Mutating the old binding in place would
erase evidence about the missing workspace, while unconstrained replacement
can duplicate sessions and silently abandon unique repository work.

## Decision

Kandev represents recovery as an atomic binding-epoch reset on the task's
existing environment and session. A transactional compare-and-swap changes the
environment from ready or stopped to creating, clears stale provider handles
and repository inventory, and assigns the current session as materialization
owner. Automatic recovery requires durable evidence that the old workspace
contained no unique repository work; otherwise a stamped, human-authorized
action is required. The original launch receives at most one retry.

## Consequences

Task, environment, session, workflow position, durable conversation, and plan
artifacts keep their identities. The durable creating or failed states expose
interrupted recovery across restart. Recovery cannot restore bytes already
lost outside Kandev.

## Alternatives Considered

- Create immutable environment and session generations. Rejected because the
  existing state machines provide durable ownership and failure visibility
  without schema churn or duplicated conversation ownership.
- Always create a fresh workspace on a missing-directory error. Rejected
  because a missing directory is not proof that no unique work was lost.
- Keep requiring a successor task. Rejected because it breaks task/workflow
  identity and duplicates server-side artifacts.
- Retry indefinitely. Rejected because persistent configuration or provider
  failures would create unbounded workspaces and sessions.
