---
id: "01-document-permission-boundary"
title: "Document the Git permission boundary"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/git-operations-permission-boundary.md"
---

# Task 01: Document the Git permission boundary

## Acceptance

- `docs/public/git-operations.md` explains that the Changes panel and agent
  shell use different Git permission paths.
- The public docs explain the `.git/index.lock` symptom, the Changes-panel
  recovery path, and agent-specific mode names.
- `docs/public/sessions-and-review.md` explains that Changes-panel commits use
  Kandev's control path and links to the Git reference.

## Verification

```shell
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Search the changed docs for `agent shell`, `index.lock`, `Changes panel`, and
`agent-specific` or equivalent wording. Confirm that the diff does not change
source code, navigation, or page frontmatter.

## Files likely touched

- `docs/public/git-operations.md`
- `docs/public/sessions-and-review.md`

## Dependencies

None.

## Parallelism

`sequential`. Both pages share the same terminology and acceptance criteria.

## Inputs

- Spec: `docs/specs/agents/requirements/git-operations-permission-boundary.md`.
- Plan: `docs/plans/git-operations-permission-boundary/plan.md`.
- Issue: [#2951](https://github.com/kdlbs/kandev/issues/2951).
- Existing Git execution path in `apps/backend/internal/agent/handlers/git_handlers.go`,
  `apps/backend/internal/agent/runtime/agentctl/git.go`, and
  `apps/backend/internal/agentctl/server/process/git.go`.
- Existing public-doc conventions in `docs/public/README.md`.

## Output contract

Report the exact public-doc validation results, changed sections, copy-review
terms, and any remaining wording risk. Update this task and `plan.md` in the
same conversation.

## Results

Updated the **Prerequisites and trust boundary** section in
`docs/public/git-operations.md` and the **Inspect changes** section in
`docs/public/sessions-and-review.md`.

The docs now explain the separate agent-shell and Changes-panel permission
paths, the `.git/index.lock` symptom, the Changes-panel recovery path, and
agent-specific mode names. The copy review found no new em dashes, semicolons,
contractions, or banned modal wording in the added text.

- `rtk node --test scripts/validate-public-docs.test.mjs`: 61 passed.
- `rtk node scripts/validate-public-docs.mjs`: 41 published pages validated.
- `rtk git diff --check`: passed.
- Acceptance-term search: found `agent shell`, `index.lock`, `Changes panel`,
  `agent mode`, `permission path`, and `agentctl` in the changed docs.
- No source-code, public navigation, or public-page frontmatter changes were made.
- No temporary files or runtime resources were created.

PR review remediation:

- Added separate guidance for permission-denied failures and active or stale
  `.git/index.lock` contention. The docs now explain that the Changes panel
  uses the same worktree and does not bypass an active lock.
- Updated the specification and specification index status to `shipped`.
- Re-ran the public-doc validation and diff checks after the remediation.
