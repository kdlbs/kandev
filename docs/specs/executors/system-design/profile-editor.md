---
status: current
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
| AC-EXECUTORS-PROFILE-EDITOR-001.8 through .12 | Partial-save script persistence |

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
baseline readiness, deletion handling, and store updates. Navigation unification
requires no migration or backend change. Navigation itself never saves or deletes data.

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

## Partial-save script persistence

Ordinary REST PATCH and WebSocket profile requests in
`internal/task/handlers/executor_profile_handlers.go`, and the `executor_profile`
settings-domain operation in `internal/backendapp/settings_domain_operations.go`,
already carry optional script pointers into `Service.UpdateExecutorProfile` in
`internal/task/service/service_resources.go`. Nil means omission; a non-nil
pointer to an empty string means clear. Existing JSON null decoding remains
nil; this repair adds no null wire contract. The canonical web editor submits
both scripts explicitly, so preserving omissions does not resolve stale full
editor drafts.

The service currently loads a profile snapshot, applies supplied fields, and
calls a full-row repository update. That snapshot must remain available for
existing authorization, Kubernetes validation, Sprites token merging, and other
field behavior. For ordinary built-in saves only, carry the original two script
pointers separately to storage instead of treating snapshot scripts as intent.

Use a small `models.ExecutorProfileScriptIntent` in a focused model file and a
required `ExecutorRepository.UpdateExecutorProfileWithScriptIntent(ctx,
profile, intent) error` method. The method receives the existing prepared model
for all other fields. It refreshes only that model's committed script pair and
timestamp after a successful commit. Required aggregate test doubles implement
the seam explicitly in focused files; unsupported test doubles fail closed.
There is no production fallback to a full-row write and no generic patch API.

### Atomic update and acknowledgement

The SQLite repository package also supports PostgreSQL. The new method uses the
existing writer pool, parameter rebinding, JSON serialization, and UTC timestamp
source. Within a short native transaction, the actual UPDATE writes the existing
non-script assignments and timestamp, and includes a script assignment only
when its pointer is present. There are exactly four script-presence combinations.
Omitted script columns remain untouched by the statement.

Capture `prepare_script`, `cleanup_script`, and `updated_at` with UPDATE
RETURNING into local values. Exhaust and close result rows, check iteration and
close errors, then commit. Publish those values to the caller's model only after
commit succeeds. Defer rollback for unsuccessful paths. No later profile reread
supplies the acknowledgement: another save could have committed by then. This
guarantee covers the script pair and this save's timestamp; unrelated fields
retain existing snapshot behavior and are not promised a global coherent view.

Use the established [SQLite writer transaction boundary](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).
Check context before admission and after a returned transaction; drain/join
cancellation and release the connection on every outcome. No instant busy-wait
cancellation guarantee is added. PostgreSQL performs no current-row read before
its atomic UPDATE, so the UPDATE itself acquires its row lock. There is no
read-modify-write critical section requiring an added advisory lock. Actual
physical row-wait tests must prove it retains a holder's committed omitted
script and still applies an explicit script after the wait.

Serialization, query, scan, rows, and commit errors return failure and suppress
the service's success event. Missing rows preserve the ordinary missing-profile
error behavior. This path adds no schema, timestamp algorithm, retry loop, or
new event shape. Commit errors do not authorize a success acknowledgement or an
assumption that an uncertain commit rolled back.

### Compatibility and launch projection

`UpdateExecutorProfileIfUnmodified` remains the full exact timestamp CAS used
when `ExpectedUpdatedAt` is present. Config-mode MCP obtains that version in
`internal/mcp/handlers/config_executor_handlers.go`; it remains guarded for all
updates. Legacy `UpdateExecutorProfile` stays a full replacement. Plugin remote
profiles return through `updatePluginExecutorProfile` before the new seam and
retain their local-script restrictions. Kubernetes and remote Docker admin
checks, Kubernetes configuration validation, global secret-reference admission,
and Sprites token/env merge behavior precede storage as before.

`Executor.applyProfile` in `internal/orchestrator/executor/executor_state.go`
loads stored `PrepareScript` into `SetupScript`, `CleanupScript` into the
cleanup configuration, and nonempty cleanup into lifecycle metadata. Verify
that real projection with the saved row. It does not establish remote execution
or mutate an existing resource. The [public runtime guide](../../../public/executors.md#script-behavior-is-runtime-specific)
continues to own execution timing, including terminal archive/delete cleanup
for SSH and Sprites and no executor cleanup execution for Local or Docker.

### Verification boundary

Permanent tests must reproduce the four disjoint interleavings with independent
real SQLite services, stores, and physical connections, gating only the captured
profile read. Assert stored values, responses, and success-event script pair and
timestamp. Add explicit-clear, both-present, same-script commit order, own-commit
acknowledgement, exact/legacy/plugin/admission, cancellation, rollback, and
missing-row controls. Use fixed persisted timestamps for exact-CAS fixtures.

Registered REST and WS handlers and the actual settings-domain operation need
real database integration evidence. A separate PostgreSQL behavior test must
observe distinct backend PIDs and actual statement row blocking before holder
release; environment skipping is not delivery evidence. Scoped store conformance
and actual native Windows RUN/PASS complete persistence portability coverage.
The repair changes no UI layout, copy, navigation, or touch behavior; backend
integration evidence satisfies this data-only mobile boundary without new
browser tests or frontend builds.

## Implementation plans

- [Unified profile editor](../../../plans/executor-profile-editor-unification/plan.md)
- [Preserve scripts during partial saves](../../../plans/executor-profile-script-preservation/plan.md)
