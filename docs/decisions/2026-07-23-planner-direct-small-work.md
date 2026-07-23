# Planner Direct Small Work

**Status:** accepted
**Date:** 2026-07-23
**Area:** workflow

## Context

Delegating every action reloads context and adds coordination/token cost, even
when one planner can safely complete a localized change faster.

## Decision

The planner is the default product and technical architect. It directly handles
one clear concern with a few localized files, no meaningful isolation or
parallelism benefit, and quick targeted verification. It delegates broad or
unknown exploration, substantial plan tasks, large/cross-component work,
parallel packets, long/noisy E2E or debugging, exceptional specialist review,
and final Spark `verify`; long PR monitoring stays with cheap `pr-poller`.
Architect is only a user-requested independent second opinion.

## Consequences

Small work avoids avoidable context reload while retaining skills, TDD,
dirty-worktree safety, focused checks, and final Spark verification for
code/test/config changes. Delegated workers retain one-packet/no-subagent
isolation. No product spec applies.

## Alternatives Considered

- **Delegate everything:** rejected for routine context and coordination cost.
- **Planner does everything:** rejected because independent evidence and long
  noisy work still benefit from specialization.
