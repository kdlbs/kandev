# Built-in MCP caller authentication: provider compatibility evidence

Date: 2026-11-12. Host: local Linux workstation.

## Method

`acpdbg mcp-probe` injects a streamable-HTTP MCP sentinel into ACP
`session/new` with a random per-probe `Authorization: Bearer` header. It
records whether each request that reaches the sentinel carries that header
(`header_observed`, `header_missing`). It does not record the value. Probes ran
against already-installed binaries through `--exec`, so they did not install
packages into the running Kandev's managed runtime.

```bash
make -C apps/backend build-acpdbg
apps/backend/bin/acpdbg mcp-probe --timeout 60s --out /tmp/acpdbg --exec "<agent command>"
```

Only rows marked "verified" count as supported. Shared implementation and unit
tests do not prove that a provider sends the header.

## Results

| Provider | Version | Probe command | Result | Evidence |
| --- | --- | --- | --- | --- |
| Claude ACP (`claude-agent-acp`) | 0.106.0 | `--exec "claude-agent-acp"` | Verified | `initialize` and `tools/list` reached the sentinel; `header_observed: true`, `header_missing: false` |
| Claude ACP (`@agentclientprotocol/claude-agent-acp`) | 0.100.0 | Manual `make dev` session with enforcement on | Verified | `ask_user_question_kandev` created the question in the owning task and returned the answer; after a backend restart, the session resumed its conversation on a new agentctl instance that reused the same port, and the agent saw no MCP rejection. Requests without a credential or with an invented one returned 401. No Kandev tool call ran after the resume |
| OpenCode ACP | 1.19.5 | `--exec "opencode acp"` | Verified | `initialize` and `tools/list` reached the sentinel; `header_observed: true`, `header_missing: false` |
| Custom ACP (`atomica-code`) | 1.0.105 | `--exec "atomica-code --acp"` | Verified | `initialize` and `tools/list` reached the sentinel; `header_observed: true`, `header_missing: false`. The plan named 1.0.102; the installed version is now 1.0.105 |
| Copilot ACP | Not installed | `--exec "copilot --acp"` | Not verified | `/usr/bin/copilot` is an installer shim ("GitHub Copilot CLI was not found"); `initialize` failed with EOF |
| Codex ACP | Not installed locally | None | Not verified | Package not cached; not installed to avoid changing the managed runtime |
| Codex app-server | n/a | Unit test | Not verified live | `codexConfig` emits `http_headers` for credentialed entries |
| Gemini, Auggie, Grok, Amp, Muse, Devin | Not installed | None | Not verified | Not installed on this host |
| Cursor ACP and TUI | Not installed | None | Not verified | `cursor-agent` absent. Re-approval after each fingerprint change is unconfirmed |
| Pi | Not installed | None | Not verified | Project-file content covered by `TestProjectMCPDeliveryPreparedOnlyAfterSuccessfulWrite` |
| Mock agent (ACP SSE, TUI HTTP) | In tree | Go tests and web E2E | Verified | `cmd/mock-agent` header tests and the clarification E2E specs with the E2E profile, where enforcement is on |

## Consequence for promotion

Claude ACP, OpenCode ACP, and `atomica-code` send the configured header on
streamable HTTP. Every other row stays "not verified". Promotion of
`features.builtinMcpCallerAuth` to `prod: "true"` needs evidence for the
providers in use on the target installation. Enabling the toggle on one
installation is safe when that installation only uses verified providers.
