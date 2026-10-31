---
id: "03-native-qualification"
title: "Record native plugin diagnostics and limitations"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.1
  - AC-AGENTS-COPILOT-PLUGINS-003.2
  - AC-AGENTS-COPILOT-PLUGINS-003.3
  - AC-AGENTS-COPILOT-PLUGINS-004.1
  - AC-AGENTS-COPILOT-PLUGINS-004.7
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-007.1
  - AC-AGENTS-COPILOT-PLUGINS-007.3
  - AC-AGENTS-COPILOT-PLUGINS-007.6
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 03: Native diagnostics

## Summary

Record exact-package native observations, a reusable opt-in test harness, and
honest limitations. The user replaced privileged policy certification as a
production prerequisite with
[native Copilot authority](../../decisions/2026-10-28-copilot-native-policy-authority.md).
Completion here means diagnostic delivery, not all native cases passing or
enterprise enforcement being certified.

## In scope

- Evaluate exact `1.0.88` and `1.0.104` packages independently.
- Use temporary plugin/MCP fixtures, repeated structured mounts, and actual
  endpoint traffic. Preserve existing authentication without copying it.
- Record coexistence, native-disable observations, and unavailable managed
  policy separately from harness/setup errors.
- Keep the `copilot_native` external acceptance harness and focused offline
  contract coverage; ordinary tests stay credential-free.
- Preserve sanitized evidence in the original compatibility ledger.

## Out of scope

Production admission changes, administrator rights, real system/MDM/enterprise
policy changes, login/token fetching, plugin management, and blanket consent.
An unset fixture selector is not a native incompatibility verdict.

## Acceptance

1. Both exact candidates have commands, binary/platform identity, observations,
   failures, and limitations; no absent/failed case is described as passed.
2. The opt-in harness is discoverable and fails honestly when required
   diagnostic controls are missing. That result does not gate production.
3. Focused offline checks pass and all owned processes/listeners/runtime roots
   are cleaned up without changing production or real policy/auth stores.

## Verification

Run from the worktree root. These are ordinary/offline checks:

```bash
(test -x apps/backend/bin/acpdbg || make -C apps/backend build-acpdbg)
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/copilotplugins)
(cd apps/backend && go test -trimpath -tags copilot_native -count=1 -run '^TestNative' -v ./internal/agent/copilotplugins)
(cd apps/backend && go test -race -trimpath -tags copilot_native -count=1 -run '^(TestNativeMCPSSESentinelProtocol|TestNativeMCPHTTPSentinelReadOnlyContract|TestNativePermissionAllowsAnnotatedSentinelWithoutACPKind)$' ./internal/agent/copilotplugins)
(cd apps/backend && go test -trimpath -tags copilot_native -list . ./internal/agent/copilotplugins)
bash scripts/lint-go-changed apps/backend/internal/agent/copilotplugins/native_acceptance_test.go apps/backend/internal/agent/copilotplugins/native_acceptance_acp_test.go apps/backend/internal/agent/copilotplugins/native_acceptance_cases_test.go apps/backend/internal/agent/copilotplugins/native_acceptance_contract_test.go apps/backend/internal/agent/copilotplugins/native_acceptance_process_other_test.go apps/backend/internal/agent/copilotplugins/native_acceptance_process_unix_test.go apps/backend/internal/agent/copilotplugins/native_acceptance_sse_test.go
git diff --check
```

The following explicit native commands were used with an owned private
`KANDEV_COPILOT_NATIVE_ARTIFACT_DIR`. They are diagnostics, not mandatory release
gates, and can exit nonzero when an unavailable enterprise control is required:

```bash
(cd apps/backend && KANDEV_COPILOT_NATIVE_VERSION=1.0.88 go test -trimpath -tags copilot_native -count=1 -timeout 10m ./internal/agent/copilotplugins -run '^TestCopilotPluginNativeAcceptance$' -v)
(cd apps/backend && KANDEV_COPILOT_NATIVE_VERSION=1.0.104 go test -trimpath -tags copilot_native -count=1 -timeout 10m ./internal/agent/copilotplugins -run '^TestCopilotPluginNativeAcceptance$' -v)
```

## Files likely touched

The seven `native_acceptance*_test.go` files named by the lint command above,
the original compatibility ledger, and synchronized delivery records.

## Dependencies

None. Real diagnostics require an already-authorized identity; ordinary
verification does not. No administrator or system-policy access is authorized.

## Risks

Native protocol/tool selectors vary by version. Endpoint traffic and successful
response delivery must not be confused with optional ACP tool-update events.
Missing authoritative denial is not proof of enforcement or of bypass.

## Parallelism

`parallel-safe` relative to completed WO-01/02 only. Execution is sequential;
never overlap another native attempt or shared-file implementation.

## Inputs

- [Native authority and diagnostics](../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md).
- [Original evidence ledger](../copilot-profile-plugins/compatibility-evidence.md).
- Existing `internal/agent/acpdbg`, selected managed runtime commands,
  `/acp-debug`, `/tdd`, and backend guidance.

## Results

Delivered seven build-tagged files and eight native attempts on
`darwin/arm64` (macOS 26.7.1): three initial attempts plus one corrective
follow-up per version. Every native invocation exited 1, ran one top-level
test, and had no subtests. No full enterprise-policy qualification is claimed.

- Exact package/binary identity and both roots were observed for each version.
  Initial harness defects included unsupported version-specific flags, missing
  SSE `headers`, and the wrong tool-selector namespace.
- The command-boundary regression
  `TestNativeACPCommandUsesCopilotServerToolIdentifier` failed on the old
  `mcp__server__tool` selector, then passed with Copilot's actual `server-tool`
  selector. Consent remains restricted to one exact annotated read-only fixture
  operation with empty input.
- The corrected 1.0.104 run observed one real call and one successful result
  from each plugin, sibling, native-config, and ACP-injected sentinel.
  Its disable case had zero target traffic and one allowed sibling call,
  but no authoritative denial was captured.
- The corrected 1.0.88 run observed all four endpoint calls. Its then-current
  assertion still required ACP tool updates and stopped before the paired
  disable case. The later evidence-correlation correction was not natively
  rerun for this version; do not invent that result.
- Managed policy was not run. Official macOS sources are forced MDM,
  a fixed root-owned system file, or enterprise-account settings. A tempdir
  is not that source. No such configuration was created or changed.
- Final tagged offline checks passed: 10 top-level tests and 20 subtests,
  zero skips. Ordinary provider tests passed; the earlier verbose run reported
  22 passing top-level tests, 14 passing subtests, and one skip. The final
  non-verbose run supplied no fresh count.
- The three focused race tests passed. Native-test discovery succeeded.
  Changed-file lint over the exact seven-file list reported zero issues;
  whitespace checks passed.
- All eight runtime roots were removed; exact subprocesses/listeners closed.
  Raw captures remain private. No production files, real policy/auth stores,
  live Kandev instance, staging, commits, or pushes were changed by this worker.

The user's later decision makes these diagnostic limitations non-blocking for
WO-04. It does not turn them into passing policy tests. WO-04 subsequently
delivered supported native process preparation.

PR review also corrected first-seen permission-request handling in the offline
diagnostic driver. The [WO-04 remediation results](task-04-qualified-process-loading.md#pr-review-remediation)
record its regression and passing tagged race check. Native acceptance results
above remain historical and were not rerun for this correction.
