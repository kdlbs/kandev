---
id: "01-continuation-evidence"
title: "Establish continuation evidence and rollout contract"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 01: Establish continuation evidence and rollout contract

## Summary

Prove the native restore/read contract and carry a conservative typed safety
snapshot through terminal failures. Add the complete default-off feature-toggle
contract and configuration transport before any consumer can authorize work.

## In scope

- Load scoped guidance and TDD. Read the design's Compatibility and evidence
  and Rollout and restart sections.
- Add isolated native Cursor restore coverage; record installed CLI version,
  negotiation, sanitized result, and supported read frame in
  `compatibility-evidence.md` beside the plan. Use tiny disposable read-only
  fixtures and an owned conversation; never switch host networks or use the
  user's task. The probe confirms history/identity, not exactly-once effects.
- Proposed typed support/safety fields, bounded tool ledger, immutable
  pre-sweep snapshot, terminal-event conversion and remote JSON propagation.
- All-off profile/config/registry/frontend flag identity, restart metadata, and
  typed managed-startup and adapter propagation with absent-state fail-closed behavior.
- Positive completed read and output-only fixtures; negatives for pending,
  conflicting, unknown, write/execute/MCP/subagent/background/permission,
  overflow, generation replacement, and missing remote fields.

## Out of scope

Automatic dispatch, weakening `EffectObserved`, generic normalizer/title-based
classification, search-tool support, new public capability negotiation, and flag
promotion.

## Acceptance

- A real isolated native restore retains provider conversation identity and
  saved request/history and can finish the interrupted read request with safe
  re-reading when needed. Record version and evidence; a skipped native
  check or unsupported shape does not satisfy completion.
- Current terminal evidence is bounded, positive, immutable before sweeping,
  and preserved through runtime hops. Missing, stale, or unsafe evidence remains
  ineligible without changing existing replay decisions.
- The toggle is registered once and off in prod/dev/e2e; disabled configuration
  does not collect continuation evidence, and old remote helpers imply no support.

## Verification

New named tests are implementation targets. The native test must run, not skip,
when its opt-in environment variable is set. It must use a temporary HOME/workspace
with only the minimum existing credential access, never copy/log credentials.

```bash
cursor-agent --version
(cd apps/backend && KANDEV_TEST_CURSOR_NATIVE_CONTINUATION=1 go test -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestCursorNativeConversationRestore$' -count=1 -v)
(cd apps/backend && KANDEV_TEST_CURSOR_NATIVE_CONTINUATION=1 go test -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestCursorNative(AbruptDisconnectRestore|SessionResume)$' -count=1 -v)
(cd apps/backend && go test -tags fts5 -race ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/types/streams ./internal/agent/runtime/lifecycle ./internal/orchestrator/watcher -run 'Test(CursorContinuationEvidence|ContinuationSafetySnapshot|ContinuationConfig)' -count=1)
(cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
```

In a fresh worktree, install once from `apps/` with
`pnpm install --frozen-lockfile` before the frontend check. Capture no raw prompts
or account information in the committed compatibility evidence.

## Files likely touched

- `profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/{registry,config}.go` and contract tests
- `apps/backend/internal/agentctl/types/types.go` and `streams/agent.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/{dialect_cursor,adapter,adapter_prompt,adapter_updates,adapter_tools}.go`
- New `dialect_cursor_continuation_test.go`, `cursor_native_continuation_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/{event_types,events,types}.go`
- Agentctl configure construction, watcher event conversion, associated tests
- `apps/web/lib/state/slices/features/types.ts`
- `docs/plans/provider-interruption-continuation/compatibility-evidence.md`

Brace expressions above are file groups, not new directories. Trace actual
configuration constructors/converters before editing.

## Dependencies

None. Implementation must not claim provider support before the native proof.

## Risks

Cursor restore semantics or coarse tool kinds may not establish the required
contract. Preserve absence of support and report the unresolved evidence rather
than treating it as permission for replay.

## Parallelism

`sequential`

## Inputs

Paired design and requirements; current Cursor dialect, `Adapter.LoadSession`,
`CursorACP.Runtime`, replay-safety tests, and runtime-feature-flags skill.

## Results

Completed on 2026-10-02. Cursor Agent `2026.10.01-e373342` advertises
`loadSession=true`; direct `session/resume` is unavailable (`-32601`). Isolated
cancellation and abrupt-process-loss probes restore the same ID with
`session/load`, but omit interrupted read results and assistant output. A
continuation instruction permitting safe re-reading finishes the original read
request in the same ID. The requirements/design use that narrower contract.
See [compatibility evidence](compatibility-evidence.md).

The bounded adapter ledger, immutable terminal snapshot, lifecycle/watcher JSON
propagation, startup/instance/adapter flag propagation, and default-off registry,
profile, and frontend contracts are implemented. Focused race tests and scoped
lint passed. Native read-continuation tests have a positive completion assertion.
The adapter prefers advertised resume and filters historical load notifications.
Task 02 is implemented and its affected desktop integration checks pass.

## Execution todos

- [x] Run the isolated native compatibility probe and record which interrupted state Cursor restores.
- [x] Add bounded continuation evidence and preserve its immutable snapshot through agent events.
- [x] Add the default-off runtime toggle and test disabled behavior and remote omission.
- [x] Run the exact Task 01 checks and record results before starting Task 02.

The strengthened native read continuation assertion passed for deliberate cancellation and abrupt process loss. Typed bounded safety evidence, terminal lifecycle and watcher JSON propagation, old-helper omission, managed startup/instance/adapter flag inheritance, and all-off registry/profile/frontend contracts passed focused tests with race detection. Scoped lint reported zero issues. Native `session/resume` returned `-32601`; production restoration already prefers an advertised resume method and suppresses `session/load` history replay. Task 02 is complete; the flag remains off.

PR review remediation fences foreign-session permissions, retires async finalizers on attested terminal RPC errors, and verifies complete snapshot serialization. Focused ACP and lifecycle race tests pass. The live Cursor probes have stronger assertions but were not rerun during fixup.
