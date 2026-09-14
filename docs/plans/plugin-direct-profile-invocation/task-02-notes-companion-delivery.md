---
id: "02-notes-companion-delivery"
title: "Notes companion manifest, errors, and evidence"
status: done
wave: 1
depends_on:
  - "01-direct-profile-invocation"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-DIRECT-PROFILE-INVOCATION-001
acceptance_criteria:
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.1
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.4
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.5
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.8
system_design:
  - ../../specs/plugins/system-design/plugin-direct-profile-invocation.md
---

# Task 02: Notes Companion Manifest, Errors, and Evidence

## Summary

Update the Notes plugin (yattdev/kandev-plugin-notes at
`f8548f9d7a1eaabf19d27bb0b0af8f75d8eaeec4`, PR 7) to declare the
agent-profile-backed setting, translate `FailedPrecondition` into an
actionable setup response, align packaging, and record final delivery
evidence.

## In scope

- Notes manifest declaration and contract-pinning tests for the
  agent-profile field name and format.
- Refresh the enhancement/proofread path to send the selected stable profile
  ID in the `Host.InvokeUtilityAgent` payload.
- Translate `FailedPrecondition` webhook responses to HTTP 412 with an
  actionable profile-selection/setup message; keep operational failures as
  502-class execution errors.
- SDK target output update, README/usage-copy alignment to a single direct
  profile selection, packaging metadata consistency, and test tooling pin
  alignment (Kandev SDK v0.2.4).
- Final PR body, visual evidence, and Notes-side read-me screenshots.

## Out of scope

- Host-side routing, eligibility, translation, and UI rendering, covered by
  Task 01.
- Legacy `utility_agent` semantics for other plugins.

## Acceptance

- Notes declares and pins the agent-profile settings field, its payload
  carries the stable profile ID, and its UI shows the selected profile label.
- A missing or ineligible profile yields HTTP 412 with actionable copy, not
  a generic 500/502.
- Notes packaging, version metadata, doc links, and screenshots are
  consistent with the shipped manifest and SDK.

## Verification

```bash
cd kandev-plugin-notes
go test ./...
node --test ui/bundle.test.mjs
go vet ./... && test -z "$(gofmt -l .)"
```

## Files likely touched

- kandev-plugin-notes `internal/server`, `manifest`, `webhook`, `go.mod`,
  `go.sum`, README, docs links, packaging, UI bundle smoke tests.

## Dependencies

- `01-direct-profile-invocation`

## Risks

- Full-replacement plugin config updates can drop fields unless the plugin's
  hydration writes the complete record.
- Committing PR screenshots requires binary assets on the Notes fork head.

## Parallelism

`sequential`

## Inputs

- `docs/specs/plugins/requirements/plugin-direct-profile-invocation.md`
- `docs/specs/plugins/system-design/plugin-direct-profile-invocation.md`
- kandev-plugin-notes Task 01 implementation and Kandev SDK contract.

## Results

- Notes PR 7 CI green (`Build plugin packages`,
  `Tidy, format, vet, and test`, `GitGuardian`).
- Notes PR body corrected to reference the commands actually run in QA.
- Draft statuses cleared by marking Notes PR 7 (`f8548f9d7...`), Notes PR 7,
  and Kandev PR 2870 (`0f36c0a6b...`) ready for review; both are CLEAN and
  MERGEABLE at their exact delivery heads.
