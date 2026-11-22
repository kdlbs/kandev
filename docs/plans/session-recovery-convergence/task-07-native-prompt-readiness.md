---
id: "07-native-prompt-readiness"
title: "Reconcile native delivery before prompt readiness"
status: completed
wave: 7
depends_on:
  - 06-terminal-delivery-recovery
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 07: Native prompt readiness

## Evidence and correction

An idle native successor published boot readiness with unresolved journal work
and no recovery fence. Its first new user instruction created the fence and was
persisted blocked, without a durable submission. Explicit Resume later resolved
the same history and a distinct second instruction succeeded. Neither a timed
self-recovery nor loss of native history is established by that sequence.

Reuse guarded loaded-native reconciliation before boot publishes idle state or
drains queued work. If evidence is insufficient, publish the existing recovery
fence before first send. Repeat the same gate in pending and V1 protocol
admission, and re-read capability after reconciliation. Preserve owner and
inventory guards, automation ownership, known outcomes, and no-replay behavior.
No new UI controls or copy are needed; desktop and phone use the existing banner
and retain the composer draft while explicit Resume runs.

Before: resumed conversation -> enabled send -> blocked user message -> banner.
After: resumed conversation -> guarded reconciliation -> enabled first send;
or resumed conversation -> recovery banner with retained draft -> explicit
Resume -> user sends the draft once. Phone uses the same existing touch controls.

## Work and validation

Update orchestrator delivery protocol admission and boot handling. Add SQLite
service regressions for absent fences, both protocol variants, successful proof,
missing owner proof, and unchanged old instructions. Extend native-resume browser
fixtures on desktop and phone to enter through ordinary resume without a seeded
fence, show recovery before first send, and preserve a draft across Resume.

Run resource-capped focused Go tests and scoped lint; typecheck changed browser
fixtures. Run all browser execution in CI only. Open a follow-up PR promptly,
address its CI/reviews, and lead through merge. No live operational mutations,
new tasks/sessions, delegation, or local E2E.

## Results

Focused Go regressions failed on the original missing-fence boot and admission
paths, then passed with the correction and refusal controls (orchestrator 3.821s).
Specification/catalog and public-document validation passed. Scoped E2E
typechecking found no remaining changed-file errors, but shared existing E2E
helpers have duplicate-method and global-declaration errors. Browser execution
and final delivery are pending CI; no local E2E was run.

CI exposed provider-continuation restart and mobile runtime-replacement paths
whose native boot succeeded while delivery retirement was still pending. The
early delivery-gate return skipped attempt-owned provider recovery bookkeeping,
leaving the old provider-failure card after delivery metadata was restored.
A focused regression reproduces that stale error. Boot now completes provider
recovery bookkeeping while retaining the independent delivery fence and skipping
queue drain until delivery is ready. CI owns the existing browser regressions.

PR #4468 merged at 2026-10-14T00:38:18Z as 2f501583ff55b9f64047ae29c99b39c33ee5d2cb. All 57 executed checks passed, including the previously failing browser regressions; 11 checks skipped. No unresolved review threads remained.
