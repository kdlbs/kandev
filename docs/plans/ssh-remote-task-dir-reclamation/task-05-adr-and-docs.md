---
id: "05-adr-and-docs"
title: "Reclamation ADR and shipped-behavior documentation"
status: done
wave: 3
depends_on: ["03-cleanup-job-phase"]
plan: "plan.md"
requirements: "../../specs/executors/requirements/remote-task-directory-reclamation.md"
acceptance_criteria:
  - AC-SSH-TASKDIR-RECLAMATION-006.1
  - AC-SSH-TASKDIR-RECLAMATION-004.1
system_design: "../../specs/executors/system-design/remote-task-directory-reclamation.md"
---

# Task 05: Reclamation ADR and shipped-behavior documentation

## Summary

Record the two durable decisions in an ADR, and update the one public document
that describes shipped user-visible behavior. The `AGENTS.md` correction and the
`spec.md` scope move ship with the design package rather than here.

## Scope

- New ADR via `/record decision` covering: (a) reclamation is opt-in per executor
  profile and defaults off, because existing hosts hold directories created under
  a documented no-delete promise and an upgrade must not retroactively break it;
  (b) the removal lives in the durable task-resource cleanup job rather than
  `SSHExecutor.StopInstance`, because the stop path is per-session, swallows
  errors for terminal targets, and lacks the task graph needed for ownership.
- `docs/public/feature-status.md` SSH executor row updated to describe the
  shipped behavior, replacing the current "automatic directory deletion are not
  provided" clause.
- `docs/decisions/INDEX.md` entry.

## Exclusions

- `apps/backend/AGENTS.md` and `docs/specs/executors/system-design/ssh-executor.md` are corrected as
  part of the design package, not here.

## Implementation acceptance conditions

1. The ADR states both decisions with their rejected alternatives.
2. The feature-status SSH row describes reclamation as opt-in per profile, gated
   on unpushed-work and ownership checks, and terminal-outcome-only.
3. No Unicode em dash in changed user-facing lines.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `docs/decisions/<adr-id>.md` (new)
- `docs/decisions/INDEX.md`
- `docs/public/feature-status.md`

## Dependencies

Task 03 — the docs describe behavior that must exist first.

## Inputs

- Requirements `REQ-SSH-TASKDIR-RECLAMATION-005` and `-006` for the rationale.
- `/docs-maintainer` for the public-docs pass.

## Risks

Documenting the feature as on-by-default would mislead operators into thinking
their disk is being reclaimed when it is not.

## Safety

No deletion path is reachable from this task.

## Output contract

Set `status` to `done`, tick the box in `plan.md`, and report the ADR ID and the
documents changed.
