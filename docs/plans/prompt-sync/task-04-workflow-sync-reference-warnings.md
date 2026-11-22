---
id: "04-workflow-sync-reference-warnings"
title: "Workflow Sync unresolved-reference warnings"
status: done
wave: 2
depends_on: ["02-prompt-ownership"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-006
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-006.1
  - AC-INTEGRATIONS-PROMPT-SYNC-006.2
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 04: Workflow Sync unresolved-reference warnings

## Summary

After applying its files, Workflow Sync reports `@name` references in parsed
workflow and step prompts that do not resolve to a saved prompt.

## In scope

- A narrow `PromptReferenceChecker` interface in `workflowsync`, satisfied by
  the prompt service through `UnresolvedPromptReferences`, and an optional
  setter wired in `backendapp`.
- Grouping by (file, workflow, missing name), with referencing steps listed
  in position order, in the warning format from the system design.
- A checker error adds one warning and does not fail the run. A nil checker
  skips the check.

## Out of scope

Prompt Sync, UI changes, and checking manual workflows.

## Acceptance

- Unresolved references in a step prompt and in a workflow prompt produce
  one grouped warning per name.
- Fully resolved references, and definitions with no references, produce
  output and status identical to before, including `unchanged`.
- An invalid (frozen) file is not checked.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/workflowsync -count=1
go test -trimpath -race ./internal/workflowsync -run 'Reference|Apply' -count=1
```

## Likely files

- `apps/backend/internal/workflowsync/{service_host_binding.go,service.go,reference_warnings.go}` and tests
- `apps/backend/internal/backendapp/services.go`

## Risks

- `@` tokens that are not prompt references (for example `@claude`) will
  warn. This is accepted by AC-INTEGRATIONS-PROMPT-SYNC-006.1; keep the
  message precise so users can tell why.

## Results

- `go test -trimpath ./internal/workflowsync -count=1`: pass.
- `go test -trimpath -race ./internal/workflowsync -run 'Reference|Apply' -count=1`: pass.
- `pnpm e2e:run --host -- tests/workflow/workflow-sync-gitlab.spec.ts`: 6 passed.
