---
id: "01-permission-intent"
title: "Default and execute Devin permission intent"
status: in-progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DEVIN-PERMISSIONS-001
acceptance_criteria:
  - AC-AGENTS-DEVIN-PERMISSIONS-001.2
  - AC-AGENTS-DEVIN-PERMISSIONS-001.3
  - AC-AGENTS-DEVIN-PERMISSIONS-001.4
  - AC-AGENTS-DEVIN-PERMISSIONS-001.5
  - AC-AGENTS-DEVIN-PERMISSIONS-001.6
  - AC-AGENTS-DEVIN-PERMISSIONS-001.7
system_design:
  - ../../specs/agents/system-design/devin-profile-permissions.md
---

# Task 01: Default and execute Devin permission intent

## Summary

Default new Devin profiles to Smart without changing existing profiles.
Use the saved ACP mode and sandbox environment override consistently during discovery and launch.

## In scope

- New-profile default helpers and mode/override validation.
- API creation, startup first provisioning, explicit update, duplication, and unchanged-profile compatibility cases.
- Real process-environment and requested-mode confirmation at existing probe/runtime seams.
- Conditional failure for unadvertised Smart, unsupported Autonomous, or sandbox initialization errors.

## Out of scope

- Frontend controls, localization, commercial model prompts, prerequisite installation, and general argument-order refactoring.
- Retroactive defaults, new persistent fields, or changes to organization or host approval policy.

## Acceptance

1. New UI-equivalent API and bootstrap profiles use the defined defaults, while saved records and explicit choices retain their intent.
2. The existing probe/launch boundaries deliver the correct sandbox environment and confirm the selected ACP mode before inference.
3. Unsupported modes or sandbox failures stay visible without another mode, an unsandboxed retry, or an implicit auto-approval change.

## TDD and scope details

First add `TestDevinProfileDefaults` in `controller/devin_profile_permissions_test.go`.
Assert Smart, explicit `DEVIN_SANDBOX=false`, and false auto-approval on a genuinely new default profile.
The current missing Smart default is the RED condition.
Cover create omission, null and empty mode, explicit modes, enabled sandbox, explicit false, and inherited values.
Assert that duplication preserves an empty mode and absent override exactly.
Assert that reconciliation of an existing row and a name-only update do not add new defaults.
An explicit PATCH empty mode remains empty.

Add pure tests in `profileconfig/devin_test.go` for atomic mode/override validation.
Preserve unrelated environment entries and CLI flags.
Reject explicit enabled sandbox plus Smart/Bypass before execution.
Keep secret-backed or custom legacy values intact unless their fields change explicitly.

Add `TestDevinSandboxProfileContext` at the controller/host utility boundary.
Compare saved and complete draft requests across baseline and model-option discovery.
Require literal true/false overrides to reach the child environment after sanitization.
Retain `ACP_BACKEND` stripping and current credential-selection behavior.
No test may log environment values beyond its synthetic fixture data.

Add `TestDevinPermissionModeLaunch` at the existing lifecycle mode-confirmation seam.
Use a fake ACP peer that advertises modes and records configuration changes before any prompt.
Require Smart or Bypass confirmation when advertised.
Reject missing Smart and missing Autonomous without a fallback prompt.
Cover an explicit sandbox startup failure and require only one sandboxed attempt.
Do not infer successful OS sandboxing from a fake subprocess test.

The current CLI documentation and investigation establish the mapping constraints.
A future real-runtime acceptance run can use a supported sandbox host with prerequisites.
Its result must distinguish session initialization from actual OS enforcement.
This work order does not require a commercial prompt or an OS security audit.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/settings/profileconfig ./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/hostutility -run 'Test.*(Profile|Devin|Mode)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*(Devin|Mode|Permission)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/settings/profileconfig/devin.go apps/backend/internal/agent/settings/profileconfig/devin_test.go apps/backend/internal/agent/settings/controller/profile_crud.go apps/backend/internal/agent/settings/controller/agent_discovery.go apps/backend/internal/agent/settings/controller/profile_discovery.go apps/backend/internal/agent/settings/controller/devin_profile_permissions_test.go apps/backend/internal/agent/hostutility/devin_profile_permissions_test.go apps/backend/internal/agent/runtime/lifecycle/devin_permission_mode_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Adjust the explicit lint list to the actual changed Go files, including any additional test files.

## Files likely touched

- New `apps/backend/internal/agent/settings/profileconfig/devin.go` and `devin_test.go`
- `apps/backend/internal/agent/settings/controller/profile_crud.go`
- `apps/backend/internal/agent/settings/controller/agent_discovery.go`
- `apps/backend/internal/agent/settings/controller/profile_discovery.go`, only for shared validation wiring
- New `apps/backend/internal/agent/settings/controller/devin_profile_permissions_test.go`
- New `apps/backend/internal/agent/hostutility/devin_profile_permissions_test.go`
- New `apps/backend/internal/agent/runtime/lifecycle/devin_permission_mode_test.go`

Keep production protocol handling generic. A failed existing mode-confirmation test requires a focused correction and matching coverage.

## Dependencies

Complete Tasks 01/02 in [the combined delivery package](../first-draft-profile-discovery/plan.md) before this companion's implementation.
This work order otherwise owns the initial backend result for the companion package.

## Risks

- Create defaults must not run on exact copies or on unrelated existing-profile updates.
- Root CLI permission flags do not establish ACP mode selection.
- Environment inheritance must remain distinguishable from explicit false.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/devin-profile-permissions.md)
- [System design](../../specs/agents/system-design/devin-profile-permissions.md)
- Existing profile CRUD, duplication, profile discovery, and generic ACP mode tests.

## Results

Completed 2026-11-03.

- New `internal/agent/settings/profileconfig/devin.go` owns new-profile defaults and atomic validation. It preserves inherited, secret-backed, and custom sandbox values.
- Defaults run only on creation paths (`CreateProfile`, nested agent creation, and first bootstrap provisioning). PATCH, duplication, DTO projection, and existing-profile reconciliation add nothing.
- `UpdateProfile` validates an explicitly changed mode/override pair, and contextual probe resolution validates the saved profile's effective pair before provider startup. `ErrInvalidDevinPermissionIntent` maps to HTTP 400 on create, update, and probe routes.
- Saved-profile discovery now validates the complete edited permission intent: a draft mode in the launch snapshot (or the resolve request's mode) wins over the saved mode, so enabling sandbox fixes Smart -> Autonomous and the reverse without a false conflict. Saved-only discovery keeps the persisted intent, and an omitted draft mode never invents a saved conflict.
- Tests: `TestDevinProfileDefaults` (create omission, explicit modes, enabled sandbox, explicit false, rejected conflict, duplication, name-only update, explicit empty mode, conflicting update), `TestDevinSandboxProfileContext` (saved and draft env reaches both probe paths), `TestSavedProfileDiscoveryUsesEditedPermissionIntent` (both directions, baseline and dependent resolution), `TestSavedProfileDiscoveryRejectsConflictingPermissionIntent`, `TestSavedProfileDiscoveryKeepsSavedIntentWithoutDraft`, the HTTP permission-intent mapping test, `TestDevinUnavailableSmartKeepsRequestedValue`, and the pure `profileconfig` table.
- Commands: `go test -trimpath -tags fts5 ./internal/agent/settings/profileconfig ./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1` passed.
- Remaining gap: the documented lifecycle acceptance (`TestDevinPermissionModeLaunch`, confirmed mode before inference, and the sandbox initialization failure) is not implemented. The task stays in progress until that seam is covered; no completion is claimed for it.
