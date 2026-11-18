# ADR-2026-11-16-ui-screenshot-evidence: Require screenshot presence on UI PRs

**Status:** proposed
**Date:** 2026-11-16
**Area:** workflow

## Context

The PR skill already asks for screenshots, but contributors can submit without using that skill.
A required check can catch missing screenshots without building a screenshot-verification service.

## Decision

For candidate UI changes, require a Markdown image in the PR's Screenshots section.
Apply the same rule to internal PRs and external forks.
Use trusted code and PR metadata only.
Reuse the existing label-exception pattern with `no-screenshots-allow`.
Add the status to the target ruleset after implementation and authorized smoke checks.

## Consequences

The check establishes image presence. Authors and reviewers retain responsibility for freshness, visibility, viewport coverage, and relevance.
No image downloader, commit declaration, custom comment command, or permission registry is needed.
A persistent exception label needs maintainer attention when PR scope changes.
Merge-queue support remains separate because current main does not use a queue.

## Alternatives considered

- Skill instructions alone: contributors can bypass them.
- Any image anywhere: unrelated badges can qualify.
- A checked checkbox: no image is necessary.
- Image downloads and commit declarations: more machinery without proof that an image shows the changed UI.
- Custom exception commands: duplicate GitHub's existing label controls.

## Related artifacts

- [Requirements](../specs/ci/requirements/ui-screenshot-evidence.md)
- [Design](../specs/ci/system-design/ui-screenshot-evidence.md)
- [Plan](../plans/ui-screenshot-enforcement/plan.md)
