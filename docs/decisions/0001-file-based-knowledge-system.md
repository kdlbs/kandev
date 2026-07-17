# 0001: File-based knowledge system

**Status:** accepted
**Date:** 2026-03-28 (updated 2026-07-16)
**Area:** infra

## Context

Agents working on Kandev had no way to record architectural decisions or store implementation plans for future reference. This led to repeated questions about "why was X done this way?" and lost context when features were revisited. The project needed a knowledge system that works across agent providers (Claude Code, Codex, Copilot) without custom infrastructure.

## Decision

Use a three-tier, file-based knowledge system:

- **Tier 1 (always loaded):** `CLAUDE.md` stays slim and points to Tier 2 indexes.
- **Tier 2 (index files):** `docs/decisions/INDEX.md` and `docs/specs/INDEX.md` — one-line-per-entry tables that agents read to find relevant items.
- **Tier 3 (individual files):** Individual ADRs (`docs/decisions/<adr-id>.md`) and feature specs (`docs/specs/<slug>/spec.md`), loaded only when needed.

ADRs `0001` through `0042` retain their numeric IDs. New ADRs use decentralized,
date-prefixed IDs in the form `YYYY-MM-DD-short-title`, with a sufficiently specific slug to avoid
same-day collisions. The complete filename stem is the stable ADR ID. This removes the need for
parallel branches to reserve a shared next number.

Architecture decisions are recorded as ADRs (this file is an example). Product features are captured as specs under `docs/specs/<slug>/` — the "what & why", committed to git. Implementation plans (`docs/specs/<slug>/plan.md`) and post-ship notes (`docs/specs/<slug>/notes.md`) live alongside specs but are **gitignored** — they are ephemeral working files regenerated from the spec as needed, not permanent records.

A `/record` skill creates ADRs and a `/spec` skill creates specs, but the system works without them — agents can create files directly following the conventions.

ADRs are reserved for durable architectural constraints, boundaries, contracts, ownership rules,
operational invariants, and repo-wide conventions for which meaningful alternatives existed.
Simple features belong in specs, implementation sequencing belongs in plans, and local tactics or
ordinary bug fixes remain in code, tests, and commit history unless they establish a rule that
future work must follow.

## Consequences

- Agents can discover past decisions by reading a small index file, then drill into specific ADRs.
- No file grows unbounded — each decision is its own file.
- Knowledge is committed to git and survives across sessions, branches, and agent providers.
- The `/feature` skill integrates with the decision log (reads in Phase 2, writes in Phase 6).
- Requires discipline to record decisions — this is a convention, not an enforcement mechanism.
- Date-prefixed IDs are longer than sequence numbers, but branches can create them independently
  without filename collisions over the next shared integer.

## Alternatives Considered

- **Continue allocating sequential ADR numbers:** Rejected for new ADRs because parallel branches
  must read and update the same allocation point, producing duplicate IDs and avoidable conflicts.
- **Use random UUIDs:** Rejected because they avoid coordination but make filenames and references
  difficult to scan. A date plus specific slug is decentralized while remaining recognizable.
- **Database-backed memory (SQLite, vector search):** Too complex, requires infrastructure, doesn't work for agents that only read files.
- **Append-only daily logs (OpenClaw pattern):** Good for persistent agents but Kandev agents are session-scoped — daily logs would accumulate noise.
- **Auto-compaction system:** Not needed — the tiered index approach prevents unbounded growth in the first place.
- **CLAUDE.md inline decisions:** Would make CLAUDE.md grow unbounded and overwhelm agent context.
