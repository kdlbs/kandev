# ADR-2026-07-26-single-session-model-switching: Single-Session Model Switching

**Status:** accepted (amended 2026-07-27)
**Date:** 2026-07-26
**Area:** workflow, infra

## Context

Native custom subagents inherited the primary session's frontier model and, in
one implementation run, multiplied cached context and monitoring turns. The
result obscured the user's manual model choice and made a lower-cost
implementation switch ineffective.

## Decision

Kandev's durable planning and implementation workflow uses the user-started
primary conversation by default. Custom-agent definitions are removed across the
shared, Codex, Cursor, and OpenCode mirrors except for one read-only PR poller
per supported native platform. Platform-provided explorers and other
harness-managed investigation agents remain available. Feature work
creates durable specs, plans, and task files on a strong model, pauses for the
user to manually switch the same conversation to a lower-cost model, then
implements task files sequentially with TDD and their exact listed validation
commands.
Plans may label parallel-safe waves; native subagents are launched only after
the user explicitly authorizes them, with the selected active model and no
full-history context fork.

The PR poller is a narrow delivery exception. The primary conversation may
launch it only after the user explicitly asks to wait for CI or review updates.
It uses a cheap platform-specific model, reports a time-bounded status summary,
and cannot edit, remediate, post or resolve comments, or spawn children.

This decision supersedes [Planner Direct Small Work](2026-07-23-planner-direct-small-work.md)
and [Post-Commit Hook-Aware Verification](2026-07-23-post-commit-hook-aware-verification.md).

Bug fixes use the same artifact checkpoint: root-cause evidence, a behavioral
repair-spec amendment or concise repair spec, a fix plan, and task files are
created before implementation. The user reviews those artifacts before choosing
the implementation model and whether to authorize subagents.

The default Codex guidance is Sol/high for design and Terra/medium for
implementation and tests. Luna/low is limited to short mechanical read-only
work. An architectural, public-contract, persistence, or high-impact security
decision requires an explicit user-approved switch back to a strong model.

## Consequences

Model choice and spending are visible in one transcript, with no inherited
implementation-worker model or full-history fan-out. The PR poller adds a small
isolated context cost only for explicit waits, avoiding repeated expensive
primary-session polling. Task files become the durable handoff
between model tiers and define the only required pre-PR validation. They also
make parallelism visible without spending automatically. Broad local
simplify, QA, code-review, security-review, and full-verification passes are
not automatic; the two configured PR AI reviewers are the semantic-review gate.
Work is sequential and may take longer than parallel delegation, while being
cheaper and more predictable.

Fixes take longer to begin than an immediate patch, but gain a reviewable scope,
reproducible regression contract, and the same user-controlled implementation
handoff as feature work.

## Alternatives Considered

- Keep role-specific subagents with lower model pins: rejected because generic
  spawns could bypass the pins and inherit the primary model.
- Keep subagents but restrict their context forks: rejected because even bounded
  delegation adds routing, monitoring, and model-accounting complexity when it
  is automatic; accepted only as an explicit user-controlled choice.
- Use Luna for implementation by default: rejected because it is better suited
  to short mechanical read-only work than normal implementation and tests.
- Run local semantic-review and broad verification gates before every PR:
  rejected because they duplicate task-defined tests and the configured PR AI
  reviewers while increasing cost.
