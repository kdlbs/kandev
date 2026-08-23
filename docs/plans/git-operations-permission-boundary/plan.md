---
spec: docs/specs/agents/git-operations-permission-boundary.md
created: 2026-08-23
status: implemented
---

# Implementation Plan: Agent Git Permission Boundary

## Overview

Issue [#2951](https://github.com/kdlbs/kandev/issues/2951) requests public
documentation for the difference between Git commands from an agent shell and
Git operations from the Changes panel. Source inspection confirms that the
panel sends `worktree.commit` through Kandev's backend and `agentctl` Git
operator, while the agent shell remains subject to the installed agent's mode.

The implementation updates the existing Git reference and the Changes-panel
guide. It does not change backend, frontend, agent, or permission behavior.

## Public documentation

### Git operations reference

Update `docs/public/git-operations.md` in **Prerequisites and trust boundary**.
Explain the separate permission paths, the `git status` versus write failure
case, the `.git/index.lock` diagnostic, the Changes-panel recovery path, and
agent-specific mode names.

### Sessions and review guide

Update `docs/public/sessions-and-review.md` in **Inspect changes**. State that
Changes-panel Git operations use Kandev's control path rather than the agent's
shell, and link readers to the Git reference for the permission-boundary
troubleshooting steps.

## Tests

- **What:** The updated pages retain valid frontmatter, navigation, links, and
  published-page structure.
  **File:** `docs/public/git-operations.md` and
  `docs/public/sessions-and-review.md`.
  **How:** Run the public-docs unit tests and validator.
- **What:** The copy covers the issue acceptance criteria without changing
  implementation behavior.
  **File:** the same two public-doc pages.
  **How:** Review the diff and search for the permission-boundary terms listed
  in the task file.

## E2E Tests

No E2E test is planned. This change documents existing behavior and does not
change the UI, WebSocket contract, or Git execution path.

## Verification Results

- `rtk node --test scripts/validate-public-docs.test.mjs`: 61 passed.
- `rtk node scripts/validate-public-docs.mjs`: 41 published pages validated.
- `rtk git diff --check`: passed.
- Acceptance-term search passed for the agent shell, `.git/index.lock`, Changes
  panel, agent mode, permission path, and `agentctl` wording.
- No source-code, public navigation, or public-page frontmatter changes were made.
- PR review remediation added lock-specific recovery guidance and marked the
  implemented specification as `shipped`.

## Implementation Waves And Parallel Candidates

The default execution order is sequential in the primary conversation.

Wave 1:

- [x] [task-01-document-permission-boundary](task-01-document-permission-boundary.md)

The task owns both public-doc files because the copy and terminology must stay
consistent. It is not parallel-safe.

## Risks And Out-of-scope Work

- Do not imply that the Changes panel changes the agent's permission mode.
- Do not describe mode names as Kandev-wide labels. Names come from the
  installed agent.
- Do not promise that an agent can write Git metadata in every executor.
- Do not change Git execution, sandbox, or permission behavior in this task.
