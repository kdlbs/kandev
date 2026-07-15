---
name: docs-maintainer
description: Keep public Kandev docs current when code or behavior changes affect CLI commands, config keys, install/deploy flows, workflows, executors, APIs, screenshots, or user-facing terminology. Use this before finishing any change with public documentation impact, and when reviewing whether a change needs docs.
---

# Docs Maintainer

Use this skill to decide whether public docs need updates and to make those updates in the right place.

## Docs Boundaries

- Public website docs source lives under `docs/public/**`.
- Internal product/spec planning stays under `docs/specs/**`.
- Implementation plans stay under `docs/plans/**`.
- Architecture decisions stay under `docs/decisions/**`.
- Raw supporting notes can remain under `docs/**` outside `docs/public/**`, but do not publish them unless rewritten for users.
- The landing/docs website consumes public docs through its manifest and generated content. Do not hand-edit generated website docs.

## When Docs Need Updates

Check public docs when a change affects:

- CLI commands, flags, install commands, or runtime launch behavior.
- Configuration keys, environment variables, defaults, profiles, or feature flags.
- Workspaces, workflows, tasks, agents, executors, worktrees, Git behavior, or review flows.
- Docker, Kubernetes, service, desktop, remote environment, or Windows instructions.
- Public APIs, WebSocket messages, workflow import/export schemas, or integration contracts.
- Screenshots, visible UI labels, navigation, onboarding, or user-facing terminology.

Skip public docs when the change is:

- Purely internal refactoring with no behavior change.
- Test-only, fixture-only, or build-only without user-visible behavior.
- A speculative plan or design note that belongs in `docs/specs/**`, `docs/plans/**`, or `docs/decisions/**`.

## Workflow

1. Identify docs impact from the diff and changed behavior.
2. Search `docs/public/**` first for affected terms and commands.
3. If public docs exist, update them with the same PR as the behavior change.
4. If no public docs exist but the behavior is user-facing, add or propose the smallest useful public page/section.
5. If the change only updates implementation intent or architectural history, update specs/plans/ADRs instead.
6. Keep public docs task-oriented: prerequisites, commands, expected result, troubleshooting, and links to reference.
7. Preserve internal links inside `docs/public/**` where possible. Link to source-only raw docs only when the raw note is intentionally not published.
8. Note docs impact in the PR body.

## Validation

Run the checks relevant to your change:

```bash
# Replace SEARCH_TERM with the command, config key, or terminology that changed.
rg -n "SEARCH_TERM" docs/public docs/specs docs/decisions
```

For website docs publishing changes, also run from the landing repo:

```bash
pnpm --filter @kandev/docs fetch-docs
pnpm exec vitest run apps/docs/lib/docs-processing.test.ts
pnpm --filter @kandev/docs build
```

## Final Report

State one of:

- `Public docs updated:` with changed `docs/public/**` files.
- `Internal docs updated:` with changed specs/plans/decisions.
- `No docs change needed:` with one concrete reason.
