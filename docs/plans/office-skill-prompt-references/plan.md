---
created: 2026-10-17
status: implemented
requirements:
  - REQ-OFFICE-SKILL-PROMPT-REFERENCES-001
system_design:
  - ../../specs/office/system-design/skill-prompt-references.md
legacy_specs: []
---

# Implementation plan: Expand saved-prompt references in Office skills

## Outcome

Delivered `SKILL.md` files keep the authored `@name` references and append the
Kanban saved-prompt expansion block with a skill-specific instruction line.
Stored skill content does not change.

## Reproduced failure

| Failure | Cause | Repair |
| --- | --- | --- |
| An `@name` in Office skill content reaches the agent as literal text | `officeskills.SkillReaderAdapter` copies `sk.Content` verbatim, and no part of the skill deploy path calls the prompts expander | Add a skill variant of the prompts service expansion, give the adapter an optional expander, and wire the prompts service into it in `wireRuntimeSkillDeployer` |

## Work orders

Execute sequentially. Delegation is not authorized.

- [x] [01: Expand saved-prompt references in delivered skills](task-01-expand-skill-prompt-references.md)

## Verification mapping

| Criteria | Required implementation evidence |
| --- | --- |
| 001.1 (header), 001.4 | New tests in `internal/prompts/service/skill_expansion_test.go`: the skill variant uses the shared marker and skill instruction; the Kanban output of `FormatPromptReferenceExpansions` is unchanged; a Kanban-header block in skill content is replaced, not duplicated; skill output is idempotent |
| 001.1, 001.3, 001.6 | New tests in `internal/office/skills/runtime_adapter_prompt_test.go`: a known reference appends one block and keeps the `@name`; no reference leaves content identical; a nil expander leaves content identical; the inner reader's skill is not mutated |
| 001.2, 001.4 | The same test file, backed by a real `promptservice.Service` over a stub repository: an edited saved prompt is reflected on the next read, and a forged block is replaced |
| 001.3 (lookup failure) | Repository error leaves content identical and returns no error |
| 001.5 | New test in `internal/agent/runtime/lifecycle/skill`, or the adapter test, that deploys an expanded skill with frontmatter. `SKILL.md` starts with `---` and contains the block. All executors share the adapter, so one delivery path is enough |

The numeric prefixes refer to `AC-OFFICE-SKILL-PROMPT-REFERENCES-*`.

## End-to-end evidence

No Playwright test. This change has no new UI. The observable result is the
`SKILL.md` file content, which the backend tests assert directly.

## Risks

- `AppendReferenceExpansionsToTrustedContext` merges blocks by exact header
  prefix. Skill blocks never enter that path, so the merge stays Kanban-only.
- Authored skill text that contains a `CONTEXT PROMPTS:` system block is
  removed, because the prompts service treats it as browser input. This is
  unlikely in practice and consistent with Kanban.
- Kanban-launched skills also gain expansion. This is intended (see system
  design).
