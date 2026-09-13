# ADR-2026-09-10-pr-documentation-coverage: Require linked delivery context for pull requests

**Status:** proposed
**Date:** 2026-09-10
**Area:** workflow

## Context

PR #3137 adds worktree recovery behavior with no changed specifications or delivery records.
Its `fix:` title illustrates why feature-title checks cannot identify all behavior changes.
The diff does not establish which harness the contributor used.

The file-based knowledge system permits manually authored artifacts.
Enforcement should validate useful context while allowing small corrections without artificial documentation churn.

## Decision

Propose deterministic structural coverage with narrow path exemptions and the explicit `no-docs-allow` label.
Other changes require a changed work order linked to a plan, requirements, acceptance criteria, and system design.
Previously merged specifications can be referenced without cosmetic edits. Reviewers judge whether changed behavior needs updated contracts.
The override persists while the label is present and affects only this check.

CI owns this policy. It does not require use of a particular agent or prove that plans were committed before code.
The check reports failures before ruleset activation; mandatory merge enforcement requires a separately verified administrator rollout.

## Consequences

Results are reproducible and need no model credentials. Small code fixes and refactors can require a maintainer exception.
Structural links cannot prove semantic relevance. Review remains responsible for incomplete or misleading artifacts.
PR-wide exceptions can become stale as scope grows; maintainers must remove the label when it no longer applies.

## Alternatives considered

- Require documents for every PR: adds unnecessary work to tests, translations, and documentation corrections.
- Gate only `feat:` titles: misses behavior-changing fixes and can be bypassed by renaming a PR.
- Classify by line counts: a small protocol change can matter more than a large mechanical edit.
- Let a PR-body declaration exempt runtime changes: makes the policy self-waivable without the requested label.
- Use an AI classifier as the required gate: introduces cost, nondeterminism, and untrusted prompt inputs before a simple structural policy has been evaluated.
- Require changes to every linked document: encourages meaningless edits when an implementation follows an already merged design.

## Related artifacts

- [Requirements](../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../specs/ci/system-design/pull-request-documentation-coverage.md)
