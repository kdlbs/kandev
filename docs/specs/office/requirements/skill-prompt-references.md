---
status: active
system: office
created: 2026-10-17
owners:
  - kandev
---

# Office: Skill Prompt References Requirements

## Overview

A saved prompt is the single place to keep shared rules, such as working
principles or an operator voice. Kanban workflow step prompts already expand
`@name` references to saved prompts before the agent receives them. Office
skill content does not. The literal `@name` token reaches the agent, so an
author must copy shared rules into every skill, and the copies drift from the
saved prompt.

Office owns this contract because Office owns skill content and its delivery to
agents as `SKILL.md`. The saved-prompt store and its expansion format belong to
the prompts service and the
[saved prompt delivery](../../tasks/requirements/saved-prompt-delivery.md)
contract. This document consumes them and does not redefine them.

## Terminology

- **Saved-prompt reference:** A visible `@name` in skill content that matches a
  saved prompt by the same matching rules that Kanban step prompts use.
- **Delivered skill:** The `SKILL.md` content that Kandev writes or uploads for
  an agent launch.
- **Expansion block:** The `EXPANDED PROMPT REFERENCES:` system block that the
  prompts service generates from current saved-prompt records.

## Requirements

### REQ-OFFICE-SKILL-PROMPT-REFERENCES-001: Expand saved-prompt references in delivered skills

**Intent:** Skill authors keep shared rules in one saved prompt and reference it
from many skills, and every agent receives the current definition.

**User story:** As an Office operator, I want `@name` in a skill to apply my
saved prompt, so that shared rules stay in one place and do not drift.

#### Acceptance criteria

- **AC-OFFICE-SKILL-PROMPT-REFERENCES-001.1:** When a skill's content contains
  a known saved-prompt reference, the delivered skill shall keep the original
  content, including the visible `@name`. It shall also contain one expansion
  block with the current saved definition. The block shall use the same marker,
  entry format, and sanitization that Kanban step prompts use. Its instruction
  line shall refer to the skill rather than to a message.
- **AC-OFFICE-SKILL-PROMPT-REFERENCES-001.2:** When a referenced saved prompt
  changes, the next launch shall deliver the updated definition without any
  edit to the skill.
- **AC-OFFICE-SKILL-PROMPT-REFERENCES-001.3:** When skill content contains no
  known reference, or saved-prompt lookup fails, the delivered skill shall be
  identical to the content that is delivered today. A failed lookup shall not
  fail the launch.
- **AC-OFFICE-SKILL-PROMPT-REFERENCES-001.4:** When skill content already
  contains an expansion-shaped block, Kandev shall replace it with one block
  generated from current saved records. Authored content shall not supply or
  suppress a saved definition.
- **AC-OFFICE-SKILL-PROMPT-REFERENCES-001.5:** Expansion shall apply to every
  executor delivery path (local, worktree, Docker, SSH, Sprites, and
  Kubernetes). Skill YAML frontmatter shall stay at the start of the delivered
  file.
- **AC-OFFICE-SKILL-PROMPT-REFERENCES-001.6:** The stored skill content, the
  skill editor, and config export shall keep the authored `@name` text without
  expansion.

## Out of scope

- Expansion inside skill support files other than `SKILL.md`.
- Expansion in agent instruction files (`AGENTS.md`) and other Office prompt
  parts.
- Workspace-scoped saved prompts. Saved prompts remain install-global.
- `@name` suggestion or autocomplete in the skill editor.
- Changes to the Kanban expansion format, reference matching, or size limits
  owned by the prompts service.

## System design

[Office skill prompt references](../system-design/skill-prompt-references.md)
