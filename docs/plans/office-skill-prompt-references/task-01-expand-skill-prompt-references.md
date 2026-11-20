---
id: "01-expand-skill-prompt-references"
title: "Expand saved-prompt references in delivered skills"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SKILL-PROMPT-REFERENCES-001
acceptance_criteria:
  - AC-OFFICE-SKILL-PROMPT-REFERENCES-001.1
  - AC-OFFICE-SKILL-PROMPT-REFERENCES-001.2
  - AC-OFFICE-SKILL-PROMPT-REFERENCES-001.3
  - AC-OFFICE-SKILL-PROMPT-REFERENCES-001.4
  - AC-OFFICE-SKILL-PROMPT-REFERENCES-001.5
  - AC-OFFICE-SKILL-PROMPT-REFERENCES-001.6
system_design:
  - ../../specs/office/system-design/skill-prompt-references.md
---

# Task 01: Expand saved-prompt references in delivered skills

## Summary

Pass Office skill content through the prompts service expander when the
runtime deployer reads it. Keep stored content unchanged.

## In scope

- `internal/prompts/service/expansion.go`: add
  `AppendSkillReferenceExpansions` and a skill instruction constant. Factor the
  shared clean-and-resolve steps out of `AppendReferenceExpansionsWithContext`.
  Make the block formatter take a header internally. Keep
  `FormatPromptReferenceExpansions` and the Kanban output byte-identical.
- `internal/office/skills/runtime_adapter.go`: add a local
  `promptReferenceExpander` interface and an optional expander on
  `SkillReaderAdapter`. Set it with a `WithPromptReferenceExpander` method or a
  constructor option, following nearby style. Expand `Content` only. Do not
  mutate the skill that the inner reader returns.
- `internal/backendapp/main.go` `wireRuntimeSkillDeployer`: accept the prompts
  service and pass it to the adapter. Update the call site at the
  `wireRuntimeSkillDeployer(...)` invocation to pass `services.Prompts`.
- Tests listed in the plan's verification mapping.

## Out of scope

Support files, `AGENTS.md` instructions, the older
`internal/office/skills/materialization.go` path (it does not feed the runtime
deployer), and UI changes.

## Acceptance

1. With a real `promptservice.Service`, adapter output for content that contains
   `@shared-rules` keeps the authored text and contains exactly one expansion
   block with the skill instruction line and the current saved content. The
   existing `expansion_test.go` passes unchanged. Content with no reference, a nil
   expander, or a failed lookup is byte-identical to the input.
2. A deploy of an expanded skill that has YAML frontmatter writes a `SKILL.md`
   that starts with the frontmatter and contains the block.
3. The backend builds with the wiring change, and existing adapter, deployer,
   and backendapp tests pass.

## Verification

Run from `apps/backend`:

```bash
go test -trimpath -race ./internal/prompts/service ./internal/office/skills ./internal/agent/runtime/lifecycle/skill -count=1
go build -trimpath ./internal/backendapp/... ./cmd/kandev
```

From the repository root:

```bash
bash scripts/lint-go-changed apps/backend/internal/prompts/service/expansion.go apps/backend/internal/prompts/service/skill_expansion_test.go apps/backend/internal/office/skills/runtime_adapter.go apps/backend/internal/office/skills/runtime_adapter_prompt_test.go apps/backend/internal/backendapp/main.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Likely files

- `apps/backend/internal/prompts/service/expansion.go`
- `apps/backend/internal/prompts/service/skill_expansion_test.go` (new)
- `apps/backend/internal/office/skills/runtime_adapter.go`
- `apps/backend/internal/office/skills/runtime_adapter_prompt_test.go` (new)
- `apps/backend/internal/backendapp/main.go`

## Dependencies and risks

- `internal/office/skills` tests may import `internal/prompts/service` and
  `internal/prompts/models` as test-only dependencies. Production code depends
  only on the local interface.
