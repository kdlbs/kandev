---
id: "06-public-docs"
title: "Document Copilot profile plugin behavior and limits"
status: pending
wave: 7
depends_on: ["03-acp-process-loading", "04-profile-process-parity", "05-profile-ui"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-004.4
  - AC-AGENTS-COPILOT-PLUGINS-004.6
  - AC-AGENTS-COPILOT-PLUGINS-005.2
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-006.4
  - AC-AGENTS-COPILOT-PLUGINS-007.1
  - AC-AGENTS-COPILOT-PLUGINS-007.2
  - AC-AGENTS-COPILOT-PLUGINS-007.5
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
---

# Task 06: Document native plugin behavior

## Summary

Document the delivered feature using the actual qualified runtime/source matrix and tested UI labels.
Keep current public behavior separate from unsupported or untested native combinations.

## In scope

- Use `/docs-maintainer` and `/simple-english` to add a focused Copilot section in `docs/public/agents-and-profiles.md`.
- Keep the existing how-to page structure. Explain select, Refresh, Save/Reset, and missing-selection repair.
- Explain whole-plugin execution, additive semantics, no global activation writes, local/worktree scope, and next-process timing.
- Describe native authentication conditions and isolated-executor errors without suggesting credential copying or login automation.
- Document version/source support from WO-00 and actual implementation results. Do not claim compatibility from supplied observations alone.
- Explain nonempty Copilot passthrough's selected managed command and the base-command preview's deferred mounts.
- Search root README and screenshot catalog for conflicting claims. Change them only if the delivered behavior makes their current text incorrect.
- Reconcile the requirement/design pair with implementation results and promote lifecycle status only after every work order passes.
- Record any missing native smoke/authentication/platform coverage in the compatibility evidence and handoff.

## Out of scope

Unrelated public-page rewrites, a new product landing page, screenshot/video production, runtime releases, commits, or publication.
Do not add a new public page/navigation entry when an existing profile section covers this task.

## Acceptance

1. Public guidance matches actual selection, process timing, source/runtime support, and repair behavior without whitelist or authentication guarantees.
2. All known native-policy/authentication limitations remain explicit, with no implication that directories or credentials are portable.
3. Public/internal documentation validators pass and work-order results retain exact checks and honest native coverage status.

## Verification

Run from the repository root. These commands validate documentation, not implementation correctness.

```bash
rg -n 'Copilot|copilot_selected_plugins|plugin-dir|selected plugins|plugin loading' docs/public README.md docs/screenshots.md docs/specs/agents/requirements/copilot-profile-plugins.md docs/specs/agents/system-design/copilot-profile-plugins.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Read every changed public paragraph beside the actual UI and WO-00 matrix.
Native acceptance commands/results must already be recorded by the work orders that own the behavior.
If a new native discrepancy appears, fix its owning work order and rerun its targeted checks.
Do not create a second broad verification phase here.

## Files likely touched

- `docs/public/agents-and-profiles.md`.
- `README.md` and `docs/screenshots.md` only if existing claims conflict.
- `docs/plans/copilot-profile-plugins/compatibility-evidence.md`, work-order results, and plan status.
- Paired requirement/design lifecycle metadata after all implementation work is complete.

## Dependencies

WO-03 through WO-05 must have actual passing implementation results. WO-00 support boundaries remain authoritative.

## Risks

Documentation can accidentally present native-enabled status as profile selection or promise exclusive loading.
An observed version on another machine is not proof for the current managed package/platform.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/copilot-profile-plugins.md#compatibility-qualification).
- Actual WO-00/03/04/05 results and the final UI labels.
- `docs/public/README.md`, `.agents/skills/docs-maintainer/SKILL.md`, and the existing runtime update guide.

## Results

Pending 2026-10-28. Public docs now describe the policy gate and do not claim native mounting support.

- Added a Copilot plugin how-to section to `docs/public/agents-and-profiles.md` covering selection, Refresh, Save/Reset, missing-plugin repair, whole-plugin execution, additive semantics, local/worktree scope, next-process timing, runtime qualification, and the deferred command preview.
- Documented native authentication/MCP policy ownership and the absence of credential copying or plugin management.
- `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`, and `python3 scripts/lint-spec-files.py --all`: passed.
- No conflicting claim was found in `README.md` or `docs/screenshots.md`, so neither changed.
