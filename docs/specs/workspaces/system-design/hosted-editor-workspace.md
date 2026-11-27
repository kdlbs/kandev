---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-HOSTED-EDITOR-001
---

# Hosted Editor Workspace System Design

## Ownership and existing contracts

Workspaces owns the launch target derived from an already resolved root.
[Editor file containment](editor-file-containment.md) owns lexical admission;
[task folder opening](../../tasks/system-design/open-task-folder.md) describes
the separate native file-manager action. Neither contract needs rewriting.
The [embedded capability ADR](../../../decisions/2026-07-30-embedded-editor-executor-capabilities.md)
remains authoritative. This restores a documented integration contract and
introduces no new architectural boundary, storage, or ADR.

## Requirement mapping

| Criteria | Design sections |
| --- | --- |
| `AC-WORKSPACES-HOSTED-EDITOR-001.1` | Root classification and URL construction |
| `AC-WORKSPACES-HOSTED-EDITOR-001.2` | Existing consumer wiring, Compatibility and errors |
| `AC-WORKSPACES-HOSTED-EDITOR-001.3` | Root classification and URL construction, Verification |
| `AC-WORKSPACES-HOSTED-EDITOR-001.4` | Compatibility and errors, Verification |
| `AC-WORKSPACES-HOSTED-EDITOR-001.5` | Existing consumer wiring, Presentation and documentation |

## Existing consumer wiring

`EditorsMenu.openWith` in `apps/web/components/task/editors-menu.tsx` invokes
`useOpenSessionInEditor.open({ editorId, worktreeId })` without `filePath`.
The shared hook sends `editor_id`, `worktree_id`, and the optional file and
coordinates through `openSessionInEditor`. `Controller.OpenSessionEditor`
passes these fields to `Service.OpenEditor` through the existing
`POST /api/v1/task-sessions/:id/open-editor` contract. The optional `url`
response is opened by the shared hook with `window.open`; internal VS Code
sentinels take the separate embedded path. No transport or client edit is needed.

`Service.OpenEditor` resolves session, settings, editor eligibility, workspace,
and lexical file target. `resolveSessionPath` accepts worktree and association
IDs; only an omitted ID permits existing first-worktree/repository fallback.
`resolveFilePath(root, "")` returns the root. Consequently, the current
`dispatchEditorKind` supplies a nonempty root to `buildHostedURL`, which treats
every nonempty path as a file. Its empty-path `folder=/` branch cannot represent
the real selected workspace request.

## Root classification and URL construction

Restrict the correction to `custom_hosted_url` near `dispatchEditorKind` and
`buildHostedURL` in `apps/backend/internal/editors/service/service.go`.
Carry the resolved worktree root into the hosted builder alongside the resolved
target. Compare the cleaned native paths after existing resolution/admission:
a target equal to that root is a workspace launch. Empty input, `.`, and
`src/..` therefore share the root outcome. Do not infer nested folder types,
inspect the filesystem, or reinterpret client paths.

For a root launch, set `folder` to the selected resolved absolute root without
coordinate suffixes. For other targets retain the current `file` construction,
including line-only and line/column suffix rules. Continue parsing the configured
URL with `net/url`, changing its query through `url.Values`, and preserving its
ordinary path and non-target query values. Decoded values, rather than query
order or incidental percent-encoding spelling, define preservation. No new
policy for base URLs carrying `file`, `folder`, `workspace`, or other vendor
preselection parameters is introduced by this fix. Existing isolated empty-path
helper behavior can be retained; production root opening must use the resolved
root rather than `/`.

The [Coder FAQ](https://github.com/coder/code-server/blob/main/docs/FAQ.md#how-does-code-server-decide-what-workspace-or-folder-to-open)
lists the folder query in directory selection. It does not establish support
for Kandev's existing legacy `file` parameter. File controls prove Kandev
compatibility only, not vendor-specific file navigation.

## Compatibility and errors

| Integration / shape | Result and evidence | Limit |
| --- | --- | --- |
| Custom hosted, resolved root | Exact decoded `folder` from real service, no generated `file` | Ordinary base URL; no vendor preselection precedence claim |
| Custom hosted, contained file | Existing decoded absolute `file` with coordinates | Existing Kandev contract only |
| Embedded root | Exact `internal://vscode`; capability rejection retained | No embedded process or panel launch |
| Remote SSH / builtin / custom command | Existing dispatch and helper behavior remains | Existing package controls; no OS/SSH launch |
| Missing/disabled/invalid/escaped request | Existing error category and empty URL | No selection or admission bypass |

Keep `resolveSessionPath`, `findSessionWorktreePath`, `resolveFilePath`, native
commands, SSH URI policy, and the early empty embedded-root branch unchanged.
No retries, persistence, telemetry, or lifecycle behavior is added. Existing
handler error mapping and shared localized error toast remain in force.

## Verification

Use independently authored tests in
`apps/backend/internal/editors/service/hosted_workspace_test.go`, invoking the
real `Service.OpenEditor` with ordinary repository/settings fakes and synthetic
`t.TempDir` roots. Exercise two different associated worktrees so selected-root
assertions cannot accidentally accept the first. Stub only dependencies; never
stub the service, derive expectations with the production builder, or launch an
external process. A small local fixture may embed the existing task repository
fake and override repository fallback/error reads.

Use explicit decoded expected URL fields, exact chosen roots, and empty URL plus
existing error categories for failures. Cover ordinary base path/non-target
query encoding and opaque special-character paths in the same service matrix.
Keep `editor_file_containment_test.go` intact. The plan owns named regressions,
controls, bounded commands, and evidence. Service-to-returned-URL is the changed
end-to-end boundary; registered HTTP routing and live IDE rendering are not
claimed.

## Presentation and documentation

The correction changes backend target data used by the shared desktop/phone
opener. It changes no rendered layout, touch target, scrolling, navigation,
focus, or copy. Apply the mobile state/data exception with targeted service
coverage and source wiring evidence; no frontend changes, browser gate, mobile
E2E, or ASCII UI preview are needed.

`docs/public/developer-tools.md` already promises selected-worktree launching
and hosted file/folder query parameters. Searches of the root README and
screenshot catalog found no conflicting launch instructions. The public page
is a how-to guide with editor reference material; restoring its stated behavior
needs no public-doc or screenshot edit.
