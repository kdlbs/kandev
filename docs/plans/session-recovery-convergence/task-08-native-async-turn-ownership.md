---
id: "08-native-async-turn-ownership"
title: "Preserve native async turn ownership"
status: in_progress
wave: 8
depends_on:
  - 07-native-prompt-readiness
plan: plan.md
requirements:
  - REQ-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001
acceptance_criteria:
  - AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.12
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 08: Native async turn ownership

## Evidence and scope

Retained inbox events show generation-less synthetic `async_turn_idle`
completions carrying a previously completed dispatched turn. The successor
check correctly rejected them. One completion preceded a still-active foreground
tool's terminal update. The affected session remains active and is not a repair
target for live mutation.

Preserve explicit/numbered ownership, leave unowned native event turn IDs unset,
and publish matching generation-less readiness so the existing orchestrator
snapshot identifies the async turn before a queued successor starts. Track
foreground tools in the ACP idle finalizer; wait for their terminal updates,
preserve recognized background monitors, and cancel on real prompt boundaries.
No UI changes, old prompt replay, historical rewrites, or successor-guard bypass.

## Validation and delivery

Red tests reproduce both premature completion and stale turn injection. Verify
foreground terminal statuses, multiple tools, background/nested controls, prompt
epoch cancellation, durable payload identity, readiness identity and successor
protection. Use focused resource-capped Go tests and race checks. All browser
execution stays in CI. Commit, push, open a follow-up PR promptly, address CI and
reviews, and lead through merge in the existing session.

## Results

Focused adapter and lifecycle race checks pass after reproducing both failures.
Documentation catalog and specification lint pass. Orchestrator generation-less readiness/successor integration also passes under
the race detector. Commit checks, CI and review remain pending.
Public session controls, commands, and recovery instructions are unchanged, so
no public documentation update or screenshots are required.

Review reproduced two additional boundaries: native replies without provider
message IDs merged across completed human prompts, and child updates without
repeated parent metadata became foreground blockers. Add journal-retained native
message IDs, retain a distinct legacy message scope without dispatch ownership,
and reuse remembered tool lineage. A real SQLite projection/reopen/replay test
checks separate native transcript rows and turn associations. The documentation
coverage failure was a missing requirement declaration in design frontmatter.

The review regressions passed under the race detector after correction, including
real SQLite projection, reopen and idempotent replay for legacy and journaled
message identities. The documentation coverage evaluator now reports covered.

Later review also covered events with neither a provider message ID nor any
preceding dispatch identity: use their durable sequence as the conservative
replay-stable scope. Missing foreground terminals intentionally keep completion
blocked; adapter shutdown does not fabricate a terminal or release queued work.

CI exposed surviving-turn completion and buffered-output ownership gaps. Desktop,
phone, and SSH restart checks remained running; the overload trace persisted all
2,000 reasoning chunks but split them at chunk 1,907 when the pending flag cleared.
Focused regressions reproduce the missing restored turn and lost enqueue-time
generation. Preserve the recovered generation-to-turn binding and stamp ACP
activity before journaling, so draining output neither changes message scope nor
revives a completed prompt. Local E2E remains prohibited; CI validates browser
and executor restart behavior.
