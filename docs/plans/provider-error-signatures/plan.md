---
created: 2026-10-08
status: implemented
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
legacy_specs: []
---

# Implementation Plan: Classify provider limits by signature

## Overview

Several provider rules matched topic words anywhere in the text:
`claude.stderr.rate.v1` and the OpenCode, Copilot, and Amp rate rules used
`(?i)rate.?limit`, the OpenCode and Amp quota rules used `(?i)quota`, and the
Claude subscription rule used `(?i)subscription`. Agent prose that quoted
another session's error, for example a coordinator reporting that a child hit
"Rate limit exceeded", was classified as the reporting agent's own provider
limit and suspended a healthy account.

One work order replaces the topic-word patterns with provider error
signatures. See [task 01](task-01-match-provider-signatures.md).

## Scope

### In scope

- The provider rule patterns in `apps/backend/internal/agent/runtime/routingerr/rules.go`.
- Table tests for prose that must not classify and signatures that must.

### Out of scope

- Rule order, confidence, and classification flags.
- Provider-neutral and runtime-environment rules.
- How classified failures are attributed or acted on by dynamic routing.

## Verification

- `go test ./internal/agent/runtime/routingerr/`
- Package tests of the classifier consumers (`dynamic`, `registry`,
  `lifecycle`, `agentctl/server/adapter/transport/acp`, `office/scheduler`,
  `office/service`, `utility/handlers`, `orchestrator`) compared with the same
  packages on the base revision.
- `golangci-lint run ./internal/agent/runtime/routingerr/...`
- `make -C apps/backend build`
