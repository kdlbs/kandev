---
status: current
system: agents
requirements:
  - REQ-AGENTS-MINIMAX-001
  - REQ-AGENTS-MINIMAX-002
  - REQ-AGENTS-MINIMAX-003
---

# MiniMax Code system design

## Boundaries and evidence

Extend the built-in agent registry with `MiniMaxACP`, using the existing native
ACP scaffold. No new transport, persistence schema or provider fallback is
needed. MiniMax Code owns its subscription provider and catalog; Kandev's
hostutility/profile capability caches project it rather than maintaining a
second static model catalog.

Verified against official [upstream](https://github.com/MiniMax-AI/minimax-code)
commit `e77c7630363b7bec2762e852b8ad6af475bca002` and published npm 0.5.10:
`mcode acp`, `mcode login [--region cn|global] [--no-browser]`,
`mcode --model provider/model[#variant]`, `--session <id>` and `--continue`.
Initialization advertises loadSession, HTTP/SSE MCP and text-only prompts.
Unauthenticated session/new returns ACP auth_required. Actual live subscription
access is unavailable in this task environment.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-AGENTS-MINIMAX-001 | Identity and setup |
| REQ-AGENTS-MINIMAX-002 | Protocol and models |
| REQ-AGENTS-MINIMAX-003 | Credentials and isolation |

## Identity and setup

`agents/minimax_acp.go` supplies native command metadata, version-check discovery,
install recipe, logos and LoginAgent. `registry.LoadDefaults` registers it and the fixed utility probe command allowlist accepts `mcode`, with a
unique display order. Install pins the verified official package and includes
optional SQLite dependencies and required install scripts. The login PTY uses
`mcode login --no-browser`; localized instructions explain native CN/default
and Global commands and the existing Ctrl+C-to-shell recovery path. The shared
AgentLoginDialog renders those instructions on desktop and phone, uses the
existing quick terminal surface and refreshes native models before rescanning
the agent cards after Done. Long command previews truncate within the dialog.

## Protocol and models

Structured commands and inference use `mcode acp`; model selection is applied
after session/new through the shared sessionmodel typed-config path. Upstream
`control-state.ts` advertises `m:<encoded-provider>:<encoded-model>:u` or
`m:<encoded-provider>:<encoded-model>:v:<encoded-variant>`. Treat these as opaque
IDs on ACP. For terminal mode only, decode valid IDs to
`provider/model[#variant]`. The native empty variant maps to `#none-thinking`. Invalid IDs remain explicit invalid CLI inputs,
never a request to silently use the default model.

Native catalog entries include M3, M3.1 Flash Preview, M2.7 highspeed and M2.7.
The current native defaults are 512000 context / 128000 output for both M3
entries, and 200000 / 128000 for M2.7. Native optional 1000000 context is not
Kandev's default; ACP does not advertise media input. Discovery remains
account-dependent and authoritative. Unknown/unavailable models are not seeded
as selectable static fallbacks.

MCP arrives through ACP session/new/load. The existing permission handler owns
session/request_permission; no yolo flag is injected. Standard cancel and load
reuse transport/acp and utility. Shared ACP regression tests exercise model selection, permission requests,
HTTP/SSE MCP, cancellation and resume. Published 0.5.10, with an isolated
BYOK endpoint, completed text prompts, MCP initialize/tools/list/tools/call,
active cancellation and subsequent session load. Native permission-mode
selection succeeded, but its policy did not emit request_permission for the
probe tool calls; permission-handler evidence comes from the shared tests.
These probes do not establish live subscription access.

## Credentials and isolation

`~/.minimax` holds config and sessions; public data-directory overrides are
MINIMAX_DATA_DIR and MAVIS_DATA_DIR, with internal runtime profile/data overrides.
Strip those path/profile overrides at every existing Runtime.StripEnv boundary.
Persist executor `.minimax` at `/root/.minimax` for containers; never bind the
real host home. Native region selection and credentials remain CLI-owned.

OAuth FileStore keys combine service/region and a hash of the absolute auth
home, so copying credentials into another home does not establish usable auth.
RemoteAuth is nil: remote/container users sign in inside their executor instead
of receiving host tokens or arbitrary BYOK provider fallback. Login removes the
same path/profile overrides on POSIX hosts so it targets the same default home.
User config may contain keys; no config bundle is copied automatically.

## Presentation and recovery

Reuse current agent catalog, install terminal, auth terminal, profile editor and
model picker. No new layout or touch interactions. Phones use existing settings
navigation and the full-screen quick terminal plus the existing model picker. Login help
uses i18next in all supported locales. Native protocol errors remain visible
through existing capability statuses; installation does not infer authentication.

## Validation

Registry and agent tests cover install availability, commands, logos, isolation,
permissions and passthrough model/resume. ACP fixture tests cover published
configuration shapes. Desktop and mobile rendered tests cover login guidance
and encoded model profile persistence. Public setup docs report supported
commands, credential ownership and live-test limits.

## Implementation plans

- [Native MiniMax delivery](../../../plans/native-minimax/plan.md)
