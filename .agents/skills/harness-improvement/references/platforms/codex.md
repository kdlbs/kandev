# Codex Platform Reference

Verified against OpenAI Codex docs on 2026-06-14.

Sources:
- https://developers.openai.com/codex/skills
- https://developers.openai.com/codex/subagents

## Skills

Codex skills are directories with a required `SKILL.md` and optional `scripts/`, `references/`, `assets/`, and `agents/openai.yaml`.

Required `SKILL.md` fields:

```yaml
---
name: skill-name
description: Explain exactly when this skill should and should not trigger.
---
```

Codex can invoke skills explicitly or implicitly based on `description`. Keep descriptions front-loaded and specific because large skill lists may truncate descriptions.

Project-local Kandev skills use `.agents/skills/<name>/SKILL.md`.

## Custom Agents / Subagents

Launch workers with Codex-native `spawn_agent`, coordinate with
`send_message`/`followup_task`, and collect them with `wait_agent`. Do not use
Kandev MCP task/session APIs for Codex delegation.

Codex custom agents are standalone TOML files:

```text
.codex/agents/<name>.toml
```

Required fields:

```toml
name = "reviewer"
description = "PR reviewer focused on correctness, security, and missing tests."
developer_instructions = """
Review code like an owner.
Lead with concrete findings and cite files.
"""
```

Useful optional fields:

```toml
model = "gpt-5.6-terra"
model_reasoning_effort = "high"
sandbox_mode = "read-only"
nickname_candidates = ["Atlas", "Delta"]

[[skills.config]]
path = ".agents/skills/code-review/SKILL.md"
enabled = true
```

Project agent settings live under `[agents]` in `.codex/config.toml`:

```toml
[agents]
max_threads = 6
max_depth = 1
job_max_runtime_seconds = 1800
```

Keep `max_depth = 1` unless recursive delegation is explicitly intended.

## Permissions And Inheritance

Codex subagents inherit the parent sandbox policy and live runtime overrides. A custom agent can set defaults such as `sandbox_mode = "read-only"`, but parent runtime overrides may still apply.

Use read-only sandbox for review, exploration, and security agents. Use write access only for implementers or fixers.

## Conversion Notes

Claude-style `.agents/agents/<name>.md` files are not Codex custom-agent TOML. To port one:

- `name` -> `name`
- `description` -> `description`
- markdown body -> `developer_instructions`
- `model` -> `model`
- Claude `effort` -> `model_reasoning_effort` where applicable
- Claude `tools`/`permissionMode` -> Codex sandbox/MCP/tool configuration where possible
