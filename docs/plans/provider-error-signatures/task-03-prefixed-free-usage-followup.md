---
id: "03-prefixed-free-usage-followup"
title: "Cover prefixed OpenCode free-usage diagnostics"
status: done
wave: 3
depends_on: ["02-provider-catalogue-followup"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
acceptance_criteria:
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.12
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
---

# Task 03: Cover prefixed OpenCode free-usage diagnostics

## Outcome and boundary

Record and test the established error-label prefixes for OpenCode's
`free usage limit reached` quota diagnostic. The classifier rule already
exists in this PR. This work adds focused regression coverage for the rule,
its ACP sanitization path, and assistant diagnostic-candidate marker. It also
updates the requirement and design records to state the contract.

The signature stays scoped to `opencode-acp` and to a diagnostic line that
starts with one of `AI_APICallError:`, `Error:`, `Internal error:`, or
`API Error:` outside a fenced code example. It accepts case and flexible
horizontal whitespace, but a label or phrase cannot cross a line boundary.
The sanitized ACP message retains diagnostic line breaks after credentials
and identifiers are redacted. The rule does not match an unknown label, quoted
prose, split-line signatures, or another provider. Existing structured status
and earlier account-limit rules retain precedence. The diagnostic gives no
reset time, so classification does not infer one.

## Implementation

- `apps/backend/internal/agent/runtime/routingerr/rules.go` contains the
  existing provider-scoped rule and its position after earlier account-limit
  rules.
- `apps/backend/internal/agent/runtime/routingerr/opencode_free_usage_limit_test.go`
  covers accepted labels in prompt-send and streaming phases, case and
  horizontal-whitespace variations, split-line rejection, fenced-example
  rejection, later diagnostic lines, prose and provider boundaries, priority,
  recovery flags, and the absence of a reset hint.
- `apps/backend/internal/agentctl/types/streams/provider_error.go` preserves
  sanitized diagnostic line boundaries. The ACP transport test proves that a
  later prefixed line remains classifiable without exposing credentials.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/provider_diagnostic_marker_test.go`
  proves fenced assistant examples do not set the diagnostic-candidate marker,
  while a diagnostic after the closed fence still sets it.
- The requirement and system design define this signature in criterion
  `AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.12`.

## Acceptance

1. Every accepted label classifies the diagnostic as a high-confidence hard
   quota limit in prompt-send and streaming phases, without an inferred reset
   hint.
2. Fenced examples, quoted prose, unknown labels, split-line and incomplete
   phrases, and other providers do not match. A valid diagnostic after a
   closed fence still matches, and existing structured status and earlier
   account-limit rules keep their classifications.

## Verification

- `cd apps/backend && go test ./internal/agent/runtime/routingerr -run 'TestClassify_OpenCode(PrefixedFreeUsageLimit|FreeUsageIgnoresFencedExamples)'`
- `cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'TestConvertMessageChunk_FencedFreeUsageExampleIsNotDiagnosticCandidate|TestProviderErrorFromErrorPreservesPrefixedFreeUsageLineBoundary'`
- `cd apps/backend && go test ./internal/agent/runtime/routingerr ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`

The initial regressions failed as expected: split-line signatures and fenced
examples were classified, and ACP sanitization removed the diagnostic line
boundary. The focused routingerr and ACP transport regression runs passed.
After the fix, the routingerr, streams, and ACP transport packages
passed with `go test ./internal/agent/runtime/routingerr
./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp`
(`0.158s`, `0.602s`, and `26.662s`). The documentation catalog validated 436
decisions and 1,931 specifications; specification lint passed; and the local
PR documentation coverage evaluator returned `covered` with no errors.
