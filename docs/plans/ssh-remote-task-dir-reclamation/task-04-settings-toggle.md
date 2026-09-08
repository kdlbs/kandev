---
id: "04-settings-toggle"
title: "SSH profile reclamation settings toggle"
status: done
wave: 3
depends_on: ["02-profile-opt-in"]
plan: "plan.md"
requirements: "../../specs/executors/requirements/remote-task-directory-reclamation.md"
acceptance_criteria:
  - AC-SSH-TASKDIR-RECLAMATION-006.1
  - AC-SSH-TASKDIR-RECLAMATION-006.4
  - AC-SSH-TASKDIR-RECLAMATION-001.4
system_design: "../../specs/executors/system-design/remote-task-directory-reclamation.md"
---

# Task 04: SSH profile reclamation settings toggle

## Summary

Expose `ssh_reclaim_task_dir` as a per-profile switch in the SSH executor profile
settings, defaulting to off, with copy that states the blast radius: which host,
which workspace root, that removal is permanent, and that an unarchived task
re-clones.

## Scope

- Switch in the SSH executor profile settings surface, adjacent to
  `workdir_root` since it scopes to the same root.
- Help text naming the host and the resolved `<workdir_root>/tasks/` path.
- All copy through `t()`; keys added to `en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`
  (use `pnpm run i18n:zh-hant` for the Traditional pair).
- No Unicode em dash in any user-facing string.

## Exclusions

- No browser of preserved directories.
- No bulk or retroactive sweep control.

## Implementation acceptance conditions

1. The switch defaults to off for a profile with no stored value and persists
   `"true"` / `"false"` to the profile config.
2. Help text names the host and the `<workdir_root>/tasks/` path the setting
   governs and states that removal is permanent.
3. Desktop and mobile both expose the setting as a touch-sized inline control
   with no new drawer or navigation level.

## Verification

```bash
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps && pnpm --filter @kandev/web lint)
```

## Files likely touched

- `apps/web/components/settings/ssh-settings.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/*.json`

## Dependencies

Task 02 (the config key must exist).

## Inputs

- System design section `Per-profile setting`.
- `/mobile-parity` for the mobile control pattern.
- `docs/i18n.md` for the five-locale gate.

## Risks

Copy that undersells permanence. The setting arms an irreversible remote
deletion and the text must say so plainly.

## Safety

No deletion path is reachable from this task.

## Output contract

Set `status` to `done`, tick the box in `plan.md`, and report the control
placement, the i18n keys added, and commands run.
