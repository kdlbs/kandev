---
status: done
---

# Task 04: Document and verify the repair

## Objective

Make the operator-facing behavior discoverable and run the repository checks
needed to hand off the repair confidently.

## Scope

- `docs/public/plugins.md`
- Any generated/translation updates required by task 03.
- Targeted and final verification commands.

## Requirements

- Document that Error is recoverable with Enable, where failure diagnostics are
  shown, and that successful recovery clears them.
- Document that failed Enable refreshes the authoritative diagnostic, while
  stored diagnostics redact credential-like values and home paths.
- Document that ring-buffer overflow warnings are aggregated per plugin rather
  than emitted for every dropped event.
- Keep public terminology consistent with the existing plugin docs.

## Acceptance

- Public docs validation passes.
- Backend targeted tests, web tests, type checking, linting, and plugin E2E checks pass,
  including the concurrent runtime barrier and phone overflow assertions.
- Backend formatting and required repository gates pass, or any environment
  limitation is recorded explicitly in the handoff.

## Dependencies

Tasks 01–03.
