---
id: "13-public-docs"
title: "Public docs for native code review"
status: pending
wave: 10
depends_on: ["12-e2e-and-mock-agent"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 13: Public docs for native code review

## Inputs

- Run `/docs-maintainer` and follow it; this task records the required edits.
- Spec **What**, **Failure modes**, **Persistence guarantees** — user-facing behavior only, no internals.

## Work

1. `docs/public/sessions-and-review.md` — a new section under "Review a diff" covering: how to start a review, what a finding is, the three per-finding actions, that findings are advisory and nothing is auto-applied, per-repository grouping, stale behavior, that findings persist server-side (unlike pending inline comments, which are browser-local), and the utility-agent dependency with the Settings path. Add troubleshooting rows for "Review changes is unavailable" and "findings show as stale".
2. `docs/public/tasks-and-workflows.md` — add the **Run code review** entry action to the step-settings table, and note in "Build a human gate" that a review step can precede a human gate and that a failed review does not block the transition.
3. `docs/public/feature-status.md` — add a row for native code review; update the "Changes and cumulative Review" row to mention agent findings; confirm the "Utility agents and custom one-shot helpers" dependency-bound row still reads correctly for this path.
4. `docs/public/automation-and-mcp.md` — add `publish_review_findings_kandev` to the task-MCP tool list if that page enumerates tools.

## Acceptance

- No implementation detail (table names, package paths, Go types) appears in public docs.
- Every documented behavior matches a spec scenario or failure mode.

## Verification

Read each edited page end to end and confirm the terminology matches the shipped UI labels (**Review changes**, **Resolve**, **Dismiss**, **Send to agent**, **Run code review on entry**).

## Files likely touched

`docs/public/{sessions-and-review.md,tasks-and-workflows.md,feature-status.md,automation-and-mcp.md}`.

## Output contract

Summary, files changed, blockers, risks, `status: done`, plan checkbox.
