---
id: "03-utility-execution"
title: "Authorized discovery and utility execution"
status: done
wave: 3
depends_on:
  - "02-wrapper-preparation"
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
  - AC-AGENTS-WRAPPERS-001.1
  - AC-AGENTS-WRAPPERS-001.2
  - AC-AGENTS-WRAPPERS-001.3
  - AC-AGENTS-WRAPPERS-001.4
  - AC-AGENTS-WRAPPERS-001.5
  - AC-AGENTS-WRAPPERS-002.5
  - AC-AGENTS-WRAPPERS-003.1
  - AC-AGENTS-WRAPPERS-003.2
  - AC-AGENTS-WRAPPERS-003.5
  - AC-AGENTS-WRAPPERS-004.1
  - AC-AGENTS-WRAPPERS-004.2
  - AC-AGENTS-WRAPPERS-004.5
  - AC-AGENTS-WRAPPERS-005.1
  - AC-AGENTS-PROFILE-DISCOVERY-001.3
  - AC-AGENTS-PROFILE-DISCOVERY-001.4
  - AC-AGENTS-PROFILE-DISCOVERY-001.5
  - AC-AGENTS-PROFILE-DISCOVERY-002.4
  - AC-AGENTS-PROFILE-DISCOVERY-002.5
system_design:
  - ../../specs/agents/system-design/generic-launch-wrappers.md
  - ../../specs/agents/system-design/profile-capability-discovery.md
---

# Task 03: Authorized discovery and utility execution

## Summary

Apply authorized generic wrappers to ACP discovery and profile-backed utility prompts. Keep trusted child-command admission independent of wrapper admission.

## In scope

- Add the versioned internal inference envelope and authenticated control-boundary validation. Reject unknown versions, null, and conflicting legacy prefix input.
- Construct provenance only after profile/draft authority checks. Preserve `org.config.manage`, secret scope, and custom-agent registration checks.
- Integrate shared preparation in ACP probe, ACP prompt, and OpenCode secondary listing. Test unavailable wrappers and same-context ACP fallback.
- Resolve secret-backed environment values in host and session-bound utility callers. Retain profile precedence and final stripping.
- Preserve cache generations and add wrapper grammar version to identity. Refuse native non-ACP wrapper contexts before startup. Sanitize utility errors and logs.

## Out of scope

No session configure changes, passthrough changes, plugin capability, or agentctl capability advertisement. Use explicit capability fixtures in tests until Task 05.

## Acceptance

1. Real subprocess fixtures prove matching wrapper argv and environment for baseline, model options, secondary discovery, and both utility prompt callers.
2. Unauthorized or forged requests cannot spawn. Discovery sends no prompt and makes no profile or task-session writes.
3. Missing wrappers, unsupported protocols, and cancellation fail without unwrapped fallback, leaked secrets, or surviving owned descendants.

## Verification

Run from the repository root. Implement new behavior with failing tests first.
New test and harness paths below are delivery targets, not files already available in this design package.

```bash
(cd apps/backend && go test ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/runtime/lifecycle ./internal/agentctl/server/utility ./internal/agentctl/server/api)
(cd apps/backend && go test -race ./internal/agent/hostutility ./internal/agentctl/server/utility -run 'GenericWrapper|ProfileProbe|ProfileCapability|ProbeCleansUp')
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths from this work order when Go files change.
Record the expanded command and result. Do not replace targeted checks with an unrecorded broad suite.

## Files likely touched

- `apps/backend/internal/agent/hostutility/{public,profile_probe,profile_capability_cache}.go` and `generic_wrapper_test.go`.
- `apps/backend/internal/agent/settings/handlers/profile_discovery_handlers.go` and `generic_wrapper_test.go`.
- `apps/backend/internal/agent/settings/controller/profile_discovery.go` and adjacent tests.
- `apps/backend/internal/agent/runtime/lifecycle/utility.go` and `generic_wrapper_utility_test.go`.
- `apps/backend/internal/agentctl/server/utility/{types,acp_executor,codex_app_server}.go`, native executor siblings, and `generic_wrapper_test.go`.
- `apps/backend/internal/agentctl/server/api/` inference admission and tests.
- `apps/backend/internal/agent/runtime/agentctl/` inference client and capability checks.

## Dependencies

[Task 02](task-02-wrapper-preparation.md). Read its recorded results before implementation.

## Risks

Changing the wrapper resolver alone would open admission without provenance and leave environment divergence. Test the full authenticated request-to-process path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/generic-launch-wrappers.md).
- [System design](../../specs/agents/system-design/generic-launch-wrappers.md), especially the sections that match this work order.
- [Plan](plan.md), compatibility matrix and test mapping.
- [Historical reproduction](reproduction.md), which is diagnostic evidence only.
- Scoped backend/agentctl guidance and nearby existing tests. For browser work, read web guidance and the e2e and mobile-parity skills.

## Results

Implemented the separate authenticated wrapper envelope for ACP discovery, prompts, and OpenCode secondary discovery. Inner commands retain their existing allowlist. Native non-ACP utility transports refuse wrappers. Profile secrets use the scoped resolver, and control credentials are removed after profile overrides.

Subprocess tests cover arbitrary executable paths, exact argv, wrapper receipts, failed launches, sanitized logs, and descendant cleanup. Auth tests cover valid control credentials, disabled authentication, and forged provenance. Old peers receive no wrapped dispatch.

Focused tests and race checks passed. Broad package failures and their unchanged-HEAD comparison are recorded in [acceptance.md](acceptance.md). No provider-name exception was added.
