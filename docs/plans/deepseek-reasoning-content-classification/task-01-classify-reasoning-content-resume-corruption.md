---
id: "01-classify-reasoning-content-resume-corruption"
title: "Classify DeepSeek reasoning_content 400 as resume corruption"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.3
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.5
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.6
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.6
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
---

# Task 01: Classify DeepSeek reasoning_content 400 as Resume Corruption

## Summary

Map the DeepSeek thinking-mode `reasoning_content ... must be passed back`
400 to the existing `resume_corrupted` code with fresh-session remediation,
and make `IsResumeCorrupted` recognize the same signature.

## Acceptance

- The exact single-line signature classifies as high-confidence
  `resume_corrupted` under rule `deepseek.reasoning_content.nonreplayable.v1`,
  with no automatic retry or fallback.
- `IsResumeCorrupted` returns true for the signature and false for empty text,
  unrelated `reasoning_content` prose, a gap beyond 80 characters, and phrases
  on separate lines. A gap of exactly 80 characters is accepted.
- Redaction cannot shorten an oversized raw gap into a matching signature.
  The rule checks both the sanitized excerpt and the raw diagnostic.
- Agent explanations and quoted diagnostics do not match. A standalone
  canonical DeepSeek diagnostic and an anchored error line remain accepted.
- The existing `resume_corrupted` user-action policy vetoes automatic retry
  and unclassified fallback. The diagnostic does not prove the failure's cause
  or guarantee recovery from an adapter-level omission in a fresh session.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/routingerr -run "ReasoningContent|ResumeCorrupted"
```

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go`
- `apps/backend/internal/agent/runtime/routingerr/reasoning_content_nonreplayable_test.go`

## Risks

A broad `reasoning_content` match could turn agent prose into a veto. The rule
requires `must be passed back` within 80 characters on the same line.
The raw match guard preserves that bound when sanitization masks a long token.
Canonical diagnostic or error-line framing prevents the two phrases in agent
prose from becoming a resume-corruption verdict.

## Results

- [PR #4494](https://github.com/kdlbs/kandev/pull/4494) implements the bounded
  signature and reuses the existing recovery policy.
- Original focused test passes are author-reported. Review retains the flags
  assertions and adds exactly-80 and newline cases. These predicate tests and
  their negative mutations passed the controlled verification; no production
  RED is claimed for that coverage-only change.
- A real assertion against the exact sanitizer/runtime matcher exposed an
  oversized raw gap matching after redaction. The minimal new-rule raw guard
  addresses it; real `Classify` tests cover redactable 80/81 and newline gaps.
  Hosted CI verifies the full classifier and its unchanged recovery policy.
- Codex's prose-collision finding was reproduced against the exact predicate
  and addressed with diagnostic framing plus predicate/`Classify` regressions.
- Independent focused and hosted verification receipts belong to the review
  task's sibling evidence directory and are not inferred from author reports.
