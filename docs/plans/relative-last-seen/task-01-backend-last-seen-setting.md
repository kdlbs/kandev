---
id: "01-backend-last-seen-setting"
title: "Backend last_seen_display setting"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/relative-last-seen.md"
---

# Task 01: Backend `last_seen_display` setting

## Acceptance

- `UserSettings` model, DTO, store default, service patch, controller mapping, boot-state
  mapping, AND the two hand-built store codec paths and the hand-built WS event snapshot all carry
  `last_seen_display`; the store default and any unknown value normalize to `"absolute"`.
- The two manual JSON codec paths in `store/sqlite.go` are covered explicitly: the
  `marshalUserSettingsPayload` map entry, the `scanUserSettings` payload struct field, and the
  scan assignment with `NormalizeLastSeenDisplay` coercion. A model/DTO-only change silently drops
  the field on write and falls back to the default on read.
- The PATCH write is expected-revision CAS: `UpsertUserSettingsPreservingTaskCreateLastUsed` gains
  `expectedRevision` and its UPDATE gains `AND settings_revision = ?` (SQLite + Postgres paths),
  returning a revision-conflict sentinel on zero rows. Zero rows is ambiguous, so the repo
  existence-checks the user after a zero-row conditional UPDATE: missing user → the existing
  user-not-found error (preserved for direct callers/tests); present → `ErrUserSettingsRevisionConflict`
  (both drivers tested, including the RETURNING zero-row case).
- ALL full-blob writers route through one bounded read-apply-CAS-retry helper in the service
  (read → apply patch → upsert with the read revision → on conflict re-read and re-apply the
  ORIGINAL patch, ~3 attempts): `UpdateUserSettings` AND `ClearDefaultEditorID` (service.go:883-900
  currently upserts once; without the shared helper it either fails to clear on conflict or
  bypasses CAS and can still overwrite `last_seen_display`). Exactly one event publication after
  the successful final write. Every upsert callsite is updated: the store interface, both driver
  implementations, the controller/service test fakes, and the direct sqlite_test.go calls
  (~31/96/151/1274/1323/1379, passing the revision from the preceding read; the existing
  concurrent test at ~150 asserts conflict-and-retry semantics).
- Barrier-based concurrent tests prove it: two PATCHes (one with `last_seen_display: "relative"`,
  one unrelated omitted-field) started from the same initial revision — the final row contains
  both fields, and the losing write merged via the retry loop rather than reverting
  `last_seen_display`; and a concurrent `ClearDefaultEditorID` + settings PATCH (the Clear applies
  `default_editor_id` while the PATCH applies `last_seen_display`, different fields) — the final
  row contains BOTH effects (editor cleared AND `last_seen_display` set), and TWO events are
  published, one per successful write with distinct revisions (each CAS winner publishes its own
  successful write; the loser retries and publishes its own — the helper contract is exactly one
  event per successful final write, so two successful writes emit two events; a one-event outcome
  is only valid in the two-phase no-op scenario above).
- The shared helper's apply/no-op contract is explicit and covers the task-create patch: the
  helper signature is `updateUserSettingsCAS(ctx, apply func(*models.UserSettings) (bool, error),
  taskCreatePatch *models.TaskCreateLastUsed)`, where `apply` is evaluated against EVERY freshly
  read model, captures immutable inputs (the original `editorID`), and returns `applied=false` for
  a no-op. The IMMUTABLE `taskCreatePatch` (extracted once from the original request, as
  `UpdateUserSettings` does today at service.go:198-202 with `taskCreateLastUsedPatchEmpty`) is
  passed to EVERY CAS attempt as the upsert's separate patch argument, so a retry re-applies the
  same merge against the fresh row. `applied` is true when the callback mutated the model OR the
  task-create patch is non-empty (the json merge is still a write even when the blob is otherwise
  unchanged — a task-create-only PATCH must NOT be classified no-op and dropped). The helper
  writes and publishes ONLY when applied — a no-op means NO write and NO event.
  `ClearDefaultEditorID`'s existing `settings.DefaultEditorID != editorID` early-return
  (service.go:883-902) moves inside the callback and is re-evaluated per attempt, so a competing
  PATCH that changes `default_editor_id` to another value on a retry read makes the Clear a no-op.
- Service retry test for the task-create path: a task-create-only PATCH (no other field changes)
  forced into a revision conflict survives via the retry loop, merges with the concurrent winner's
  row, increments the revision exactly once on its own successful retry (the winner already
  bumped R to R+1, so the final revision is R+2 — two total increments across two successful
  writes), and emits the resulting snapshot event (this pins the existing public PATCH behavior at
  service.go:197-206 / service_test.go:1257-1283 under the CAS rewrite).
- Barrier test for the no-op path FORCES a real CAS conflict and callback re-evaluation (the
  earlier fresh-read-only ordering could pass even with the editorID check left OUTSIDE the shared
  helper, because Clear's first read already saw the new value): initial `DefaultEditorID =
  "old-editor"` at revision R. PHASE 1: release Clear's initial `GetUserSettings` (its read
  completes seeing `"old-editor"`), then BLOCK its upsert/CAS attempt on a channel. PHASE 2:
  release the full PATCH (`default_editor_id: "new-editor"`), wait for its commit + event (row now
  R+1). PHASE 3: release Clear's stale upsert — it conflicts (expected R vs current R+1), the
  retry loop fresh-reads the row (sees `"new-editor"`), the callback re-evaluates with the
  immutable `"old-editor"` and returns `applied=false` → NO Clear upsert and NO Clear event.
  Assert: final row keeps `"new-editor"` at R+1; Clear's read count >= 2 (initial + retry
  re-read); `clearUpsertAttempts == 1` (the forced stale CAS invocation that conflicts — the
  counter records invocations, so a zero total would contradict the required conflict);
  `clearSuccessfulUpserts == 0` (the stale attempt never commits); `clearEvents == 0`; exactly one
  event total, with the PATCH's revision. Name the fake's counters accordingly (attempts vs
  successful commits). KEEP a separate deterministic fresh-read no-op test (Clear starts only
  after the PATCH committed; immediate no-op, no conflict) as a distinct scenario — both prove
  different halves of the contract.
- The existing direct repository tests are MIGRATED to the CAS contract, not just listed, and the
  migrated tests MOVE to new focused files: `TestSQLiteRepositoryAssignsUniqueAtomicSettingsRevisions`
  (~120-180) starts two candidates from one read and currently asserts both writes succeed with
  revisions [1,2]; it becomes one success + one `ErrUserSettingsRevisionConflict`, with the
  fresh-read/reapply retry behavior covered by the service barrier tests (the direct repo layer has
  no retry). The stale-blob merge tests — Postgres round trip (staleSettings upsert ~1274), SQLite
  stale write after `UpdateTaskCreateLastUsed` (~1323), and the SQLite patch path (~1379) — re-read
  the CURRENT revision before the direct upsert where their purpose is the `task_create_last_used`
  json merge (the merge assertions must keep passing), or assert `ErrUserSettingsRevisionConflict`
  where staleness is the point. MOVING them (deletion from `sqlite_test.go` + migrated copy in the
  new file) is mandatory: `sqlite_test.go` (1,466 lines) and `service_test.go` (1,820 lines) are
  already over Revive's 800-effective-line limit, which applies to test files
  (`apps/backend/AGENTS.md` "Code-quality limits", `.golangci.yml` revive file-length-limit; the
  `_test.go` exclusions do not disable revive), so ANY added code there fails the changed-file
  lint. New tests land in new files: `apps/backend/internal/user/store/sqlite_last_seen_cas_test.go`
  (codec round-trip, CAS conflict/zero-row, migrated repo tests) and
  `apps/backend/internal/user/service/user_settings_cas_test.go` (CAS retry, barrier tests,
  no-op contract, event-count assertions); the controller mapping test may stay in
  `controller_test.go` (under the limit).
- `publishUserSettingsEvent` in `service/service.go` includes the normalized `last_seen_display`,
  so the WS `user.settings.updated` payload reaches every other tab (the initiating tab alone
  getting the PATCH response is not cross-tab sync).
- NO writer-id plumbing: the publisher signature stays `publishUserSettingsEvent(ctx, settings)`
  (no direct test callsite changes; `app_status_bar_visibility_test.go:44` and
  `service_test.go:1312, 1333, 1354, 1377, 1399, 1417` keep their current shape). Pending-selection
  protection is entirely client-side (pending-write gating in task 03), because the event is a
  full snapshot that any user-settings write re-publishes and no writer-id scheme can tell whether
  a foreign snapshot changed `last_seen_display` or merely re-asserted the old value.
- Covered by an UpdateUserSettings integration test asserting the PUBLISHED event map (not just a
  direct publisher test): a PATCH with `last_seen_display: "relative"` produces the normalized
  `last_seen_display` in the published snapshot.
- The controller test asserts `req.LastSeenDisplay` reaches the service request (the manual
  adapter can silently drop the field; the REST handler and `wsUpdateUserSettings` share this one
  controller path, handlers.go:113).
- A PATCH with `last_seen_display: "relative"` persists and reads back as `"relative"`; a PATCH
  with an invalid value is rejected by the service; a PATCH omitting the field leaves it untouched.
- `FromUserSettings` emits the canonical value via `NormalizeLastSeenDisplay` (matching every other
  enum-like setting at the API boundary), so a model assembled outside the store cannot leak an
  invalid wire value.
- The boot payload includes `lastSeenDisplay` keyed from the DTO.

## Verification

```bash
make -C apps/backend test
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/store/sqlite.go` (+ NEW `store/sqlite_last_seen_cas_test.go`:
  codec round-trip tests (stored JSON, unknown coercion, later-PATCH update), CAS conflict and
  zero-row driver tests, and the MIGRATED repo tests moved out of `sqlite_test.go`)
- `apps/backend/internal/user/service/service.go` (+ NEW `service/user_settings_cas_test.go`:
  the shared-CAS-retry tests, barrier tests (PATCH/PATCH, Clear/PATCH, two-phase no-op), the
  published event-map integration test, and the new synchronized CAS fakes; the existing
  publisher tests at `service_test.go:1312-1417` and `app_status_bar_visibility_test.go:37-52`
  are UNCHANGED and stay where they are)
- `apps/backend/internal/user/dto/dto.go` (+ `dto_test.go` regression: unknown model value emits
  canonical `"absolute"`)
- `apps/backend/internal/user/controller/controller.go` (+ `controller_test.go`: focused mapping
  test asserting `req.LastSeenDisplay` reaches the service request, mirroring the existing enum
  mapping tests; the controller is a manual field-by-field adapter, so an omission compiles and
  silently no-ops)
- `apps/backend/internal/backendapp/boot_state_routes.go` (+ `boot_state_routes_test.go`)

The concurrent CAS service tests use a DEDICATED synchronized fake (in `user_settings_cas_test.go`),
NOT the existing `recordingUserRepository`/`recordingEventBus` fakes: those return one shared
mutable `getSettings` pointer on every read (service_test.go:1741-1750), mutate/read unprotected
fields (1753-1771), and append to `publishedEvents` unsynchronized (1793-1801), so concurrent
callers race under `go test -race`, one candidate can observe the other's patch before the CAS
decision, and event counts are unsafe. The new fake: clones settings for every read, models
revision-conditional writes, records reads/upserts through synchronized state/channels, and uses a
thread-safe event recorder; channel barriers gate phase-1 commit/event observation before releasing
phase 2 (no timing/polling).

## Dependencies

None.

## Inputs

- Spec "What", "Failure modes" (normalization), "Out of scope" (JSON blob, no schema migration)
- Existing enum precedent: `applyChangesPanelLayout`, `NormalizeLspStatusLocation`,
  `changesPanelLayout(...)` boot mapping

## Output contract

Return a compact handoff capsule with acceptance status, exact test command/results, risk tags,
uncertainties, and set this task to `done`.
