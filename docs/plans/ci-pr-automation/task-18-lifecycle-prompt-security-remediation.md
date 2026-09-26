---
id: "18-lifecycle-prompt-security-remediation"
title: "Lifecycle prompt security remediation"
status: done
wave: 10
depends_on:
  - "17-final-lifecycle-transition-coverage"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 18: Lifecycle prompt security remediation

## Acceptance

- Lifecycle review-requested, merged, and closed prompts are immutable,
  versioned server-owned templates. Their only dynamic value is a validated
  canonical GitHub PR URL; untrusted GitHub text and caller-supplied prompt
  content cannot enter an automated agent turn.
- HTTP and current-task MCP reject lifecycle override fields. The lifecycle
  controls remain partial, idempotent, current-task-bound booleans.
- The additive legacy lifecycle-override columns are retained for replay
  compatibility but cleared at startup and ignored at runtime.
- Lifecycle queue acceptance and prompt claim require an active task. Archive
  or deletion winning the race creates no checkpoint, message, or prompt;
  acceptance winning is canceled by normal archive semantics.
- Durable ADR, spec, plan, task records, and affected public GitHub integration
  documentation state the final contract without claiming lifecycle overrides.

## Verification

```bash
rg -n 'review_prompt_override|merged_prompt_override|closed_prompt_override|lifecycle prompt override|custom lifecycle prompt' docs
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs
```

## Files touched

- `apps/backend/internal/github/{controller,models,service_ci_automation,store}.go`
- `apps/backend/internal/mcp/handlers/task_pr_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_pr_automation.go`
- `apps/backend/internal/orchestrator/messagequeue/`
- focused backend regression tests
- `docs/decisions/0051-pr-agent-notifications-extend-task-pr-automation.md`
- `docs/specs/ui/requirements/ci-pr-automation.md`
- `docs/plans/ci-pr-automation/`
- `docs/public/integrations.md`

## Dependencies

- Completed in the shared worktree: lifecycle override removal from HTTP,
  current-task MCP, and effective prompt resolution; canonical lifecycle
  prompts; and archive-safe delivery linearization.
- Plan dependency: Task 17 final lifecycle transition coverage.

## Constraints

- Preserve the existing task PR automation subsystem, one-minute PR watch
  poller, per-PR checkpoints, and durable queue. Do not add a scheduler,
  generic automation subsystem, replacement session, or public customization
  contract.
- Preserve normal archive cancellation semantics when lifecycle acceptance wins
  the archive race.
- Do not commit, push, or alter unrelated application behavior.

## Output contract

- Mark this task done only after backend remediation is complete, durable and
  public docs match the immutable prompt contract, and the listed validation
  commands pass.
- Report changed files, stale-claim search results, validation results,
  blockers, residual risks, and any divergence from the plan.
