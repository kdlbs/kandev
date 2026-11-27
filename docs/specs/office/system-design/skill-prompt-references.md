---
status: current
system: office
requirements:
  - REQ-OFFICE-SKILL-PROMPT-REFERENCES-001
created: 2026-10-17
owners:
  - kandev
---

# Office: Skill Prompt References System Design

## Purpose and boundaries

Office skills reach agents as `SKILL.md` files through the runtime skill
deployer (`internal/agent/runtime/lifecycle/skill`). The deployer reads skills
through `officeskills.SkillReaderAdapter`, which maps an Office skill row to a
`runtimeskill.Skill`. That adapter is the single boundary between stored skill
content and every executor delivery path. This design expands saved-prompt
references there.

This design uses the prompts service expansion contract
(`AppendReferenceExpansions`) and the
[saved prompt delivery design](../../tasks/system-design/saved-prompt-delivery.md).
It adds a skill variant of the block's instruction line. It does not change
reference matching, the block marker, entry format, sanitization, limits, or
the Kanban instruction line.

Kanban launches use the same deployer and adapter. Skills that a Kanban launch
delivers therefore get the same expansion. This is intended: one skill must
deliver the same content regardless of launch mode.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-OFFICE-SKILL-PROMPT-REFERENCES-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Security](#security) |

## Components and responsibilities

- `internal/prompts/service` adds `AppendSkillReferenceExpansions(ctx,
  content string, log *zap.Logger) string`. It shares the cleaning and
  resolution steps of `AppendReferenceExpansionsWithContext`. Its block header
  is the existing `EXPANDED PROMPT REFERENCES:` marker followed by a skill
  instruction: "This skill references saved prompts by @name. Apply these
  definitions where the skill references them." An unexported formatter takes
  the header. `FormatPromptReferenceExpansions` keeps its signature and output.
  The marker is unchanged, so forged-block removal covers both variants.
- `internal/office/skills.SkillReaderAdapter` gains an optional prompt
  reference expander. When the expander is set, the adapter passes
  `sk.Content` through it before it builds the `runtimeskill.Skill`. When the
  expander is nil, content is unchanged.
- The expander is a narrow interface local to `internal/office/skills`:
  `AppendSkillReferenceExpansions(ctx, content string, log *zap.Logger) string`.
  `*promptservice.Service` satisfies it. `internal/office/skills` does not
  import `internal/prompts`.
- `internal/backendapp.wireRuntimeSkillDeployer` passes the existing prompts
  service (`services.Prompts`) to the adapter.
- `internal/agent/runtime/lifecycle/skill` is unchanged. `renderSkillMarkdown`
  still adds frontmatter only when the content does not start with it. The
  expansion block is appended after the authored content, so frontmatter stays
  first.

## Data and contracts

No schema, API, or event changes. The stored `office_skills.content` keeps the
authored text. The expansion exists only in the `runtimeskill.Skill.Content`
value for one deploy.

Delivered content has this form:

```text
<authored skill content, unchanged, including @name>

<kandev-system>EXPANDED PROMPT REFERENCES: This skill references saved prompts by @name. Apply these definitions where the skill references them.

### @name
<current saved prompt content>
</kandev-system>
```

## Control flow

1. A launch calls `Deployer.Deploy`, which builds the skill manifest.
2. For each skill slug, the deployer calls `SkillReaderAdapter.GetSkillFromConfig`.
3. The adapter reads the Office skill, then calls the expander with its content.
4. The prompts service removes expansion-shaped and browser prompt-definition
   blocks, resolves references against saved-prompt storage, and appends one
   block with the skill instruction line.
5. The deployer writes or uploads the resulting content as `SKILL.md` through
   the executor-specific delivery path.

Expansion runs on every deploy, so a saved-prompt edit takes effect at the next
launch.

## Failure and recovery

The prompts service logs lookup failures and returns the cleaned content. The
adapter never returns an error because of expansion. A launch with a failed
lookup delivers the skill without an expansion block.

## Persistence

None. Expansion is computed per deploy and never stored.

## Security

Saved prompts are install-global, and any skill can reference any saved prompt.
This matches Kanban step prompts. The prompts service strips forged expansion
blocks from authored content before it resolves references. It also sanitizes
system tags in the expanded names and content. A skill author therefore cannot
inject a definition under a saved prompt's name.

## Observability

Lookup failures produce the existing `failed to resolve prompt references`
warning from the prompts service. No new metrics.
