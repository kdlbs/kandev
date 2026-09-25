---
status: draft
system: agents
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-AUTH-002
  - REQ-AGENTS-CURSOR-AUTH-003
---

# Cursor MCP OAuth bridge system design

## Ownership and source evidence

Agents owns the profile value and launch policy. Executors owns the local-versus-remote boundary.
The [settings parity contract](../../platform/requirements/agent-settings-parity.md) supplies discovery and mutation conventions.

Current `materializeRuntimeProjectMCP` writes project MCP configuration for ACP providers.
It returns early when no servers exist.
Current `applyPassthroughMCP` handles terminal strategies separately.
`CursorACP.Runtime` declares `mcpconfig.CursorStrategy{}`.
Custom terminal agents can select the persisted strategy key `cursor`.

The user supplied the Cursor auth layout and slug algorithm.
Automated tests will exercise that contract with synthetic credentials.
They cannot prove compatibility with every Cursor release.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-CURSOR-AUTH-001 | Eligibility, aggregation, launch integration |
| REQ-AGENTS-CURSOR-AUTH-002 | Profile contract, disable behavior, settings UI |
| REQ-AGENTS-CURSOR-AUTH-003 | File safety, failures, verification |

## Profile contract

Add `cursor_mcp_auth_enabled` to the persisted agent profile and shared DTO contracts.
Use `CursorMCPAuthEnabled` in Go and `cursorMcpAuthEnabled` in browser models.
This is a profile preference with a user-requested default of true, not a release toggle.

Add an integer SQLite column with `NOT NULL DEFAULT 1`.
Cover fresh schema creation, additive migration, and legacy table reconstruction.
Create requests use a boolean pointer so omission selects true and explicit false remains false.
Patch omission preserves the saved value.
All creation paths, including discovery and seeding, must set the default explicitly when SQL inserts include the column.
Duplication copies the value. Reconciliation must not reset a saved false.

Extend models, DTOs, CRUD conversion, SELECT/scan, INSERT/UPDATE, boot payloads, and profile notifications.
Register the boolean in `settings/dto/profile_contract.go` and `settingscatalog/defaults.go`.
Use the existing compact settings update contract and field validation.
Only the preference crosses APIs. Credential objects never cross those boundaries.

Extend `AgentProfileInfo` and `profile_resolver.go`.
Launch policy uses the resolved concrete execution profile, including Office and dynamic-profile selection.
Do not resolve an Office identity profile in place of its execution profile.
Missing or failed profile resolution must not silently enable credential sharing.

## Eligibility and home

The bridge runs for `cursor-acp` and effective `CursorStrategy` profiles.
Other agents do not read or mutate Cursor auth data.
Resolve eligibility from the actual agent/strategy, not a command-name substring.

Only native local executions that share the backend filesystem and user home are eligible.
Use existing executor/runtime classification and effective process environment.
Exclude containers and remote runtimes even when a backend-local workspace path exists.
If a profile overrides HOME to another user directory, skip host sharing rather than link unusable or unintended credentials.
Treat unknown execution locality as ineligible.

Resolve the backend user home through Go, then use its `.cursor` directory.
Do not create a missing Cursor home.
Tests inject a temporary home and never scan the developer's home.

## Helper boundary and aggregation

Add `mcpconfig/cursor_auth_bridge.go` with the requested public helpers:

- `DeriveCursorProjectSlug(workspacePath string) string`
- `AggregateCursorMCPAuth(cursorHome string) error`
- `LinkCursorMCPAuth(workspacePath, cursorHome string) error`

Keep slug derivation pure. The link helper resolves an absolute path and evaluates workspace symlinks before derivation.
Replace each slash, dot, and underscore with a dash, then trim boundary dashes.
Do not collapse internal dash runs. Reject an empty resulting slug.

Scan direct project directories under `<cursorHome>/projects`.
Exclude names containing `kandev-tasks`, symlinked project entries, and non-directory entries.
Use `Lstat` for each auth file and accept only regular, non-symlink files.
Also reject a symlinked projects root to avoid scanning an unexpected tree.

Decode JSON as `map[string]json.RawMessage`.
Require an object root and object-valued server entries.
Skip invalid files as a whole. Preserve unknown fields inside server objects.
Order sources by descending modification time and ascending path.
Select the first object for each exact server key.
Do not deep-merge tokens or combine accounts.

Rebuild `<cursorHome>/kandev-mcp-auth-unified.json` from sources on each eligible enabled launch.
The prior master is never a source.
A valid empty source object can produce an empty snapshot.
No eligible valid source means no master publication and no new link.
Use an internal result that distinguishes no sources from a successful empty snapshot.
The public error-only helper can wrap this result.
The composed link operation uses the result directly and must not infer success from an old master.

## File safety and concurrency

Use a package mutex around the complete bridge operation.
Concurrent Kandev launches serialize discovery, publication, linking, and disabled cleanup.
The lock must also cover public helper calls without recursive locking.
Independent backend processes and Cursor do not participate in this mutex.
Unique temporary names and atomic publication keep their writes complete, but cannot provide token-refresh coordination.

Write the master to a temporary file in the same directory.
Create it with mode `0600`, write complete JSON, close it, then rename it into place.
Remove temporary files after every failure.
Reject a pre-existing master that is a symlink, directory, or special file.
The master is a Kandev-owned regular file and can be replaced.
The prohibition on replacing regular user auth files applies to the project destination.

Create the project directory with requested mode `0755`, subject to umask.
Do not change permissions on existing user directories.
Reject a symlinked project destination directory.
Check destination `mcp-auth.json` with `Lstat`.
If it is regular, a directory, or special, preserve it without replacement.
If it already points to the master, leave the link unchanged.
For an absent destination, create the final symlink directly. Creation is atomic and fails if a file appears concurrently.
For an existing symlink, create a uniquely named sibling symlink and rename it over the old link.
Recheck destination type immediately before replacement.
This prevents ordinary launch races under the package lock.
It does not claim protection against an adversarial process that replaces paths between filesystem calls.

The symlink target is the absolute master path.
Bridge files do not enter generic MCP teardown lists.
Stopping one session must not remove credentials used by another session.

## Disable behavior

On the next eligible disabled launch, resolve the canonical project destination without creating directories.
Use `Lstat` and `Readlink`.
Remove only a symlink whose normalized absolute target is the Kandev master.
Accept an equivalent relative target after resolution against the link directory.
Preserve regular files, unrelated links, and the master.
Do not aggregate on this path.

A missing link is successful cleanup.
If a matching link cannot be removed, return a sanitized launch error.
Otherwise a disabled launch could still inherit shared credentials.
Saving the preference does not touch the filesystem or terminate running sessions.

## Launch integration

Call one lifecycle helper from `materializeRuntimeProjectMCP` before the empty-server return.
Call it from `applyPassthroughMCP` for the Cursor strategy before ordinary MCP materialization.
Keep `CursorStrategy.BuildPassthroughMCP` free of home-directory side effects.
Its callers also serve command previews and must not publish credentials.

Pass the resolved execution profile into bridge preparation.
The two normal launch sites in `manager_launch.go` already hold resolved profile information.
Workspace promotion must use the incoming execution profile even before it updates execution metadata.
Exercise initial launch, workspace promotion, and resumed execution that starts a new process.
Do not rerun preparation for a message sent to an already running process.

## Failures and diagnostics

Missing Cursor home, absent sources, ineligible runtime, and protected user destinations produce no warning.
Malformed or unreadable sources can produce a bounded warning with an operation/reason code.
Never log JSON, server objects, tokens, or raw parser errors containing source text.

Enabled publication or link errors produce a sanitized warning and preserve normal launch behavior.
A failed publication does not create a link to an older master.
Existing links can still reference the previous snapshot after a failed refresh.
This is a compatibility limit, not a claim of fresh authentication.
Disabled cleanup errors fail the new launch as described above.

## Settings UI and mobile contract

Show a checkbox in the existing profile settings card for Cursor and custom terminal agents whose MCP strategy is Cursor.
Use `@kandev/ui/checkbox`, a linked label, and visible description.
Do not classify this setting as a permission or send it as a CLI flag.
The value participates in create/edit drafts, dirty detection, save/discard, external-change reconciliation, and normalization.

Proposed localized copy:

- Label: "Share local Cursor MCP credentials"
- Description: "Reuse MCP sign-ins from other local Cursor projects. Applies on the next launch."
- Disabled timing note: "Turning this off removes the shared link on the next launch. Running sessions keep credentials already loaded."

Use the existing Settings save controls and failure messages.
Add English, Portuguese, Simplified Chinese, both Traditional Chinese catalogs, and Japanese.
Generate the Traditional Chinese pair with `pnpm run i18n:zh-hant`.

Entry point: Settings > Agents > applicable profile.
The nearest shipped exemplar is `agent-profile-page.tsx` and its shared settings card.
`mobile-agent-profile-layout.spec.ts` demonstrates direct phone navigation and overflow checks.
The form stays inline on the full settings route because this is one persistent preference.
The existing settings scroll container remains the sole page scroll owner.
The phone label wraps and supplies a 44px minimum touch target.
Desktop keeps normal form density. Both sizes share draft state and save logic.
No overlay or hover-only help is necessary.

## Verification and limits

Use synthetic opaque server objects and a temporary HOME.
The helper suite covers source exclusion, conflict order, canonical slug, permissions, atomic publication, protected destinations, concurrency, and disable cleanup.
Lifecycle tests cover eligibility, both launch modes, promotion, no-server behavior, profile resolution, and failure policy.
A temporary git repository/worktree test proves the expected link target without real OAuth.

Desktop and phone Playwright tests prove default-on, save/reload false, re-enable, discard, and absence for other agents.
Unit tests cover request omission, migration, duplication, and frontend draft reconciliation.
The [plan](../../../plans/cursor-mcp-oauth-bridge/plan.md) owns exact commands and test names.

Cursor can replace a symlink during its own auth writes. A later enabled launch repairs it only if it remains a symlink.
If Cursor creates a regular file, Kandev preserves that file.
Cursor token refresh through a link can update the master, but the next aggregation still uses source-project precedence.
This design does not synchronize refresh writes back to source projects.
