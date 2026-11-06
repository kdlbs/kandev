---
title: Native review candidate timeout recovery
system: agents
status: active
created: 2026-10-15
owners: [backend]
---

# Native review candidate timeout recovery

### REQ-AGENTS-NATIVE-REVIEW-TIMEOUT-001: Recover a failed reviewer candidate in the same run

Reuse the shared dynamic profile resolver and utility execution loop. Preserve
candidate order, saved failure policies, the existing eight-attempt bound per prompt batch and
timeout values. Do not create another review run, retry ledger or background
worker. Abandoned-run recovery is a separate change.

#### Acceptance criteria

- **AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.1:** Profile-backed review uses one isolated
  dynamic route per run, retaining its logical/concrete identity and generation
  across batches and fallbacks. Concrete and legacy execution remain supported.
- **AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.2:** A typed child request deadline with a
  live parent follows the existing transient failure policy. Parent cancellation
  or deadline stops execution before host fallback or another candidate. Partial
  output is never replayed through a successor. Implementation status: incomplete.
  Unconfirmed request deadlines currently stop automatic fallback; fulfilling
  child-timeout recovery requires future exact child-termination proof from its
  native owner. A live parent or empty transport response is insufficient.
- **AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.3:** Candidate exhaustion records the
  existing run as failed through its existing detached failure-write context.
  A storage error is surfaced rather than reported as a terminal write success.
- **AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.4:** Actual child admission and settlement
  remain the lifecycle/host-utility callback's responsibility. Add no operating
  changes, model calls or new queue/poller. Permanent regression tests exercise
  the real routing, identity and terminal-write boundaries with synthetic
  execution and isolated stores.
