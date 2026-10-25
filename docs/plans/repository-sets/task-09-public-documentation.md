---
id: "09-public-documentation"
title: "Public documentation for repository sets"
status: done
wave: 8
depends_on: ["08-end-to-end-coverage"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 09: Public Documentation For Repository Sets

## Acceptance

- The public task-creation and workspace-repository docs under `docs/public/**` describe repository
  sets: what a set is, that it holds repositories and not branches, how to define one from workspace
  settings or from the create dialog, and how applying one fills the picker.
- The new REST endpoints appear wherever the public API surface is documented, with the request and
  response shapes from the spec.
- Terminology is consistent with the UI copy shipped in tasks 05 to 07 ("repository set", not
  "preset" / "group" / "bundle"), and no screenshot or example contradicts the shipped behavior.

## Verification

```bash
cd /home/jeremy-coatelen/.kandev/tasks/have-pre-defined-set_jmk3jo4l/kdlbs-kandev
rg -n "repository set" docs/public | head -40
```

## Files likely touched

- `docs/public/**` (task creation, workspace repositories, and API reference pages)

## Dependencies

Tasks 05 to 08, so the documented behavior and copy are the shipped ones.

## Inputs

- Run `/docs-maintainer`: this change adds public API endpoints, a settings surface, a task-creation
  flow, and new user-facing terminology.
- Spec: What, API surface, Out of scope (say plainly that branches, remote URLs, and Quick Chat are
  not covered).

## Risks

- Documenting sets as carrying branches would contradict the model and mislead users; the docs must
  state that branches stay per task.

## Output contract

Summary, files changed, commands run with results, blockers, risks, divergence from the plan, and
task/plan status updates.
