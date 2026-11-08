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
exists in this PR. This work adds focused regression coverage and updates the
requirement and design records to state its contract.

The signature stays scoped to `opencode-acp` and to a diagnostic line that
starts with one of `AI_APICallError:`, `Error:`, `Internal error:`, or
`API Error:`. It accepts case and whitespace variations. It does not match an
unknown label, quoted prose, or another provider. Existing structured status
and earlier account-limit rules retain precedence. The diagnostic gives no
reset time, so classification does not infer one.

## Implementation

- `apps/backend/internal/agent/runtime/routingerr/rules.go` contains the
  existing provider-scoped rule and its position after earlier account-limit
  rules.
- `apps/backend/internal/agent/runtime/routingerr/opencode_free_usage_limit_test.go`
  covers accepted labels in prompt-send and streaming phases, case and
  whitespace variations, later diagnostic lines, prose and provider
  boundaries, priority, recovery flags, and the absence of a reset hint.
- The requirement and system design define this signature in criterion
  `AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.12`.

## Acceptance

1. Every accepted label classifies the diagnostic as a high-confidence hard
   quota limit in prompt-send and streaming phases, without an inferred reset
   hint.
2. Quoted prose, unknown labels, incomplete phrases, and other providers do
   not match. Existing structured status and earlier account-limit rules keep
   their classifications.

## Verification

- `cd apps/backend && go test ./internal/agent/runtime/routingerr -run 'TestClassify_OpenCodePrefixedFreeUsageLimit'`
- `cd apps/backend && go test ./internal/agent/runtime/routingerr`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`

The focused test passed (`0.007s`) and the full classifier package passed
(`0.074s`). `python3 scripts/list-docs.py validate` validated 436 decisions and
1,931 specifications. `python3 scripts/lint-spec-files.py --all` passed. The
PR documentation coverage evaluator accepted this work order and its linked
requirement, design, and plan.
