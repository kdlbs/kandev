---
id: "02-subscription-policy"
title: "Unify subscription launch and ACP readiness"
status: done
wave: 2
depends_on:
  - 01-local-runtime
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-004
  - REQ-AGENTS-ANTIGRAVITY-ACP-005
  - REQ-AGENTS-ANTIGRAVITY-ACP-006
  - REQ-AGENTS-ANTIGRAVITY-ACP-008
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-011
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-003.1
  - AC-AGENTS-ANTIGRAVITY-ACP-003.2
  - AC-AGENTS-ANTIGRAVITY-ACP-003.3
  - AC-AGENTS-ANTIGRAVITY-ACP-003.4
  - AC-AGENTS-ANTIGRAVITY-ACP-003.5
  - AC-AGENTS-ANTIGRAVITY-ACP-003.6
  - AC-AGENTS-ANTIGRAVITY-ACP-003.7
  - AC-AGENTS-ANTIGRAVITY-ACP-003.8
  - AC-AGENTS-ANTIGRAVITY-ACP-003.10
  - AC-AGENTS-ANTIGRAVITY-ACP-003.11
  - AC-AGENTS-ANTIGRAVITY-ACP-003.12
  - AC-AGENTS-ANTIGRAVITY-ACP-003.13
  - AC-AGENTS-ANTIGRAVITY-ACP-004.1
  - AC-AGENTS-ANTIGRAVITY-ACP-004.3
  - AC-AGENTS-ANTIGRAVITY-ACP-004.4
  - AC-AGENTS-ANTIGRAVITY-ACP-004.5
  - AC-AGENTS-ANTIGRAVITY-ACP-004.6
  - AC-AGENTS-ANTIGRAVITY-ACP-004.7
  - AC-AGENTS-ANTIGRAVITY-ACP-004.8
  - AC-AGENTS-ANTIGRAVITY-ACP-004.10
  - AC-AGENTS-ANTIGRAVITY-ACP-005.1
  - AC-AGENTS-ANTIGRAVITY-ACP-005.2
  - AC-AGENTS-ANTIGRAVITY-ACP-005.4
  - AC-AGENTS-ANTIGRAVITY-ACP-005.5
  - AC-AGENTS-ANTIGRAVITY-ACP-005.6
  - AC-AGENTS-ANTIGRAVITY-ACP-005.7
  - AC-AGENTS-ANTIGRAVITY-ACP-005.8
  - AC-AGENTS-ANTIGRAVITY-ACP-005.9
  - AC-AGENTS-ANTIGRAVITY-ACP-006.1
  - AC-AGENTS-ANTIGRAVITY-ACP-006.2
  - AC-AGENTS-ANTIGRAVITY-ACP-006.3
  - AC-AGENTS-ANTIGRAVITY-ACP-006.4
  - AC-AGENTS-ANTIGRAVITY-ACP-008.1
  - AC-AGENTS-ANTIGRAVITY-ACP-008.2
  - AC-AGENTS-ANTIGRAVITY-ACP-008.3
  - AC-AGENTS-ANTIGRAVITY-ACP-011.1
  - AC-AGENTS-ANTIGRAVITY-ACP-011.2
  - AC-AGENTS-ANTIGRAVITY-ACP-011.3
  - AC-AGENTS-ANTIGRAVITY-ACP-010.1
  - AC-AGENTS-ANTIGRAVITY-ACP-010.2
  - AC-AGENTS-ANTIGRAVITY-ACP-010.3
  - AC-AGENTS-ANTIGRAVITY-ACP-010.6
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 02: Unify subscription launch and ACP readiness

## Summary

Make tasks, probes, inference, restore, and registered-agent diagnostics use one personal subscription policy.
Explicitly authenticate the native server without opening consent browsers during background operations.

## In scope

- Add shared pure Antigravity compatibility helpers for `zed` 0.0.0, advertised personal auth, and bounded consent prompt parsing.
- Apply child-only credential-mode sanitation after profile environment merge. Reject incompatible HOME/GEMINI_HOME and unsupported locality.
- Carry method and interactive permission in existing instance/inference configuration. Enable personal policy only for eligible local Antigravity, with interaction reserved for operator setup.
- Negotiate `antigravity_personal_auth_v1` and validate the internal `acp_auth_policy` contract before using it with retained agentctl.
- Resolve the trusted harness and use a private no-op browser helper in the actual child environment. Test path/argument quoting on OS families.
- Reuse existing stderr consumer/sanitizer hooks; recognize consent before generic buffers/logs and return auth-required promptly in background mode.
- Preserve native credential backend selection and let the vendor handle cached credentials and refresh.
- Explicitly authenticate before new/load/resume, with process-generation checks and bounded cleanup.
- Retain initialize capabilities and method order when a later auth/session operation fails.
- Reuse policy in utility execution and registered-agent acpdbg without changing raw `--exec` semantics.
- Preserve shared CLI flags, prefixes, lifecycle methods, MCP advertisements, independent task processes, and other providers.

## Out of scope

Interactive settings flow, new account stores, remote auth relay, provider-error journal reading, and new model catalogs.

## Acceptance

1. Every supported local surface launches the same runtime/home and sends the same Antigravity identity, selecting personal OAuth without API-key fallback.
2. Missing/expired consent returns auth-required with preserved initialize data and no browser popup, leaked URL, process, or listener.
3. Restore, concurrent tasks, ordinary ACP agents, shared flags, and existing remote declarations retain their contracts.

## Verification

Run from the repository root. Task 01 supplies the dependency install.

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/acpcompat -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/agents ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run 'TestAntigravity' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/utility -run 'TestAntigravity|TestMaybeEmitAuthRequired|TestSanitizeEnvForAgent|TestProbeConfigOptions' -count=1)
(cd apps/backend && go test -trimpath ./cmd/agentctl ./internal/agent/acpdbg ./cmd/acpdbg -run 'TestAntigravity|Test.*Initialize' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp ./internal/agent/hostutility -run 'TestAntigravity' -count=1)
git diff --check
```

Named new tests: `TestAntigravityClientInfo`, `TestAntigravityConsentURL`, `TestAntigravityBrowserHelper`, `TestAntigravitySessionAuthentication`, `TestAntigravityBackgroundConsent`, `TestAntigravityLocalLaunchPolicy`, `TestAntigravityHostLaunchPolicy`, `TestAntigravityProbeAndInferencePolicy`, and `TestAntigravityDebuggerPolicy`.
Prove both conflicting ambient and explicit profile inputs are sanitized, while unrelated env survives and parent env is unchanged.
Test cached personal authentication, missing method, stale process generation, timeout, consent detection, two tasks, and stopping one task.
Test browser helper construction and execution on each claimed platform, including spaces and Windows drive letters.
Run changed-Go lint against all changed/new Go files.

## Files likely touched

- `apps/backend/internal/agentctl/acpcompat/antigravity.go` and tests (new).
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`, `adapter.go`, `adapter_session.go`, and focused auth tests.
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`, environment helpers, and new Antigravity probe tests.
- `apps/backend/internal/agentctl/server/process/` stderr and startup integration, using existing consumer/sanitizer interfaces.
- Existing agentctl instance, config, client, and inference DTO contracts carrying the typed local auth policy.
- `apps/backend/cmd/agentctl/main.go`, `antigravity_browser.go`, and helper tests.
- `apps/backend/internal/agent/runtime/lifecycle/` and `internal/agent/hostutility/` effective launch policy integration.
- `apps/backend/internal/agent/agents/antigravity_acp.go` and baseline tests.
- `apps/backend/internal/agent/acpdbg/` registered-agent selection/initialize paths and tests; `cmd/acpdbg/` if wiring requires it.

## Dependencies

Task 01 provides the common local runtime resolver.

## Risks

- Native authentication may attempt a browser on missing credentials. Suppression must occur before the vendor calls webbrowser.
- Python splits BROWSER differently from shell argv. A Unix-only no-op or naive colon quoting is insufficient.
- Retained agentctl instances need explicit capability/version checks when new auth status behavior is required.

## Parallelism

`sequential`

## Inputs

- Design sections Launch policy, Shared ACP compatibility, Background operations, and Concurrency and cleanup within Authentication.
- ADR 0044 and existing Grok shared compatibility normalization.
- Current ACP stderr consumer/sanitizer and utility stderr sink behavior.

## Results

Implemented.

- Added `internal/agentctl/acpcompat/antigravity.go` with the `zed` 0.0.0 identity, `oauth-personal` selection, consent URL parsing/validation, human-input prefix recognition, credential-mode environment names, home/store validation, and Python BROWSER helper formatting.
- Threaded the typed `acp_auth_policy` through lifecycle, agentctl instance/config/adapter, and utility inference DTOs. Local task launches and host utility probes select `oauth-personal`; interactive permission is reserved for the dedicated setup instance.
- Added the private `antigravity-noop-browser` agentctl subcommand and inject `BROWSER` only into the Antigravity child environment.
- ACP initialize uses the same `zed` identity for live, probe, inference, and registered-agent acpdbg paths. Background consent returns `auth_required` and cleans up the temporary process.
- Added compatibility tests for identity, consent URL validation, environment validation, browser helper, and auth status.

Verification: targeted `go test` across `acpcompat`, `agents`, `hostutility`, `lifecycle`, utility, ACP adapter, `acpdbg`, and `cmd/agentctl`; `go test -race` for the ACP adapter Antigravity tests; changed-Go lint 0 issues.

Live cached-credential and fresh-consent behavior remains Task 07 evidence.

Review remediation: the resolved harness now travels with the managed server through task, utility, sign-in, and acpdbg launches; credential stripping is scoped to the active local policy; retained agentctl must advertise `antigravity_personal_auth_v1`; and utility admission preserves the vetted absolute managed executable.
