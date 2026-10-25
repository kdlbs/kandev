---
id: "06-document-discovery-recovery"
title: "Document desktop discovery recovery"
status: completed
wave: 6
depends_on:
  - "03-repository-discovery-ux"
  - "04-filesystem-access-diagnostics"
  - "05-idle-workspace-access"
plan: "plan.md"
requirements:
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-002"
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-003"
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-004"
acceptance_criteria:
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.15"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.17"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-004.2"
system_design:
  - "../../specs/workspaces/system-design/local-repositories.md"
---

# Task 06: Document Desktop Discovery Recovery

## Summary

Update public desktop and configuration documents. Explain explicit roots,
Reconnect, unsigned-update limits, and diagnostic path disclosure.

## In scope

- Explain server roots and desktop-selected roots.
- Explain Home exclusions and explicit protected-folder selection.
- Explain cache refresh, denial, removal, and Reconnect.
- Disclose local paths in exported diagnostics.
- Update scoped engineering guidance only if implementation changes a convention.

## Out of scope

- Promise permanent macOS consent for an unsigned application.
- Add a signing or Apple developer-account procedure.
- Repeat the full system design in public documentation.

## Acceptance

- Public docs distinguish backend launch policy from client picker capability.
- Recovery text states the unsigned-update limit and gives the Reconnect action.
- Documentation validation and specification lint pass.

## Verification

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `docs/public/desktop-app.md`
- `docs/public/configuration.md`
- Diagnostic guidance under `docs/public/`
- Scoped `AGENTS.md` files only when implementation changes a convention
- Plan Results sections after implementation

## Dependencies

- Tasks 03, 04, and 05 define the final behavior and recovery data.

## Risks

- Documentation can imply that a folder selection survives every unsigned update.
- Diagnostic guidance can omit that local paths are sensitive data.

## Parallelism

`sequential`

## Inputs

- Completed implementation results from Tasks 01 through 05.
- REQ-WORKSPACES-LOCAL-REPOSITORIES-002 through 004.
- Current public desktop and configuration documents.

## Results

Completed.

- Documented launch policy, browser versus native picker capability, Home
  confirmation, protected-folder behavior, cache refresh, reconnect/remove
  recovery, unsigned-update limits, and sensitive diagnostic paths.
- Updated public coverage metadata and the plan package results.
- Verification passed: public documentation tests and validation plus
  `python3 scripts/lint-spec-files.py --all`.
