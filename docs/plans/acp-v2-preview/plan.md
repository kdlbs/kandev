---
created: 2026-11-23
status: implemented
requirements:
  - REQ-AGENTS-ACP-V2-001
  - REQ-AGENTS-ACP-V2-002
  - REQ-AGENTS-ACP-V2-003
  - REQ-AGENTS-ACP-V2-004
system_design:
  - ../../specs/agents/system-design/acp-v2-preview.md
legacy_specs: []
---

# Implementation plan for ACP v2 Preview

The pilot lets local Kandev sessions negotiate ACP v2 with qualified Claude
and Codex bridges. It starts off in every shipped profile. The Kandev work
orders extend the debugger, integrate the session lifecycle, and expose the flag.

[Requirements](../../specs/agents/requirements/acp-v2-preview.md)

## Provider evidence

On November 23 the stock `acpdbg` sent v1 and an overlay build sent v2. Each
probe used a fresh process and an isolated home. A v2 pass required a valid
v2 `info` envelope in addition to the selected version number.

| Agent and tested release | Result when asked for v2 | Pilot behavior |
| --- | --- | --- |
| Codex ACP 2.8.0 and 2.9.0 | V2 handshake, prompt, completion, and replay | Qualify these versions |
| Claude ACP 0.109.0 and 0.109.1 | V2 handshake, prompt, completion, and replay | Qualify these versions |
| GitHub Copilot CLI 1.0.114 | Selected v1 | Use v1 |
| Google Gemini CLI 0.70.0 | Selected v1 | Use v1 |
| JetBrains Junie 4100.11.0 | Selected v1 | Outside the pilot |
| Cursor 2026.11.22, Goose 1.60.2 | Selected v1 | Use v1 |
| OpenCode 1.19.6 and 2.3.2 | Selected v1 | Use v1 |
| Pi ACP 0.0.35, Qwen Code 0.28.4, Grok Build 1.0.92 | Selected v1 | Use v1 |
| Google Antigravity ACP 1.5.2 | Returned 2 with a v1 envelope | Use v1 |

The Kandev pins at the time of the probe were Codex 2.8.0 and Claude 0.109.0.
Codex required auth for `session/new` in the empty-home probe. For the turns,
both bridges used synthetic credentials and a loopback model fixture. All four
builds completed one v2 turn, replayed it over v2, and replayed saved history in
both v1-to-v2 and v2-to-v1 directions. Codex emitted `running` before its prompt
acknowledgement. Claude emitted it after the acknowledgement.

These initial wire tests used real adapters and fixture model output. The
verification results below cover Kandev integration. Live model credentials
remain outside the fixture tests. JetBrains Air is an ACP client. The Preview
announcement describes work by JetBrains and Zed with both adapters.
[ACP v2 Preview announcement](https://agentclientprotocol.com/announcements/acp-v2-preview)

The task workspace's `acp-v2-lab` directory contains the overlay builder,
isolated probe scripts, JSON summaries, and raw JSONL. From the task workspace,
rebuild the stock debugger with `make -C kdlbs-kandev/apps/backend build-acpdbg`.
Then run:

```bash
python3 acp-v2-lab/build_overlay.py
python3 acp-v2-lab/probe_matrix.py
python3 acp-v2-lab/local_turns.py
```

The SDK generator emitted v2 files when pointed at the Preview schema. A
compile check of the generated types found duplicate state-union fields and
references to v1 `LegacyModels`. Use the fork's transport and a small typed v2
client for the pilot.

## Scope

The pilot covers structured chat on the local process executor with the two
qualified ACP bridges and an existing account or API key. Permitted model
and mode changes, MCP attachment, approval, cancellation, replay, and rollback
are in scope. Later work can qualify other clients, provider gateways, remote
and container executors, or new background controls.

## ASCII UI preview

UI-01. Settings > System > Feature Toggles. The existing row component has
this content on desktop and phone. Its switch starts off. A saved change uses
the existing restart banner. The row and switch use the existing layout and
keyboard order. The same state applies on phone.

```text
ACP v2 preview                       [ Off ]
Use ACP v2 for supported local agents.
```

This row covers `AC-AGENTS-ACP-V2-001.1`. The label and description are
illustrative. The available state follows the registry.

## Work orders

The SDK work package is `docs/plans/acp-v2-preview/plan.md` in the
`acp-go-sdk` repository. The SDK revision is a prerequisite for order 02.

1. [Debugger wire version support](task-01-sdk-and-debugger.md) (done)
2. [Agentctl session lifecycle](task-02-agentctl-session-lifecycle.md) (done)
3. [Runtime flag and pilot qualification](task-03-runtime-flag-qualification.md) (done)

## Verification

| Acceptance criteria | Planned evidence |
| --- | --- |
| 001.1 | Backend flag precedence test and Feature Toggles E2E on desktop and phone |
| 001.2–001.3 | Process-manager fixture for disabled launch, v2 negotiation, v1 fallback, and startup failures |
| 002.1–002.3 | Agentctl turn fixture for both acknowledgement orders, transcript updates, approval, cancel, failure, and disconnect |
| 002.4 | Agentctl MCP sentinel and debugger `mcp-probe` |
| 003.1–003.3 | Agentctl replay, flag rollback, same-ID restore, and retained-process dispatch test |
| 004.1–004.2 | SDK fixtures, CLI captures, and sanitized startup diagnostics |

The product smoke test uses the ordinary local task chat entry point. Add a
scripted v2 peer to the existing mock-agent harness. Verify that chat displays
the answer before it settles, supports cancellation, and restores after restart.
Run the two real adapters with the loopback fixture after the flag is wired.

## Verification results

The updated debugger and agentctl passed the turn and replay checks for the
four qualifying releases with the loopback model fixture. Kandev uses the
published SDK revision recorded in work order 02. The browser checks covered
default ACP v1 prompt, cancel, and restart behavior, preview chat, and restore
of the same native conversation over v1 after rollback.

## Risks

- ACP v2 is in Preview. Adapter versions can use different schema revisions.
  Qualify each managed release with its installed dependencies.
- A false v2 response or a failed restore can hide a provider mismatch.
  Confirm full handshake shape and preserve the saved conversation ID.
- Prompt acceptance and completion arrive independently. Record and verify both
  before relying on Kandev's current turn and retry rules.
