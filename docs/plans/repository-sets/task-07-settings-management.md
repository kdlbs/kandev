---
id: "07-settings-management"
title: "Manage sets in workspace settings"
status: done
wave: 5
depends_on: ["04-boot-and-web-data-layer"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 07: Manage Sets In Workspace Settings

## Acceptance

- **Settings > Workspaces > _workspace_ > Repositories** gains a **Repository sets** section below
  the repository list, listing each set with its name, description, and member repositories, and
  offering create, rename, edit members, reorder members, and delete. Deleting asks for confirmation
  and states that no repository is affected.
- Member selection offers only live repositories of that workspace; a set must keep at least one
  member to save. An existing set whose members were all deleted lists as empty and is not
  auto-removed. A duplicate name is reported inline naming the existing set.
- The section is read-only in the Improve workspace, matching the page's existing `isImproveWorkspace`
  handling; it follows the page's `SettingsSection` and save-contributor conventions, works at phone
  widths as a native pattern, and its copy exists in all five locales with no em dashes.
- No new settings tab is added: the tab table, settings routes, and discovery catalog are unchanged.

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm vitest run app/settings/workspace
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/app/settings/workspace/workspace-repository-sets-section.tsx` (new) and its `.test.tsx`
- `apps/web/app/settings/workspace/use-workspace-repository-sets.ts` (new) and its `.test.ts`
- `apps/web/app/settings/workspace/workspace-repositories-client.tsx` (render the section below the
  Repositories `SettingsSection` at `:559`)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/`

## Dependencies

Task 04 for the hook, slice, and API client. Parallel-safe with task 05: different files.

## Inputs

- Spec: Defining a set, Permissions, Failure modes.
- Patterns: `SettingsSection` usage in `workspace-repositories-client.tsx:559`;
  `useSettingsSaveContributor` (`components/settings/settings-save-provider.tsx`) if the section
  holds unsaved local state.
- `workspace-repositories-client.tsx` is 624 lines against an 800-effective-line cap, so the section
  and its state hook live in their own files and the client only renders the section.
- Run `/mobile-parity` for the phone presentation of the editor.

## Risks

- Reordering members expresses itself as a full `repository_ids` replace; a partial update that omits
  the list would wipe membership.
- Set names are user data and must never be routed through `t()`.

## Output contract

Summary, files changed, tests run with results, blockers, risks, divergence from the plan, and
task/plan status updates.
