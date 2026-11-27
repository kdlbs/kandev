---
created: 2026-09-28
status: complete
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-AUTH-003
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-003
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
legacy_specs: []
---

# Cursor MCP discovery and authenticated-source repair

## Outcome and scope

Restore automatic import of enabled native Cursor plugin MCP servers with
credential objects from eligible existing projects. Preserve disabled-server
preferences from the task's source repository and task workspace only.
The user selected this disable scope explicitly; unrelated workspaces may
supply credentials but cannot disable servers in the task.

This is a follow-up design package to the incomplete
[plugin import repair](../cursor-plugin-mcp-import/plan.md), not evidence that
Atlassian now works. Initial native service compatibility is the observed
macOS keychain installation of Cursor Agent 2026.09.26-dd393fe.
Existing global/local configuration and OAuth sharing keep their current
platform support.

Out of scope: login, token refresh/validation, account merging, plugin installation,
branch/release version resolution, remote executors, settings UI changes,
automatic server approval, and modifying the user's live task.

## Confirmed root cause and evidence

Task bc66d83a-0692-47c9-92de-11dec3af95d7 had only Kandev in its project MCP
file and an empty owned-import map. The cache guard required unverified
installed.json/plugins.json files that are absent in the real installation.

Its shared Atlassian credential entry contained clientInfo only. A newer
registration-only source won over an older source with both tokens and
clientInfo. Credential selection must prefer a complete credential-bearing
object without deep-merging accounts.

Read-only native service discovery returned Atlassian enabled at the exact
cached commit 9de1ab435251042efbb6839a1ca748eacecf4727. Harness was also enabled.
Figma's separate native identifier plugin-harness-figma was disabled in the
Projects/kandev workspace. Therefore service enablement alone cannot reproduce
workspace MCP preferences. No personal token values or raw service payloads
are retained in this package.

## Technical approach and dependency order

1. [Authenticated source selection](task-01-authenticated-source.md): rank whole
   credential objects, preserving source exclusion and deterministic tie-breaks.
2. [Native inventory and repository disables](task-02-native-inventory.md):
   introduce an injectable, bounded service client and platform credential
   reader; select exact eligible cache roots and filter source/task disables.
3. [Launch integration and compatibility proof](task-03-launch-proof.md):
   wire both launch modes, reconcile owned files, test all eligibility gates
   and prove native MCP registration against a disposable fixture.

Implementation is explicitly authorized in the primary session. Execute the
three work orders sequentially. No UI change or ASCII preview is needed.

## Work orders

- [x] Task 01: authenticated source selection
- [x] Task 02: native inventory and repository disable inheritance
- [x] Task 03: launch integration and compatibility proof

## Verification

Work orders own the exact test commands. Automated tests inject temporary
homes, credential readers and HTTP transports; none reads the developer's
keychain or contacts the real service.

The native smoke uses a disposable home/workspace, local MCP server and
synthetic provider credentials. It makes no prompt-driven repository edits or
real Jira calls. If the CLI requires account auth to initialize, use the
existing account through its native flow without copying account secrets
into fixture files; capture only sanitized outcome/tool names. Record a
separate approval requirement accurately rather than enabling all MCPs.

## Risks and compatibility

- The inventory RPC is a verified private native contract, not a stable public
  API. Unsupported schemas/platform stores fail closed with a sanitized reason.
- Token presence is not token validity. Cursor still owns OAuth refresh.
- Source-repository-only disables mean Figma may import for a different source
  repository that has not disabled it; this is the selected behavior.
- Failed inventory omits native marketplace imports; unavailable disable state
  omits all automatic imports. Both reconcile unchanged owned entries. Explicit user configuration survives.
- Existing approval behavior may be an additional registration gate. A passing
  file fixture is insufficient to claim a working agent tool surface.
- There is no persistent inventory cache and no transfer of account credentials
  to project MCP files. Account credentials go only to the fixed Cursor origin.

## Results

All three work orders are complete. Focused package and lifecycle race tests,
trusted launch/promotion metadata tests, scoped lint, specification validation,
and public-doc checks passed. Native terminal and ACP each registered and
called an authenticated synthetic MCP fixture using the native plugin identity.
See [compatibility evidence](../cursor-plugin-mcp-import/compatibility-evidence.md)
for commands and limits. This does not establish real Atlassian token validity.

Prior uncommitted identity/cache repair edits remain part of the implementation.
No commit, push, live-task restart or personal credential mutation was performed.

## Follow-up: profile-authorized preparation

The [agent MCP preparation package](../agent-mcp-preparation/plan.md) extends
this delivery with profile selection, native server approval/readiness, task
recovery and preservation of native credential refresh. Its results are tracked
separately; the historical verification above does not prove the new behavior.
