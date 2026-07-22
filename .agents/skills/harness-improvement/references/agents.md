# Agents And Subagents

Use this when creating a role with separate instructions, model choice, tool access, permission mode, background execution, or isolation.

## Role Design

Create a subagent only when at least one is true:

- It needs a different model or reasoning effort.
- It needs a smaller or safer tool surface.
- It runs in parallel with sibling work.
- It has a different output contract than the parent agent.
- It should not inherit the parent conversation's full context.

Do not create separate agents for aliases of the same responsibility. Merge or route instead.

## Kandev Local Agent Shape

Current repo agent files use Claude-style Markdown frontmatter under:

```text
.agents/agents/<name>.md
```

Common fields in this repo:

```yaml
---
name: implementer
description: Implement one assigned Kandev task from a spec-driven plan using TDD.
tools: Bash, Read, Edit, Write, Grep, Glob
model: sonnet
permissionMode: acceptEdits
skills: tdd, e2e, mobile-parity, debug, context-engineering
---
```

Guidelines:

- `name`: stable lowercase identifier, hyphen-separated unless the target platform requires another style.
- `description`: when the parent should delegate to it.
- `tools`: smallest sufficient set.
- `model`: use the current role tier. Claude uses `opus` for
  architecture/security/deep review and `sonnet` for other workers. Codex uses
  GPT-5.6 Sol, Terra, and Luna for frontier, balanced, and cheap roles. Cursor
  uses Grok 4.5 for frontier review and Composer 2.5 for normal workers.
  OpenCode intentionally inherits the configured provider in this repository.
- `effort`: reasoning depth for Claude source agents; map to Codex
  `model_reasoning_effort` where supported. Kandev role mapping:
  - `high`: `architect`, `code-review`, `security-auditor`
  - `medium`: `implementer`, `test-engineer`, `qa`, `simplify`
  - `low`: `verify`, `pr-poller`
- `permissionMode`: read-only/review roles should not accept edits; implementers can accept edits.
- `skills`: preload only the minimum domain skills needed.
- Body: input required, workflow, output contract, stop conditions, and "Do not spawn subagents" unless nested delegation is intentional.

## Platform Choice

Delegation is always native to the active harness:

- Claude Code uses the `Agent` tool.
- Codex uses `spawn_agent`, `send_message`/`followup_task`, and `wait_agent`.
- Cursor uses its native custom-subagent invocation.
- OpenCode uses the `Task` tool.

Never use Kandev MCP `spawn_session_kandev`, `create_task_kandev`, or
`message_task_kandev` as a subagent mechanism. Use them only when the user
explicitly requests Kandev platform task/session management.

When the user asks to create or update a subagent/agent, update every existing project-local platform mirror by default:

- `.agents/agents/<name>.md` - Kandev source agent shape used by current repo workflows.
- `.codex/agents/<name>.toml` - Codex custom agent mirror, when present or when Codex support is requested.
- `.claude/agents/<name>.md` - Claude Code subagent mirror, when present or when Claude support is requested.
- `.cursor/agents/<name>.md` - Cursor custom subagent mirror.
- `.opencode/agents/<name>.md` or project config - OpenCode agent mirror, when present or when OpenCode support is requested.

Only update one platform when the user explicitly says "only Codex", "only Claude", or equivalent.

Load platform references as needed:

- For Claude-native subagents, read `platforms/claude.md`.
- For Codex custom agents, read `platforms/codex.md`.
- For Cursor custom subagents, read `platforms/cursor.md`.
- For OpenCode agents, read `platforms/opencode.md`.

## Cross-Platform Sync

Use `.agents/agents/<name>.md` as the semantic source unless the user identifies another platform file as authoritative. Preserve the same role, trigger, tool safety, permission intent, model tier, workflow, output contract, and stop conditions across mirrors while translating syntax to each platform.

Field mapping:

- Role name: `.agents`/Claude frontmatter `name`, Codex TOML `name`, Cursor
  frontmatter `name`, OpenCode filename or config key.
- Trigger: `.agents`/Claude `description`, Codex TOML `description`, OpenCode `description`.
- Instructions: Markdown body, Codex `developer_instructions`, OpenCode body or `prompt`.
- Model: translate model tier, not literal provider names when the platform requires provider-qualified IDs.
- Tools/permissions: translate the safety intent, not field names.
- Skills: preload only supported platform skill references; otherwise mention required skills in instructions.

If a platform mirror does not exist, create it only when repo convention or the user's request says that platform should be supported. Do not create empty platform folders just for symmetry.

## Validation

Validate only mirrors and search roots that already exist:

```bash
mirror_paths=(
  ".agents/agents/<name>.md"
  ".codex/agents/<name>.toml"
  ".claude/agents/<name>.md"
  ".cursor/agents/<name>.md"
  ".opencode/agents/<name>.md"
)
existing_mirrors=()
for path in "${mirror_paths[@]}"; do
  [[ -e "$path" ]] && existing_mirrors+=("$path")
done
((${#existing_mirrors[@]})) && git diff --check -- "${existing_mirrors[@]}"

search_roots=(.agents/agents .codex/agents .claude/agents .cursor/agents .opencode/agents .agents/skills AGENTS.md CLAUDE.md)
existing_roots=()
for path in "${search_roots[@]}"; do
  [[ -e "$path" ]] && existing_roots+=("$path")
done
((${#existing_roots[@]})) && rg -n "<agent-name>" "${existing_roots[@]}"
```

Check that orchestration skills reference new agents only where they should be used.
