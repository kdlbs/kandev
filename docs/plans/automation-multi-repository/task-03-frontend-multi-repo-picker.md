---
id: "03-frontend-multi-repo-picker"
title: "Frontend: executor-gated multi-repository picker in the automation editor"
status: pending
wave: 2
depends_on: ["01-backend-data-model"]
plan: "plan.md"
spec: "../../specs/office/requirements/automations-settings.md"
---

# Task 03: Frontend multi-repository picker

## Inputs

- `docs/specs/office/requirements/automations-settings.md` — the "Multi-repository
  selection is gated on the selected executor profile's capability" bullets
  under `## What`, plus the corresponding Scenarios.
- `apps/web/components/task-create-dialog-multi-repo-guard.ts` —
  `getMultiRepoExecutorDisabledReason(executorType)`; import directly, do not
  duplicate the supported-type set (`worktree`, `local_docker`, `ssh`,
  `sprites`).
- `apps/web/components/automations/config-section.tsx` — current single-repo
  picker (`RepositorySelection`, `buildRepositoryItems`,
  `pickSelectionFromOptionId`, `selectionToOptionId`, `SelectField`,
  `ConfigSection`).
- `apps/web/components/automations/automation-payload.ts` —
  `FormState.repositorySelection`, `resolveRepositoryId`,
  `buildCreatePayload`/`buildUpdatePayload`.
- `apps/web/components/automations/automation-editor.tsx` — `defaultForm`,
  `formFromAutomation`, `useSaveHandler`.
- `apps/web/components/automations/automation-editor-sections.tsx` —
  `repositorySelection` prop/dirty-field wiring (~lines 143-205).
- `apps/web/lib/types/automation.ts` — `Automation.repository_id`,
  `CreateAutomationRequest`/`UpdateAutomationRequest.repository_id`.
- `apps/web/lib/types/http.ts` — `ExecutorProfile.executor_type` (already
  present on the profile object; no type change needed here).
- Reference pattern (do not import directly, the settings page keeps its own
  lighter picker): `apps/web/components/task-create-dialog-repo-chips.tsx`
  and `task-create-dialog-computed.ts`'s `pickExecutorDisabledReason` for how
  task creation gates the *executor* picker once 2+ repos are selected.

## Change

1. **Types** (`apps/web/lib/types/automation.ts`): `repository_id: string` →
   `repository_ids: string[]` on `Automation`; same rename on
   `CreateAutomationRequest`/`UpdateAutomationRequest`.
2. **`config-section.tsx`**:
   - `RepositorySelection` keeps its existing `{kind:"none"}` variant —
     still needed by the single-picker fallback branch below — but the
     repeatable multi-row picker (see the new file below) never produces
     one; each row is always fully resolved the moment it's added.
   - `ConfigSectionProps`: `repositorySelection` → `repositorySelections:
     RepositorySelection[]`; `onRepositoryChange` → `onRepositoriesChange:
     (selections: RepositorySelection[]) => void`;
     `dirtyFields.repositorySelection` → `dirtyFields.repositorySelections`.
   - Compute `selectedExecutorProfile = allExecutorProfiles.find(p => p.id
     === executorProfileId)` and `multiRepoDisabledReason =
     getMultiRepoExecutorDisabledReason(selectedExecutorProfile?.executor_type)`.
   - Extend the local `SelectField`'s `items` prop type to
     `Array<{id: string; label: string; disabled?: boolean; disabledReason?:
     string}>`; render `<SelectItem disabled={item.disabled}
     title={item.disabledReason}>`.
   - Executor Profile `SelectField` items: set `disabled:
     repositorySelections.length > 1 &&
     getMultiRepoExecutorDisabledReason(p.executor_type) !== null` and
     `disabledReason: getMultiRepoExecutorDisabledReason(p.executor_type) ??
     undefined`.
   - Repository picker: branch on `multiRepoDisabledReason === null &&
     conditionType !== "github_pr"`:
     - compatible → render new `AutomationRepositoryRows` (new file, see
       below).
     - incompatible (single-repo executor, or a `github_pr` trigger) → keep
       today's single `SelectField`, bound to `repositorySelections[0] ??
       {kind:"none"}`, with a `"__none__"` sentinel mapping to an empty
       array (reuse existing `REPO_NONE_OPTION_ID`/`selectionToOptionId`/
       `pickSelectionFromOptionId` unchanged — it still returns
       `{kind:"none"}` for the sentinel/unresolvable case). For `github_pr`
       specifically, additionally disable the dropdown and show helper text
       ("PR triggers always use the PR's own repository.") — the PR's own
       repository always wins regardless of what's selected, so the picker
       is informational only.
3. **New file** `apps/web/components/automations/automation-repository-rows.tsx`:
   - Props: `repositorySelections: RepositorySelection[]`, `repositories:
     Repository[]`, `discoveredRepos: LocalRepository[]`, `onChange:
     (selections: RepositorySelection[]) => void`.
   - Renders one `SelectField`-equivalent row per selection (reuse
     `buildRepositoryItems`, marking an option already selected in another
     row as `disabled: true, disabledReason: "Already added"` — same
     semantics as the task-creation dialog's marker, simplified to a
     disabled option here since this is a plain native select, not a chip
     picker).
   - "Add repository" button appends a new row and immediately fills it with
     the next available (not-yet-selected) repository. `RepositorySelection`
     retains its transient `{kind:"none"}` variant for the single-picker
     fallback below, but the repeatable rows never produce one — each row's
     `onChange`/`addRow` handler bails out on `kind === "none"` (an
     unreachable defensive guard here, since `pickSelectionFromOptionId`
     only returns it for the `"__none__"` sentinel or a stale/unresolvable
     ID, neither of which this picker offers as an option). The button
     disables once every known repository (registered + discovered) is
     already used, since there's nothing left to fill a new row with.
   - Per-row remove control; removing the last row is allowed (results in an
     empty array, handled by the workspace-fallback behavior documented in
     `config-section.tsx`'s helper text below the picker).
4. **`automation-payload.ts`**:
   - `FormState.repositorySelection` → `repositorySelections:
     RepositorySelection[]`.
   - `resolveRepositoryId` → `resolveRepositoryIds(workspaceId, selections:
     RepositorySelection[]): Promise<string[]>` via `Promise.all` over the
     existing per-selection resolution logic.
   - New `normalizeRepositorySelections(selections, {supportsMultiRepo,
     isPRTrigger})` (in `automation-repository-selection.ts`): truncates to
     at most one entry when the picker is in single-repository mode. The
     picker only *renders* `repositorySelections[0]` in that mode — it
     doesn't truncate the underlying array — so a form that had 2+ repos
     selected under a compatible executor, then switched to an incompatible
     executor or a `github_pr` trigger without the picker being touched
     again, would otherwise still save every stale entry.
   - New `resolveNormalizedRepositoryIds(workspaceId, selections, mode)`:
     the save-boundary entry point — normalizes, then resolves. Replaces
     the direct `resolveRepositoryIds` call in `useSaveHandler`.
   - `buildCreatePayload`/`buildUpdatePayload`: `repository_id: repositoryId`
     → `repository_ids: repositoryIds`.
5. **`automation-editor.tsx`**:
   - `defaultForm.repositorySelections: []`.
   - `formFromAutomation`: `a.repository_ids.map(id => ({kind:"registered" as
     const, id}))`.
   - New `useSupportsMultiRepo(executorProfileId)` hook mirrors
     `ConfigSection`'s own `getMultiRepoExecutorDisabledReason` check via the
     shared `resolveExecutorType` helper, so the save handler enforces the
     same capability the picker rendered.
   - `useSaveHandler`: switch to `resolveNormalizedRepositoryIds(workspaceId,
     form.repositorySelections, {supportsMultiRepo, isPRTrigger})`; promote
     each discovered selection independently by mapping
     `form.repositorySelections` against the resolved `repositoryIds` array
     by index (only rows with `kind === "discovered"` get promoted to
     `{kind:"registered", id: repositoryIds[i]}`).
6. **`automation-editor-sections.tsx`**: rename the
   `repositorySelection`/`onRepositoryChange`/dirty-field wiring to the
   plural forms throughout.
7. After the rename, `grep -rnP '\brepositorySelection\b' apps/web/components/automations`
   must return no matches (confirms no missed call site).

## Acceptance

- With a `worktree`/`local_docker`/`ssh`/`sprites` executor profile selected,
  the repository picker renders as a repeatable list with a working "Add
  repository" control.
- With a `local`/`local_pc`/`remote_docker` executor profile selected, the
  repository picker renders as today's single dropdown with no "Add
  repository" control, and is bound to at most one selection.
- With 2+ repositories selected, incompatible executor profiles are rendered
  disabled in the Executor Profile dropdown with a title/reason matching
  `getMultiRepoExecutorDisabledReason`'s message.
- Save round-trips an ordered multi-repository list end to end (`FormState` →
  `repository_ids` payload → `formFromAutomation` on reload).
- Saving with a mix of registered and newly-discovered repository rows
  registers only the discovered ones and promotes just those rows.
- With a `github_pr` condition selected, the repository picker renders as the
  single dropdown (regardless of executor profile), is disabled, and shows
  "PR triggers always use the PR's own repository." Any `repository_ids`
  left in the saved payload from before the trigger was switched to
  `github_pr` are inert: the orchestrator's `resolveAutomationRepository`
  checks trigger type first and resolves from the PR's own trigger data
  unconditionally for `github_pr`, never reading `RepositoryIDs` (task 02;
  see `TestResolveAutomationRepository_GitHubPRIgnoresConfiguredRepositoryIDs`).
- Editing a form with 2+ repository selections, then switching to a
  single-repo-only executor or a `github_pr` trigger without touching the
  picker again, and saving: the emitted `repository_ids` payload has at
  most one entry on both create and update, even though the underlying
  `repositorySelections` state still held every stale entry. Covered by
  `resolveNormalizedRepositoryIds` unit tests in `automation-payload.test.ts`
  (truncation for an incompatible executor, for a `github_pr` trigger, and
  feeding the truncated ids into both `buildCreatePayload` and
  `buildUpdatePayload`).

## Verification

```shell
(cd apps && pnpm --filter @kandev/web test -- automations/config-section.test.tsx automations/automation-payload.test.ts automations/automation-repository-selection.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/web/lib/types/automation.ts`
- `apps/web/components/automations/config-section.tsx`
- `apps/web/components/automations/config-section.test.tsx`
- `apps/web/components/automations/automation-repository-rows.tsx` (new)
- `apps/web/components/automations/automation-payload.ts`
- `apps/web/components/automations/automation-payload.test.ts` (new — check
  first whether one already exists)
- `apps/web/components/automations/automation-editor.tsx`
- `apps/web/components/automations/automation-editor-sections.tsx`

## Dependencies

Task 01 (needs the `repository_ids` wire contract to match).

## Parallelism

`parallel-safe` with task 02 — disjoint files, both depend only on task 01's
finished contract.

## Output contract

Summary of changes, confirmation the `grep -rnP '\brepositorySelection\b'`
check returned no matches, exact test/typecheck command output, and a note
updating `plan.md`'s Wave 2 checkbox and this file's `status` to `done`.
