---
created: 2026-11-11
status: implemented
requirements:
  - REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001
system_design:
  - ../../specs/integrations/system-design/shared-prompt-writes.md
legacy_specs: []
---

# Implementation plan: Custom prompt agent editing

## Overview

Remove the permission switch and make every custom prompt editable through
existing authorized agent write paths. One vertical work order removes the
permission state, updates the editor, and proves the resulting behavior.
Implementation and verification results are recorded below.

The integration system owns the saved-prompt authorization contract.
The prompt service owns persistence and reference expansion.

## Scope

### In scope

- Every user-created prompt, including historical prompts whose permission is off.
- Dedicated prompt writes, generic settings updates, HTTP compatibility, and persistence.
- Desktop and phone editor removal, locale cleanup, and public documentation.
- Explicit-request instructions, retained built-in protection, and conflict detection.

### Out of scope

- Agent edits to built-ins, new tool exposure, MCP deletion, or dedicated-tool renaming.
- New confirmation protocols, automatic prompt improvement, or workflow rewrites.
- Editing captured running-turn instructions or changing prompt ownership scope.
- Commits, pushes, PR creation, and delegation.

## Technical approach

The paired [design](../../specs/integrations/system-design/shared-prompt-writes.md)
owns the target contracts. The
[decision](../../decisions/2026-11-11-custom-prompt-agent-editing.md)
records the policy and compatibility rationale.

- Remove stored permission use from `internal/prompts/models`, `service`, and `store`.
  Keep conditional ID, name, built-in, and timestamp guards.
- Remove fresh-schema dependence on `allow_agent_edits` and its additive migration.
  Ignore historical columns without rebuilding tables or backfilling prompt rows.
- Derive existing HTTP, MCP, and settings response metadata from `!Builtin`.
  Accept deprecated HTTP permission input without applying it.
- Remove `PromptAgentPermission`, permission form state, payloads, revision tracking,
  and unused locale keys. Keep the existing save bar and local content drafts.
- Update configuration context and tool descriptions to describe all custom prompts
  and explicitly requested changes. Preserve current tool catalogs and authority checks.
- Update `docs/public/automation-and-mcp.md` (reference) and
  `docs/public/developer-tools.md` (how-to). Describe the changed default and future-reference effects.
  Search `README.md` and `docs/screenshots.md` for affected instructions or captures.

The original [shared prompt package](../shared-prompt-mcp-writes/plan.md)
remains historical. Its results describe the original permission feature.
New tests replace its permission cases while retaining live delivery and conflict evidence.

## UI preview

UI-01: Settings > Prompts, editing a user-created prompt, desktop and phone.
The design handoff renders this region through Kandev rich output, as requested
by the task's preview instructions.

The control order is:

1. Prompt name input, containing `review-policy`.
2. Prompt content editor, containing the saved instructions or local draft.
3. Cancel action.
4. Existing floating Discard and Save changes actions while the form is dirty.

The Allow agent edits row and its explanatory copy are absent.
No replacement control or new explanatory row appears.
The same composition applies on a phone. The page remains the sole outer
scroll owner, with the existing safe-area-aware save bar and touch actions.
This uses the current inline prompt editor, without an overlay or new navigation.
Control order and existing save behavior are structural requirements.
Example content and field width are illustrative.

UI-01 maps to `.12`, `.9`, and `.7` of the linked requirement.
The desktop and mobile E2E scenarios below provide rendered evidence.

## Tests

All suffixes refer to `AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001`.

| Criteria | Evidence |
| --- | --- |
| .9, .13, .16 | `store/agent_permission_test.go`: replace migration/revocation cases with legacy-zero/one eligibility and schema replay. Retain `TestPromptAgentWriteGuards` and `TestPostgresPromptAgentWriteGuards` for conflicts and built-ins. |
| .9, .10, .3 | `service/agent_writes_test.go`: extend `TestAgentPromptWrites` for Settings-created prompts and retain duplicate, exact-name, missing, and built-in cases. |
| .11, .8 | `mcp/handlers/config_prompt_writes_test.go`: retain configuration-authority, validation, storage-error, and denied-write coverage. |
| .15, .13 | `prompts/handlers/agent_permission_test.go`: replace `TestPromptHTTPAgentPermission` with derived metadata and ignored legacy input. Retain `TestPromptHTTPConcurrentUpdateReturnsConflict`. |
| .9, .10, .15 | `backendapp/settings_prompt_permissions_test.go`: replace `TestSettingsPromptWritesRespectAgentPermission` with `TestSettingsPromptWritesAllowCustomPrompts`. Keep built-in rejection and effective metadata checks. |
| .1, .14 | `mcp/server/config_prompt_writes_test.go`: retain `TestSharedPromptWritesCatalog`; assert requested-change wording. Extend existing `config/prompts` context checks. |
| .12, .7 | `prompts-settings.test.tsx` and `prompts-settings-draft-meta.test.tsx`: replace permission-intent cases with content/name save, no permission payload, name/content-only dirty and revision tracking, and draft-preserving refresh. |
| .16 | `storeconformance/upgrade_test.go`: retain fixture and previous-stable replay, without a required permission value in the resulting schema. |

## E2E tests

- Chromium `tests/settings/shared-prompt-mcp-writes.spec.ts`: an HTTP-created prompt
  accepts MCP edits immediately. An open editor keeps its draft during a remote
  update. Built-ins reject writes. No permission switch appears (`.9`, `.10`, `.7`, `.12`).
- Chromium `tests/settings/prompts-settings.spec.ts`: name/content editing, save,
  cancellation, and existing prompt workflows remain functional (`.12`).
- Mobile Chrome `tests/settings/mobile-prompts-settings.spec.ts`: replace the
  permission scenario with touch editing, save/reopen, and a subsequent MCP edit.
  Assert the absent switch, reachable save action, and no document overflow (`.9`, `.12`).

Both projects capture the simplified editor through existing `prCapture` fixtures.
The mobile project supplies its normal Pixel 5 profile. No per-test device override is needed.
Run the projects sequentially. The first runner builds the backend and frontend.
The second can use `--no-build` only without intervening source changes.

## Work orders

- [x] [Task 01: Remove custom prompt permission](task-01-remove-custom-prompt-permission.md)

The single work order is sequential. It includes meaningful Red-Green-Refactor
coverage for changed behavior. It does not authorize delegation.
The work order contains all exact verification commands.

## Verification results

- Backend race tests passed for `internal/prompts/...`, `internal/mcp/handlers`,
  `internal/mcp/server`, `config/prompts`, and prompt/settings paths in
  `internal/backendapp`.
- Store upgrade conformance passed for SQLite and PostgreSQL fixture and previous-stable
  replay. PostgreSQL ran against a disposable local PostgreSQL 16 container.
- Changed-Go lint passed with 0 issues.
- Focused frontend Vitest passed 34 tests across 5 files. Web typecheck,
  `i18n:check`, and focused Oxlint passed.
- The E2E runner's host backend and pseudo-locale Vite build passed. Chromium
  passed 9 tests and Mobile Chrome passed 2 tests, including captured runs.
- Desktop and phone editor screenshots were captured and inspected against UI-01.
  Both show name and content with no permission row. Desktop also shows Cancel and
  the floating Discard/Save bar. The phone uses the existing floating bar over the
  lower editor at the captured scroll position; the editor remains in the page's
  scroll flow and no new overlay was introduced. No structural mismatch remains.
- Specification catalog validation, all 36 specification-validator tests,
  specification lint, public-doc tests and validation, and `git diff --check`
  passed after the final status updates.
- Documentation-linkage preflight returned `status: covered` with no errors.

The worktree remains unstaged and uncommitted.

## Risks

- Previously protected custom prompts become editable through all existing authorized
  write paths, including external MCP and generic settings. This is the intended policy change.
- Explicit user intent is an agent instruction. Configuration authority remains the backend admission check.
- Old clients can submit a retired permission input, but cannot disable custom-prompt agent edits.
- Removing permission state must preserve conditional writes, cache invalidation, and local editor drafts.
- PostgreSQL results require an isolated test DSN. Skipped cases do not prove PostgreSQL behavior.
