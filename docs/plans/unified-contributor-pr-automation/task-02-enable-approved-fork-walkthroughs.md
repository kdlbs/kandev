---
id: "02-enable-approved-fork-walkthroughs"
title: "Enable approved fork walkthroughs"
status: done
wave: 2
depends_on: ["01-unify-fork-approval-label"]
plan: "plan.md"
spec: "../../specs/ci/requirements/unified-contributor-pr-automation.md"
system_design: "../../specs/ci/system-design/unified-contributor-pr-automation.md"
---

# Task 02: Enable approved fork walkthroughs

## Acceptance

- A non-draft fork pull request with `safe-to-review` can start walkthrough
  generation on the supported pull request events when
  `PR_WALKTHROUGH_ENABLED` is true.
- The existing `CLAUDE_REVIEW_ALLOWLIST` direct path can authorize an opening
  or update event without relying on a label event emitted by `GITHUB_TOKEN`.
- An unauthorized fork, a draft pull request, and a fork with only
  `safe-to-test` skip the generation job.
- Same-repository generation and the
  `generate-pr-walkthrough` same-repository rerun label remain unchanged.
- The generation job checks out and verifies `github.workflow_sha`, fetches
  `refs/pull/<number>/head`, verifies `FETCH_HEAD` against the exact event head
  SHA, and does not check out or execute contributor code in the generation
  worktree.
- Artifact generation, R2 publication, public validation, and PR-body linking
  retain their existing contracts and permissions.

## TDD scenarios

1. RED: Extend the walkthrough contract test for the fork approval expression,
   direct allowlist path, exact pull request ref fetch, and fail-closed old
   label behavior.
2. GREEN: Update the walkthrough job condition and trusted object fetch.
3. GREEN: Update the legacy walkthrough specification and contract comments.
4. REFACTOR: Keep the generation, publication, and link jobs separate and keep
   the secret-bearing worktree on the trusted workflow SHA.

## Verification

```bash
python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows/pr-walkthrough.yml
git diff --check
```

## Files likely touched

- `.github/workflows/pr-walkthrough.yml`
- `.github/scripts/pr-walkthrough-workflow-contract_test.py`
- `docs/specs/pr-walkthrough/spec.md`

## Dependencies

- Task 01 must complete first so the walkthrough uses the final label
  contract.

## Parallelism

Sequential. This task owns the fork walkthrough workflow and its trusted input
provenance.

## Risks

- Fetching a contributor ref into the object database must not replace the
  trusted workflow checkout or trusted skill bundle.
- A short or stale SHA must not produce a published walkthrough. Assert the
  exact full SHA before context preparation.
- The manual rerun label must remain restricted to same-repository pull
  requests.

## Output contract

Report the fork gate, exact SHA evidence, artifact and link behavior, all
contract and security checks, and one live post-merge verification plan. Update
this task and `plan.md` with the result.

## Results

The walkthrough generation gate now accepts `safe-to-review` and the existing
`CLAUDE_REVIEW_ALLOWLIST` path for contributor pull requests. It fetches the
base repository pull request ref and verifies the exact event head SHA before
context preparation. Walkthrough generation, publication, linking, and the
trusted workflow asset boundary remain unchanged.

The walkthrough contract, context helper, renderer, PR-body helper, action
pinning, specification lint, and diff checks passed. Direct `zizmor` reports
only the intentional `pull_request_target` trigger finding for this workflow.

The final PR verification also passed the walkthrough generation, publication,
linking, and preview checks with no unresolved review threads. Post-merge live
verification remains the planned operational follow-up.
