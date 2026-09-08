---
id: "05-document-merge-retry-behavior"
title: "Document merge retry behavior"
status: done
wave: 4
depends_on:
  - "03-expose-explicit-merge-retry"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.4
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.5
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.9
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
---
# Task 05: Document Merge Retry Behavior

## Summary

Explain how automatic merge binds the current head, pauses after a failed
unchanged attempt, and resumes through explicit retry or a new eligible state.

## In scope

- Update the public GitHub integration guide.
- Update the public session and review guide.
- Explain queue adoption and obsolete automatic-error reconciliation.
- Distinguish merge retry from state refresh.
- Use Simplified Technical English and current product terminology.
- Run public-documentation and link validation.

## Out of scope

- Internal schema or endpoint details.
- Documentation for GitLab automation.

## Acceptance

- Users can identify when Kandev retries automatically and when it waits.
- Users can distinguish Retry from Refresh.
- The guides explain behavior for active queue and merged pull requests.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
```

Run the commands from the repository root.

## Files likely touched

- `docs/public/integrations.md`
- `docs/public/sessions-and-review.md`

## Dependencies

- Task 03 finalizes the visible labels and retry outcome.

## Risks

- Documentation can drift if the final action label changes during implementation.
- The guide must not imply that Retry bypasses GitHub policy.

## Parallelism

`parallel-safe-with-task-04`

## Inputs

- Task 03 user-visible retry behavior.
- Integration acceptance criteria 002.4, 002.5, and 002.9.

## Results

- Documented that automatic merge binds the reviewed head and does not repeat
  an unchanged failed attempt without a new eligible state or explicit Retry.
- Explained that Retry requests one new evaluation for the selected pull
  request, while Refresh only reloads state and cannot authorize a merge.
- Documented that active queue and merged observations reconcile the attempt as
  accepted and clear only an obsolete automatic-merge error.
- Passed the public-document tests, public-document validator, and complete
  specification lint.
