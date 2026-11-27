---
created: 2026-10-15
status: implemented
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
legacy_specs: []
---

# Implementation Plan: DeepSeek reasoning_content Resume Corruption

## Overview

Repair [issue #4493](https://github.com/kdlbs/kandev/issues/4493). DeepSeek
thinking mode reports missing `reasoning_content` with a 400. The catalogue has no rule for that
signature, so it falls through to `phase.poststart.unknown` and
`IsResumeCorrupted` reports false. The signature alone does not establish
saved-history corruption or guarantee that starting fresh repairs an adapter
that omits the required field.

Add one exact catalogue rule beside the Anthropic thinking-blocks rule and
extend `IsResumeCorrupted` to the same signature. Existing resume-corruption
handling then applies: no automatic retry or fallback, and the recovery
message steers toward a fresh session.

## Scope

### In scope

- The `deepseek.reasoning_content.nonreplayable.v1` runtime-environment rule
  mapping to `resume_corrupted` with fresh-session remediation.
- `IsResumeCorrupted` recognition of the same single-line signature.
- Canonical bare diagnostic or error-line framing, excluding agent prose that
  quotes or explains the two phrases.
- Raw diagnostic matching before redaction can shorten the signature's gap.
- Unit coverage for code, class, policy flags, and false-positive boundaries.

### Out of scope

- Automatic fallback to a later candidate after this failure. Resume corruption
  remains a veto for unclassified fallback.
- Repairing or reconstructing persisted `reasoning_content`.
- Changes to orchestration, recovery UI, or policy values.

## Work orders

- [x] [Task 01: Classify DeepSeek reasoning_content 400 as resume corruption](task-01-classify-reasoning-content-resume-corruption.md)

## Delivery and verification

- Delivered by [PR #4494](https://github.com/kdlbs/kandev/pull/4494).
- The author reported focused `ReasoningContent|ResumeCorrupted` tests and
  package `go vet` passing on the original source. These are author evidence,
  not independent verification of the review candidate.
- Review adds exactly-80-character acceptance and newline rejection coverage,
  preserving the existing recovery flags and negative prose cases. These
  predicate tests passed, and 79-character/newline mutations failed their
  assertions; that coverage-only change does not claim a production RED.
- A controlled exact-source `Sanitize` and `matchRuntimeRules` proof exposed
  an 81-character raw gap becoming a match after redaction. The new rule now
  also requires the raw signature. Real `Classify` regressions cover redactable
  80- and 81-character gaps and newline rejection. Full classifier and policy
  verification runs in hosted CI; the controlled proof is not a package pass.
- Codex review found a prose collision. An exact-predicate assertion reproduced
  it, and the matcher now requires a standalone canonical diagnostic or an
  anchored error line. Predicate and real `Classify` tests reject explanations
  and quoted diagnostics while preserving the captured error and raw bounds.
- Independent candidate verification and hosted CI results are retained in
  the review task's sibling evidence directory, outside this work order.
- No public docs change is needed: no API, configuration, recovery action, or
  user-facing terminology is added. Existing fresh-session guidance is reused.
