---
id: "01-complete-design"
title: "Complete the generic launch wrapper design"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-WRAPPERS-001
  - REQ-AGENTS-WRAPPERS-002
  - REQ-AGENTS-WRAPPERS-003
  - REQ-AGENTS-WRAPPERS-004
  - REQ-AGENTS-WRAPPERS-005
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
acceptance_criteria:
  - AC-AGENTS-PROFILE-DISCOVERY-001.3
  - AC-AGENTS-PROFILE-DISCOVERY-001.4
  - AC-AGENTS-PROFILE-DISCOVERY-001.5
  - AC-AGENTS-PROFILE-DISCOVERY-002.4
  - AC-AGENTS-PROFILE-DISCOVERY-002.5
system_design:
  - ../../specs/agents/system-design/generic-launch-wrappers.md
  - ../../specs/agents/system-design/profile-capability-discovery.md
---

# Task 01: Complete the generic launch wrapper design

## Purpose and scope

Turn the [investigation](plan.md) into a reviewable requirements, system-design,
plan, and implementation-work-order package. This is a design work order, not
authorization to implement the rejected exact-`greywall` exception.
The linked acceptance criteria are design inputs that constrain the package.
Task 01 completion does not establish runtime satisfaction of those criteria.
Implementation and regression evidence belong to Tasks 02 through 06.
The new wrapper requirements define the additional behavior.
The optional plugin evaluation is complete and defers a new capability.

No production code, permanent tests, plugin implementation, or public behavior
claims belong to this work order.

## Work

1. Re-read the reproduction and current source paths. Trace every launch path,
   including host discovery and Pi passthrough. Verify current operator authority
   instead of relying on the older ADR's unauthenticated-deployment description.
2. Resolve the five decision groups in the plan. Prefer generic structured wrapping.
   do not require a named-provider adapter for basic prefix support. Ask focused
   questions only for decisions that existing direction and contracts do not settle.
3. Choose the durable contract owner and write or update the requirements and design
   pair. Keep runtime path exposure, permission grants, and verified confinement
   distinct. Define unsupported paths and their failure behavior.
4. Decide whether the first increment needs a plugin capability. If it does, specify
   typed inputs/results, trust, lifecycle ownership, version compatibility, and
   failure behavior over existing plugin infrastructure. Do not repurpose the
   remote-environment provider interface for local wrapping.
5. Replace this design handoff with dependency-ordered implementation work orders
   and meaningful regression checks. Retain the Pi/Greywall scenario and generic
   wrapper fixtures. Return the package for review before production edits.

## Completion criteria

- Requirements and design resolve authority, runtime inputs, execution scope,
  compatibility, and the optional plugin boundary without machine-specific constants.
- Each supported launch path has planned success, refusal, and cleanup coverage.
- Existing discovery criteria remain traceable. new contracts have acceptance criteria.
- The superseded provider-name exception is not an implementation instruction.
- Plugin APIs, overrides, approvals, and learning are specified or explicitly deferred.

## Validation

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the relevant documentation checks after edits. Do not claim production tests
pass on the basis of the temporary overlay. Future implementation work orders
must specify their focused Go tests and any UI verification.

## Results

Completed the design artifacts on 2026-10-16. The catalog, 36 linter tests, full specification lint, six-work-order coverage preflight, local links, and diff checks passed.

- Created the draft generic-wrapper requirement and system-design pair under Agents.
- Resolved authority, execution-host references, launch coverage, version compatibility, and plugin scope.
- Created Tasks 02 through 06 with dependencies, acceptance criteria, owned files, and targeted checks.
- Preserved the historical reproduction and the manual instance. The local handoff file was unavailable.
- Production changes, permanent tests, and provider acceptance remain pending.

Detailed commands and results appear in [the plan](plan.md#verification-results).
