---
id: "01-qualify-native-transport"
title: "Qualify native transport and SDK dispatch"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-001
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-004
acceptance_criteria:
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.1
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.4
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.5
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-001.6
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.2
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.7
system_design:
  - ../../specs/agents/system-design/copilot-native-compaction.md
---

# Task 01: Qualify native transport and SDK dispatch

## Summary

Establish the supported native protocol and make the exact Copilot method reachable through an opt-in SDK boundary.
Import a reviewed immutable SDK version and qualify the managed CLI pin before enabling production subscriptions.

## In scope

- Read the native comment and the new design's evidence, transport, and compatibility sections.
- In the separate `kdlbs/acp-go-sdk` repository, add an explicit additional-notification allowlist connection option.
- Cover exact `github.com/copilot/sessionEvent` dispatch, default rejection/ignore behavior, existing underscore callbacks, standard routes, requests, and outbound validators.
- Merge/release the SDK prerequisite before changing this repository's replacement pin. Record the upstream PR and immutable commit.
- Inspect/probe isolated Copilot 1.0.88 and 1.0.103 using exact binaries and temporary writable `COPILOT_HOME` and working directories.
- Record the full initialize subscription, raw method, root/child attribution, manual applied completion, missing optional fields, and omission behavior.
- Establish the automatic compaction producer path with a live capture or version-pinned primary-source trace, plus a deterministic fixture.
- Record source/version and actual evidence for the selected support predicate. Update only the Copilot managed pin if required.
- Add sanitized protocol fixtures under the ACP adapter's `testdata/copilot-native/` directory. Keep raw private captures outside the repository.

## Out of scope

- Enabling native subscriptions before consumers exist, changing default RPC permissiveness, or implementing count projection.
- Assuming a missing outcome means applied, requiring a checkpoint number, or treating the manual capture as automatic coverage.
- Editing a module cache, committing a local replace path, publishing during this planning turn, or updating unrelated agent pins.

## Acceptance

1. Real JSON-RPC dispatch reaches the extension callback only for explicitly allowed vendor notifications, with all existing defaults and request/outbound restrictions intact.
2. A merged/published SDK revision resolves immutably in Kandev. The chosen CLI version has recorded attribution, manual/automatic outcome, optional-field, and omission evidence.
3. Unsupported/unknown shapes have the design's fail-closed behavior; no unverified minimum-version or complete-delivery claim remains in implementation results.

## Verification

Run from the repository root. `KANDEV_ACP_SDK_DIR` points to the separate reviewed SDK checkout.
The SDK tests below are proposed additions, not existing test claims.

```bash
(cd "${KANDEV_ACP_SDK_DIR:?Set the reviewed ACP SDK checkout}" && go test ./... -run '^TestClientAllowedExtensionNotifications' -count=1)
(cd "${KANDEV_ACP_SDK_DIR:?Set the reviewed ACP SDK checkout}" && go test ./... -run '^TestExtension' -count=1)
(cd apps/backend && go list -m -json github.com/coder/acp-go-sdk)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/acp -run 'Extension|Connection' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestCopilotNativeQualificationFixtures' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents -run '^TestCopilotACP|^TestManagedNPM' -count=1)
git diff --check
```

For live qualification, use `/acp-debug` with `--client-capabilities`, `--exec`, exact versioned binaries, and flags before any positional agent name.
The [investigation comment](https://github.com/kdlbs/kandev/issues/5089#issuecomment-6672451872) records the reproducible synthetic parent/child and `/compact` procedure.
Record actual commands, version output, sanitized fixture hashes, and evidence limitations in Results.
No live call is required during this design turn.
Unavailable qualification evidence leaves this work order pending or blocked; fixture tests alone cannot establish CLI support.

## Files likely touched

- External SDK: `extensions.go`, its connection-option definition/application, and `extensions_test.go`.
- `apps/backend/go.mod` and `apps/backend/go.sum`.
- `apps/backend/internal/agent/agents/managed_npm_runtime_versions.json` and `copilot_acp_test.go` if the managed pin changes.
- `apps/backend/internal/agentctl/server/acp/client_extension_notification_test.go` and `connection_test.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/copilot_native_qualification_test.go` (new).
- `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/copilot-native/` (new sanitized fixtures).

## Dependencies

None inside this package. The SDK merge/release is an explicit external prerequisite to importing the replacement.

## Risks

The capture used 1.0.103 while current main pins 1.0.88.
Native schema fields are optional, and notification payload/buffer limits are not durable delivery guarantees.
Do not broaden other protocol boundaries to route this one method.

## Parallelism

`sequential`

## Inputs

- [Provider requirements](../../specs/agents/requirements/copilot-native-compaction.md), criteria in frontmatter.
- [Provider design](../../specs/agents/system-design/copilot-native-compaction.md), qualification and transport sections.
- [Decision](../../decisions/2026-11-05-copilot-native-compaction-evidence.md).
- Current pinned `extensions.go`, `adapter_helpers.go`, and `clientCapabilitiesForAgent`.
- `/acp-debug`, scoped backend/agentctl guidance, and the external SDK repository's own instructions.

## Results

Done.

- SDK prerequisite merged to `kdlbs/acp-go-sdk` main at `ec6d622b199996ceb8f44544129f7895d88e7f38`. `WithAllowedExtensionNotifications` accepts exact additional inbound notification names for client connections; underscore defaults, request routing, and outbound validation are unchanged.
- Kandev resolves the immutable pseudo-version `v0.13.6-0.20261106000654-ec6d622b1999` through the existing `replace` directive.
- CLI qualification used the published Linux x64 packages and the generated Copilot SDK schemas pinned to the matching CLI snapshot:
  - `1.0.88` schema (`51a0394ecca0`): no `compactionAttemptId`, no `outcome`.
  - `1.0.103` schema (`f2b720c8ec36`): optional `compactionAttemptId`, optional `outcome`, explicit required `success`, and triggers including `threshold` for automatic compaction.
- Managed pin updated to `@github/copilot` `1.0.103`. No other agent pin changed.
- Sanitized fixtures cover manual and threshold completions, a child completion, failure, omitted data, root usage, child usage, and a start frame.
- `TestCopilotNativeQualificationFixtures`, `TestCopilotNativeCompletionAcceptance`, and `TestClientAllowedExtensionNotifications` pass.

Limitations: no live Copilot account call was made in this environment. Automatic compaction is qualified from the pinned schema and shares the same completion rule; it was not re-captured live.
