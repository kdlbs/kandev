---
id: "01-draft-discovery-context"
title: "Remove the draft parent dependency"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
acceptance_criteria:
  - AC-AGENTS-PROFILE-DISCOVERY-001.1
  - AC-AGENTS-PROFILE-DISCOVERY-001.2
  - AC-AGENTS-PROFILE-DISCOVERY-001.3
  - AC-AGENTS-PROFILE-DISCOVERY-001.4
  - AC-AGENTS-PROFILE-DISCOVERY-001.5
  - AC-AGENTS-PROFILE-DISCOVERY-001.6
  - AC-AGENTS-PROFILE-DISCOVERY-002.1
  - AC-AGENTS-PROFILE-DISCOVERY-002.2
  - AC-AGENTS-PROFILE-DISCOVERY-002.3
  - AC-AGENTS-PROFILE-DISCOVERY-002.4
  - AC-AGENTS-PROFILE-DISCOVERY-002.5
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
---

# Task 01: Remove the draft parent dependency

## Summary

Allow both profile discovery endpoints to use a complete new draft without a persisted agent row.
Preserve strict saved-profile membership and the existing authority, launch, secret, and cache boundaries.

## In scope

- Separate registry identity from saved membership in the shared resolver.
- Use the canonical registry ID for every new-draft scope, including drafts whose parent already exists.
- Keep persisted parent/profile lookups mandatory for every supplied `profile_id`.
- Add controller and HTTP regressions before changing production behavior.
- Reuse existing utility, launch validation, and error mappings.

## Out of scope

- Persistence during discovery, startup synchronization changes, UI changes, new protocols, or new cache infrastructure.
- General provider diagnostics or unrelated error cleanup.

## Acceptance

1. Devin and Qwen drafts without parents reach both healthy utility methods and return capabilities with either refresh value.
2. Invalid contexts fail before execution, and saved requests always retain their persisted ownership checks.
3. Discovery writes no agent/profile records, and identical draft scopes remain stable across explicit parent creation.

## Regression design

Add `TestFirstDraftProfileDiscoveryWithoutAgentRow` in a new controller test file.
Reuse `newProviderTestController` and `recordingProfileDiscoveryUtility`.
The default fixture contains Codex and Claude parents but no Devin or Qwen parents.
Cover both agents, initial requests and refresh, and both public controller methods.
Assert returned models, modes, option results, context revision, forwarded snapshot, and refresh flag.
Assert no parent exists after each successful endpoint call.
Add a model-option request recorder that captures its `ProfileContext`.

The first RED condition is a valid missing-parent draft returning `ErrAgentNotFound` instead of reaching the utility.
Capture that failure before changing the resolver.
The temporary investigation test from the plan was removed and is not permanent coverage.

Add `TestFirstDraftProfileDiscoveryScope`.
Compare the baseline and option contexts for the same request.
Repeat after an explicit parent insert and require the same draft scope.
Compare another registered agent, another authorization scope, and a saved profile.
Require different scopes for those different identities.
Update `TestFetchProfileDynamicModelsUsesCompleteDraftSnapshot` to expect the registry ID rather than the fixture UUID.
Keep saved-profile scope assertions unchanged.

Add table-driven refusal cases in `TestFirstDraftProfileDiscoveryValidation` and `TestSavedProfileDiscoveryRequiresParentOwnership`.
Cover unknown and disabled optional agents, non-inference entries, empty authority, incomplete/null snapshots, and unavailable global or workspace secrets.
Retain explicit empty arrays and an empty prefix as valid input.
Cover a supplied missing profile, missing parent, another agent's profile, workspace or role profiles, and dynamic profiles.
A supplied stale `profile_id` with valid launch settings must still fail.
Preserve repository error propagation on the saved path.
Use the existing optional-agent policy rather than a new enablement interpretation.

Add `TestFirstDraftProfileDiscoveryEndpoints` in a new handler test file.
Use the fake repository, registered agents, and the synthetic settings identity to prove the route, auth, and draft-identity boundary.
Assert HTTP 200 for `/probe` and contextual `/resolve` without a parent.
The controller regression `TestFirstDraftProfileDiscoveryWithoutAgentRow` uses a healthy recording utility and asserts its calls plus the returned capability and option data, so execution is proven at that seam rather than through the HTTP fake.
Assert 401 without identity, 403 without config authority, 400 for invalid launch input, and 404 for invalid targets.
Preserve the existing contextual and context-free endpoint distinctions.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/hostutility -run 'Test(Profile|ManagerProfile)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/settings/controller/profile_discovery.go apps/backend/internal/agent/settings/controller/profile_discovery_test.go apps/backend/internal/agent/settings/controller/profile_discovery_first_draft_test.go apps/backend/internal/agent/settings/handlers/profile_discovery_first_draft_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If implementation changes another Go file, include it in the changed-file lint command.
Do not treat diagnostic success as a GREEN regression result.

## Files likely touched

- `apps/backend/internal/agent/settings/controller/profile_discovery.go`
- `apps/backend/internal/agent/settings/controller/profile_discovery_test.go`
- New `apps/backend/internal/agent/settings/controller/profile_discovery_first_draft_test.go`
- New `apps/backend/internal/agent/settings/handlers/profile_discovery_first_draft_test.go`

Read `controller/agent_config.go` and `handlers/profile_discovery_handlers.go` as callers.
Production changes there are unnecessary unless evidence identifies a second defect.
The shared resolver also serves Copilot plugin inventory after that route's own parent validation.
Keep that route's saved-agent admission intact.

## Dependencies

None.

## Risks

- A missing profile must not become an authorized new draft.
- Parent UUIDs cannot remain in the new-draft scope without changing identity after the first save.
- Preserve inference and optional-agent checks before any utility call.

## Parallelism

`sequential`

## Inputs

- [Plan and reproduction evidence](plan.md#evidence-and-root-cause)
- [Requirements](../../specs/agents/requirements/profile-capability-discovery.md)
- [Identity before the first profile save](../../specs/agents/system-design/profile-capability-discovery.md#identity-before-the-first-profile-save)
- Existing `provider_config_test.go`, `profile_discovery_test.go`, and `model_config_handlers_test.go` fixtures.
- Existing `hostutility/profile_capability_cache_test.go` and `profile_probe_test.go` isolation tests.

## Results

Completed 2026-11-03 in the primary session.

- RED: the new `TestFirstDraftProfileDiscoveryWithoutAgentRow` subtests for Devin and Qwen returned `agent not found` from both controller endpoints until the resolver changed.
- `resolveProfileProbeAgent` now takes a saved-parent requirement. An omitted `profile_id` resolves the registry identity only; a supplied `profile_id` still requires the persisted parent row, and repository failures propagate instead of falling through to the draft path.
- New draft scope is `<authorization-scope>:draft:<registry-id>` for every no-profile request, and it remains stable after an explicit parent insert.
- Added `controller/profile_discovery_first_draft_test.go` (draft success for both agents and refresh values against a recording utility, scope isolation, validation/refusal table, saved-profile ownership, repository-failure propagation) and `handlers/profile_discovery_first_draft_test.go` (HTTP routing: 200 for both endpoints without a parent, 400/401/403/404 refusals, and the saved-profile permission-intent mapping). The controller regression is the utility-execution evidence; the handler test owns HTTP status mapping because the handler package has no utility injection seam.
- Commands: `go test -trimpath -tags fts5 ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/hostutility -count=1` passed; `go build ./...` passed.
