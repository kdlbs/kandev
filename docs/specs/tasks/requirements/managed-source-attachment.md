---
status: active
system: tasks
created: 2026-10-21
owners:
  - kandev
---

# Managed source attachment requirements

## Overview

This is the managed-source recovery part of [task source attachment](attach-workspace-sources.md),
split at its ownership-admission boundary to keep that document within its context limit.
The task system owns successful attachment, failure compensation and the visible
recovery outcome; workspace/provider systems retain repository and clone identity.

## Terminology

- **Current managed destination:** The exact workspace/provider identity-derived
  source-clone location used by current materialization, including scoped identity
  when present. It is not any directory below managed storage.
- **Legacy managed source:** An exactly registered older provider/origin or
  owner/name layout, admitted only with surviving path and Git ownership proof.

## Requirements

### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-008: Recover current managed sources

**Intent:** Complete eligible attachment when a current managed provider clone is absent.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1:** For an eligible host Local or Worktree source attachment, or live Worktree add-branch call, an empty provider path or missing exact current managed destination shall be materialized and exposed as readable task content with the requested branch/base and canonical inventory before success.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2:** A managed source shall pass exact workspace/provider identity, destination, Git origin and control-path ownership validation before reuse, clone mutation or successful path publication. Foreign, occupied, redirected, unverifiable and mismatched clone output shall fail without changing foreign content. Scoped identities shall not reuse unscoped legacy paths. The expanded operator-configured clone base is the control-root boundary: aliases in its parents are supported, while redirection at the base or its descendants is refused. Valid space/newline path bytes shall remain intact; resolving physical Git paths after inspection shall not grant ownership or rewrite lexical registration paths.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3:** Explicit local sources shall retain their exact local ownership despite provider metadata; absent local sources shall fail without cloning. Validated existing legacy managed sources shall remain reusable at their saved location. Missing legacy sources shall fail with guidance to re-import, without implicit recreation or redirection.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4:** Missing or changed authorized task/session/environment identity, unavailable clone validation or credentials, denied authentication and cancellation shall fail before successful attachment. Verified clone-path persistence failure shall prevent use of the new path. The attachment-only internal write shall authorize the persisted workspace, match the admitted identity/version, preserve the verified clean absolute lexical path and all unrelated fields, and publish the existing repository update after an exact write. Ordinary explicit-local canonicalization shall remain unchanged; no public request bypass shall be introduced.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.5:** Failed attachment shall restore prior task attachments, owned entries and inventory under existing compensation rules. A successfully verified shared managed clone may remain after a later task-owned failure; retries shall reuse it without duplicate attachments or worktrees.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.6:** Existing idle batch, active-turn Worktree branch, inherited environment, prelaunch deferral and remote-executor behavior shall remain applicable. This recovery shall not inspect or repair host clones for remote tasks or relocate existing task checkouts.

## Scope and compatibility

This selects current-destination recovery only, a partial response to #4426.
It does not authorize missing legacy recreation, task-checkout relocation,
provider-policy changes, background recovery, live deployment or overlay removal.
Desktop and phone retain the existing attachment action and error presentation.

## Related contracts

- [Source attachment](attach-workspace-sources.md)
- [System design](../system-design/current-managed-attachment-recovery.md)
- [Recovery decision](../../../decisions/2026-10-21-current-managed-attachment-recovery.md)
- [Implementation plan](../../../plans/current-managed-attachment-recovery/plan.md)
