# 0001: File-based knowledge system

**Status:** accepted
**Date:** 2026-03-28 (updated 2026-05-15)
**Area:** infra

## Context

Agents working on Kandev had no way to record architectural decisions or store implementation plans for future reference. This led to repeated questions about "why was X done this way?" and lost context when features were revisited. The project needed a knowledge system that works across agent providers (Claude Code, Codex, Copilot) without custom infrastructure.

## Decision

Use a three-tier, file-based knowledge system:

- **Tier 1 (always loaded):** `CLAUDE.md` stays slim and points to Tier 2 indexes.
- **Tier 2 (index files):** `docs/decisions/INDEX.md` and `docs/specs/INDEX.md` — one-line-per-entry tables that agents read to find relevant items.
- **Tier 3 (individual files):** Individual ADRs (`docs/decisions/NNNN-*.md`) and feature specs (`docs/specs/<slug>/spec.md`), loaded only when needed.

Architecture decisions are recorded as ADRs (this file is an example). Product features are captured as specs under `docs/specs/<slug>/` — the "what & why", committed to git. Implementation plans (`docs/specs/<slug>/plan.md`) and post-ship notes (`docs/specs/<slug>/notes.md`) live alongside specs but are **gitignored** — they are ephemeral working files regenerated from the spec as needed, not permanent records.

A `/record` skill creates ADRs and a `/spec` skill creates specs, but the system works without them — agents can create files directly following the conventions.

## Consequences

- Agents can discover past decisions by reading a small index file, then drill into specific ADRs.
- No file grows unbounded — each decision is its own file.
- Knowledge is committed to git and survives across sessions, branches, and agent providers.
- The `/feature` skill integrates with the decision log (reads in Phase 2, writes in Phase 6).
- Requires discipline to record decisions — this is a convention, not an enforcement mechanism.

## Alternatives Considered

- **Database-backed memory (SQLite, vector search):** Too complex, requires infrastructure, doesn't work for agents that only read files.
- **Append-only daily logs (OpenClaw pattern):** Good for persistent agents but Kandev agents are session-scoped — daily logs would accumulate noise.
- **Auto-compaction system:** Not needed — the tiered index approach prevents unbounded growth in the first place.
- **CLAUDE.md inline decisions:** Would make CLAUDE.md grow unbounded and overwhelm agent context.
