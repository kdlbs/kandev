---
id: "02-opencode-producer"
title: "Prove continuous events in the OpenCode producer"
status: blocked
wave: 2
depends_on:
  - "01-launch-observations"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-008
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-007.3
  - AC-AGENTS-BACKGROUND-WORK-008.1
  - AC-AGENTS-BACKGROUND-WORK-008.2
  - AC-AGENTS-BACKGROUND-WORK-008.3
  - AC-AGENTS-BACKGROUND-WORK-008.4
  - AC-AGENTS-BACKGROUND-WORK-008.5
system_design:
  - ../../specs/agents/system-design/background-work-opencode.md
---

# Task 02: Prove continuous events in the OpenCode producer

## Summary

Own the producer prerequisite in the anomalyco/opencode repository. Keep the
session event subscription alive after the initiating prompt returns and prove
the negotiated wire contract with a packaged CLI.

## In scope

- Begin from the recorded 2.2.0 commit, then reconcile the target upstream head.
  Record the exact base, runtime family, and current dependency state.
- Implement the proposed capability negotiation and one attached-session
  subscriber. Preserve the old behavior for clients without opt-in.
- Publish native shell outcomes, complete/incomplete attach snapshots, explicit
  automatic foreground boundaries, and cycle-tagged standard ACP content.
- Preserve prompt response ownership, permissions, cancellation, child updates,
  and close/reset/EOF cleanup. Avoid duplicate prompt/session subscription output.
- Admit opted-in user input only against its expected idle foreground revision.
  Prove atomic rejection before acceptance when automatic execution wins the race.
  Distinguish this definite rejection from a response lost after acceptance.
- Use stable session, shell, native message, cycle, and event identities.
  Include actual exit status and final output when the native producer knows them.
- Record sanitized raw wire evidence, packaged binary digest/version, test
  results, and unsupported output/recovery/control capabilities in protocol-evidence.md.
- Reconcile the proposed metadata spelling with accepted upstream behavior.
  Update this design and fixture together before Task 03 begins.

## Out of scope

Changing Kandev's transport to HTTP, reading its live user's databases,
publishing an upstream PR/release without authorization, and native job controls.
No stable package version is invented in advance.

## Acceptance

1. After a prompt response returns, a real packaged OpenCode CLI emits an
   identified job result and one complete automatic response over the same ACP
   connection. The fixture does not require a paid model.
2. Conditional foreground admission, opt-in, old clients, incomplete snapshots,
   duplicate events, permissions, and close/reset/EOF have wire-level evidence.
3. The exact producer artifact and accepted contract are recorded. Task 03
   remains gated until that artifact is available to a disposable conformance run.

## Verification

Set OPENCODE_SOURCE to the dedicated source checkout, not the Kandev worktree.
The two background-continuity test files are proposed upstream additions.

~~~bash
test -n "$OPENCODE_SOURCE"
(cd "$OPENCODE_SOURCE" && bun install --frozen-lockfile)
(cd "$OPENCODE_SOURCE/packages/cli" && bun test test/acp/turn.test.ts test/acp/translate.test.ts test/acp/background-continuity.test.ts test/acp/background-continuity.subprocess.test.ts --timeout 30000)
(cd "$OPENCODE_SOURCE/packages/cli" && bun run typecheck)
git -C "$OPENCODE_SOURCE" diff --check
~~~

Run the subprocess suite against the packaged artifact produced by the upstream
build. Record its exact build command and digest in the evidence record.
The suite must inspect actual JSON-RPC frames, including initialize and the
session/prompt response. Fake Kandev consumer tests alone do not pass this gate.

## Files likely touched

In anomalyco/opencode, not this monorepo:

- packages/cli/src/acp/turn.ts, sessions.ts, translate.ts, capabilities.ts, and tool.ts.
- Proposed packages/cli/src/acp/background-continuity.ts.
- Existing packages/cli/test/acp/turn.test.ts and translate.test.ts.
- Proposed packages/cli/test/acp/background-continuity.test.ts and
  background-continuity.subprocess.test.ts.
- packages/schema/src/session-event.ts only if native evidence lacks required identity.

In Kandev: this package's protocol-evidence.md and design contract if upstream
accepts a different wire shape. Do not change runtime defaults to claim release.

## Dependencies

Task 01 establishes the Kandev baseline. The upstream repository and a packaged
producer artifact are external dependencies. This work order remains pending
until implementation begins; external unavailability must not be reported as done.

## Risks

The release may not adopt this exact metadata proposal. Preserve the user
outcomes and revise the contract explicitly. Retaining a subscriber without
ownership and teardown tests can duplicate output or leak permission handlers.

## Parallelism

sequential

## Inputs

- [Producer design](../../specs/agents/system-design/background-work-opencode.md#producer-prerequisite).
- [Pinned sources and evidence gate](protocol-evidence.md).
- Upstream packages/cli/test/acp/turn.test.ts and native session.shell events.

## Results

Blocked on 2026-10-27. The anomalyco/opencode checkout and packaged artifact are
not available in this workspace, so no upstream implementation, release, or raw
wire experiment was performed. Recording this as blocked (not done) keeps the
producer gate honest: the consumer tasks below cannot claim continuous delivery
from deterministic consumer fixtures.

Unblocking requires a dedicated `OPENCODE_SOURCE` checkout, the accepted wire
contract, a packaged CLI digest, and the sanitized full-wire fixture described
in `protocol-evidence.md`.
