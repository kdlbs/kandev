---
id: "03-shell-outcome-delivery"
title: "Deliver owned shell outcomes through the existing journal"
status: completed
wave: 3
depends_on:
  - "02-private-plugin-host"
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-PLUGIN-003
  - REQ-AGENTS-BACKGROUND-WORK-007
acceptance_criteria:
  - AC-AGENTS-OPENCODE-PLUGIN-003.1
  - AC-AGENTS-OPENCODE-PLUGIN-003.2
  - AC-AGENTS-OPENCODE-PLUGIN-003.3
  - AC-AGENTS-OPENCODE-PLUGIN-003.4
  - AC-AGENTS-OPENCODE-PLUGIN-003.5
  - AC-AGENTS-OPENCODE-PLUGIN-003.6
  - AC-AGENTS-OPENCODE-PLUGIN-003.7
  - AC-AGENTS-BACKGROUND-WORK-007.6
  - AC-AGENTS-BACKGROUND-WORK-007.7
  - AC-AGENTS-BACKGROUND-WORK-007.8
  - AC-AGENTS-BACKGROUND-WORK-007.9
system_design:
  - ../../specs/agents/system-design/opencode-kandev-plugin.md
  - ../../specs/agents/system-design/background-work-opencode.md
---

# Task 03: Deliver owned shell outcomes through the existing journal

## Summary

Connect native shell facts to immutable ACP launch bindings and the existing
durable workload update path. Complete the first behavior without adding
conversation events, admission authority, output, or controls.

## Scope and owned files

- Own `src/behaviors/shell-outcomes.ts`, its registry entry, and
  `test/shell-outcomes.test.mjs` in the plugin package.
- Own typed shell receiver/configuration code in `server/opencodeplugin`.
- Own proposed `opencode_plugin_bindings.go` and `opencode_plugin_outcomes.go`
  beside the existing ACP `opencode_background.go` and sibling tests.
- Own the optional adapter-wrapper seam in `server/adapter` for claim snapshots
  and normalized plugin observations. Do not widen mandatory adapter methods.
- Own `server/process/opencode_plugin_delivery.go`, sibling delivery tests,
  and the narrow shared journal changes required for persistent event identity.
- Own focused regressions in `internal/orchestrator` and
  `internal/task/repository/sqlite` only where existing projection needs repair.

Reuse `confirmBackgroundLaunch`. Store the original payload association outside
`activeToolCalls` before prompt settlement removes that tool. Record original
native generation, source call, prompt generation, and delivery attribution.

The plugin caches created/exited facts but sends only claimed shell results.
Do not wait for unclaimed foreground shells in the delivery queue. Cover a
terminal that arrives before its claim and capacity exhaustion at that boundary.

Use actual native exit evidence and the design's state mapping. Preserve missing
exit codes and times. Keep inspection-only capabilities and unavailable output.

Add one process-manager ingress that uses the existing ordered writer and
commit-before-publish barrier. Persist stable event identity before returning
success. A lost acknowledgement must not duplicate projection. Existing claims
remain until commit; duplicate receipt lookup remains possible after removal.

## Acceptance

1. The real receiver settles the original claimed workload once for success,
   nonzero exit, timeout, killed, and outcome-unavailable native terminal facts.
2. Early, duplicate, reordered, stale-generation, cross-session, and lost-ack
   cases preserve ownership and terminal monotonicity. Real journal failure
   produces no acceptance acknowledgement or fabricated outcome.
3. An outcome after foreground completion or during a successor prompt emits
   only a workload observation. It neither completes nor reattributes that prompt.

## Verification

Add `opencode_plugin_outcomes_test.go`,
`process/opencode_plugin_delivery_test.go`, and
`orchestrator/opencode_plugin_background_test.go`. Add a repository regression
beside existing background-work reducer tests when needed.

Required named cases include `TestOpenCodePluginClaims`,
`TestOpenCodePluginDurableAck`, `TestOpenCodePluginLostAck`,
`TestOpenCodePluginOutcomeKeepsOrigin`,
`TestOpenCodePluginOutcomeDuringSuccessorPrompt`, and
`TestOpenCodePluginUnknownFallback`.

From `apps/backend/`:

```bash
go test -tags fts5 -count=1 ./internal/agentctl/server/opencodeplugin ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/journal -run 'Test(OpenCode|BehaviorRegistry|Plugin)'
go test -tags fts5 -count=1 ./internal/orchestrator ./internal/task/repository/sqlite -run 'TestOpenCodePlugin'
go test -race -tags fts5 -count=1 ./internal/agentctl/server/opencodeplugin ./internal/agentctl/server/process -run 'TestOpenCodePlugin(DurableAck|LostAck|OutcomeDuringSuccessorPrompt|Lease)'
```

From `apps/`:

```bash
pnpm --filter @kandev/opencode-plugin typecheck
pnpm --filter @kandev/opencode-plugin test
pnpm --filter @kandev/opencode-plugin build
pnpm --filter @kandev/opencode-plugin check:artifact
pnpm --filter @kandev/opencode-plugin test:conformance
```

The packaged conformance test now proves the actual module's early and
post-prompt terminal behavior. Fake callbacks do not satisfy that evidence.

## Exclusions and dependencies

Depend on the proved artifact and private transport. Automatic launch activation
remains off until Task 04. Add no SQL workload table, client store, output path,
root completion, native PID action, or historical backfill.

## Risks

`forwardUpdates` normally derives submission identity for adapter events.
Explicitly preserve the launch's captured identity for native outcomes.
Do not inspect the current active submission as a fallback. Old callback
generations and conflicting duplicate payloads must have no effects.

## Results

Completed on 2026-10-29.

- The plugin retains `shell.created` facts and joins `shell.exited` by native
  shell identity. Only an exact ACP-confirmed claim can enter the outbox.
- The ACP adapter retains immutable launch bindings outside `activeToolCalls`,
  captures native session generation, source call, prompt generation, and title,
  and clears claims on every session transition.
- `process.Manager` publishes claim snapshots with original submission
  attribution and commits observations through the existing ordered journal and
  `background_work_updated` path.
- The journal now stores a stable event identity beside the committed event, so
  a retry after a lost acknowledgement cannot project twice. Conflicting payloads
  under one identity fail closed.
- Successful acceptance retires the exact shell claim while durable receipts
  preserve lost-ack retries. A native-session admission guard is held across the
  journal write, and the plugin ingress is cancellable without acknowledging a
  cancelled write.
- Tests cover success, nonzero exit, timeout, killed, missing exit, early fact,
  claim removal, duplicate/lost acknowledgement, conflicting duplicate,
  original-turn attribution during a successor prompt, stale-generation
  revocation, close during commit, more than 256 completed jobs, and no root
  completion.
