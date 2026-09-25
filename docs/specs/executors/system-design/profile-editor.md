---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
---

# Executor profile editor design

## Purpose and boundaries

The existing complete editor at `/settings/executors/:profileId` owns profile
editing. All profile navigation converges on this editor. Executor connection
pages retain their existing routes and behavior.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| AC-EXECUTORS-PROFILE-EDITOR-001.1, .2, .7 | Components and navigation |
| AC-EXECUTORS-PROFILE-EDITOR-001.3, .4 | Bookmark compatibility |
| AC-EXECUTORS-PROFILE-EDITOR-001.5 | State and permissions |
| AC-EXECUTORS-PROFILE-EDITOR-001.6 | Phone composition |

## Components and navigation

`apps/web/app/settings/executors/[profileId]/page.tsx` remains the single
editor implementation. Its `ProfileEditPage` resolves profile ownership from
the hydrated executor store. Its existing section components determine which
controls apply to the executor type.

`executorProfileSettingsPath` in
`apps/web/lib/settings/executor-settings-routes.ts` takes only `profileId` and
returns `/settings/executors/${encodeURIComponent(profileId)}`. Profile
navigation does not depend on executor type. Connection helpers remain separate.

Every production caller uses this helper, including the hub, settings tree,
profile list, task disclosure, settings discovery, creation success navigation,
and task-creation credential links. Discovery appends its existing fragments.

## Bookmark compatibility

`LegacyExecutorSettingsRoute` handles both existing executor-scoped route shapes.
For an explicit profile ID, it first finds the named executor and confirms that
the profile belongs to that executor. A valid pair renders `SettingsRedirect`
to the canonical profile route. It never mounts an editable legacy form.

An invalid pair renders the existing localized unavailable-profile message and
a recovery link. It does not fall back to the first profile or resolve the
profile globally. The reduced page can remain as a small compatibility wrapper
or unavailable-state component, but contains no form state or persistence logic.

`SettingsRedirect` already uses router replacement and preserves query
parameters and fragments through `resolveSettingsRedirect`. Reuse it without
changing generic redirect behavior. Store updates can resolve a temporarily
missing pair. An unavailable state must not cause an eager redirect.

Executor-only URLs retain their current behavior: Kubernetes resolves its first
profile or its connection recovery page. Other executor types retain their
connection editor. An explicit invalid Kubernetes profile must not use the
executor-only fallback.

## State and permissions

The canonical editor retains its current save contributors, serialization,
baseline readiness, deletion handling, and store updates. No migration or
backend change is required. Navigation itself never saves or deletes data.

Kubernetes members retain read-only controls. Docker build controls retain
their administrator checks. The URL ownership check prevents accidental profile
substitution and does not replace backend authorization.

The canonical editor owns the resulting header, recovery, and delete navigation.
The old editor's separate Cancel button and save contributor disappear. The
settings shell continues to own unsaved-change prompts and discard behavior.

## Phone composition

The existing settings index and executor hub provide direct phone navigation.
`mobile-settings-sidebar.spec.ts` confirms that phones do not expose the desktop
settings sidebar. No new menu or drawer is required.

The nearest shipped form is the canonical profile editor, covered by
`mobile-executor-profile-spacing.spec.ts`. The curated direct-navigation
pattern in `components/kanban-with-preview.tsx` supports this choice: the
profile is a primary destination with a long form, not a temporary picker.

The settings content region remains the single page scroll owner. Cards retain
their existing vertical rhythm. The shared floating save control remains the
primary action and retains safe-area clearance. Touch controls retain their
existing phone sizing. Phone tests cover navigation, editing, save, reload,
bookmark recovery, and zero horizontal document overflow.

## Verification boundaries

Route-helper tests cover supported executor callers and encoded profile IDs.
Component tests cover bookmark ownership, missing records, store hydration,
redirect suffixes, and unchanged executor-only routes. Browser tests exercise
the real navigation controls and complete editor together.

Mock Docker build responses follow the existing persistence E2E pattern. This
repair verifies access to the controls, not container runtime execution.

## Implementation plans

- [Unified profile editor](../../../plans/executor-profile-editor-unification/plan.md)
