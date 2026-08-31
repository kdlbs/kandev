# Plan: OpenAI-compatible AI providers

## Overview

Add a first-class OpenAI-compatible provider primitive to the agent profile
(`provider_kind`, `provider_base_url`, `provider_api_key_secret_id`), gated on a
per-agent capability, and inject it into the live agent session, the sessionless
model probe, and the one-shot inference/utility path. Codex (`codex-acp`) is the
first supporting agent. Also correct two credential-delivery behaviors that make
a profile-declared key unreliable.

## Requirements and system design

- Requirements: [`docs/specs/agents/requirements/openai-compatible-providers.md`](../../specs/agents/requirements/openai-compatible-providers.md)
  (`REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001..004`)
- System design: [`docs/specs/agents/system-design/openai-compatible-providers.md`](../../specs/agents/system-design/openai-compatible-providers.md)
- Motivation (local working note, not committed):
  `docs/specs/openai-compatible-providers/problem-statement.md`

## Work orders

| ID | Title | Wave | Depends on | Status |
| --- | --- | --- | --- | --- |
| task-01 | Provider primitive on the agent profile | 1 | — | done |
| task-02 | ACP gateway builder (`internal/common/acpprovider`) | 2 | task-01 | done |
| task-03 | Live session injection + credential-delivery fixes | 3 | task-02 | in_progress (plumbing done; adapter test pending) |
| task-04 | Probe and inference/utility provider reach | 4 | task-03 | pending |
| task-05 | Profile editor provider section (frontend) | 3 | task-01 | pending |

Dependency order: 01 → 02 → 03 → 04. 05 runs any time after 01. 03 and 05 are
parallel-safe (backend lifecycle vs `apps/web`).

## Risks

- **codex-acp `-c` acceptance.** The design assumes `@agentclientprotocol/codex-acp`
  forwards `-c key=value` overrides to Codex the same way `mcpconfig.CodexStrategy`
  relies on for MCP. task-02 must verify this against the pinned bridge version
  (see `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`) with an
  `acp-debug` capture before the renderer shape is locked. If `-c` is not
  forwarded, fall back to a generated `config.toml` fragment written into the
  session's isolated `SessionDirTemplate` home (still never the host `$HOME`).
- **`wire_api`.** Only `responses` is currently accepted by this Codex version;
  the spec is a fixed constant, revisit if the bridge adds `chat`.
- **`mergeEnvFillMissing` precedence flip.** Scope strictly to `ReservedKeys`;
  a broad flip would change behavior for every existing profile env var. Covered
  by AC-004.3 and a regression test in task-03.
- **Partial secret resolution.** task-03 changes `resolveAgentProfileEnvVars`
  from all-or-nothing to per-entry; the required provider key is revealed on its
  own path so this does not weaken the fail-closed guarantee (AC-002.5).

## Verification strategy

- task-01/02/03/04: Go unit + integration tests alongside source
  (`make -C apps/backend test`, targeted package runs in each work order).
- task-05: `apps/web` vitest for the editor hook/validation + one desktop and
  one mobile Playwright spec for the provider section.
- End-to-end evidence for `REQ-...-002`/`003`: an integration test that starts a
  session on an `openai_compatible` Codex profile against a stub OpenAI server
  and asserts the request reached the stub with the injected bearer key.

## Out of scope

Provider catalog, per-workspace provider credentials, non-OpenAI wire protocols,
non-Codex agents, cost attribution, and the worktree offline base-branch-refresh
hard failure (tracked by the worktree owner).
