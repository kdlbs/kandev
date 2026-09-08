---
id: "01-store-service-refresh"
title: "Store and service session-IP refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/requirements/session-ip-refresh.md"
---

# Task 01: Store and service session-IP refresh

## Acceptance

- `Store.TouchSession(ctx, id, lastSeen, expires, ip string)` updates the
  timestamps always and the `ip` column only when `ip` is non-empty (empty
  never clobbers a recorded value).
- `Service.ResolveSessionToken(ctx, token, ip string)` refreshes the stored
  session IP to `ip` when the touch interval has elapsed, `ip` is non-empty,
  and `ip` differs from the recorded value; same-IP, empty-IP, and
  within-interval requests never change the stored IP.
- All existing call sites of both methods in the `auth` package compile with
  the new signatures, including the single production caller
  `httpmw.ResolveRequest` (`middleware.go:62`, passing `""` — the
  `c.ClientIP()` wiring is Task 02), and the full `internal/auth` test suite
  passes. Without the middleware update, `internal/httpmw` fails to build
  and Task 01's `go test ./internal/auth/...` is unsatisfiable.

## Files likely touched

- `apps/backend/internal/auth/store/store.go` — `TouchSession` signature +
  conditional `ip` update.
- `apps/backend/internal/auth/service_credentials.go` —
  `ResolveSessionToken` gains `ip`; touch gate passes the refresh IP.
- `apps/backend/internal/auth/store/store_test.go` — new
  `TestTouchSessionIPRefresh`; existing `TouchSession` call gains `""`.
- `apps/backend/internal/auth/service_test.go` — new
  `TestResolveSessionTokenRefreshesChangedIP`; existing
  `ResolveSessionToken(ctx, token)` calls gain `""`. No fixture change
  needed: the store is already reachable from package-auth tests as
  `f.svc.store` (same access as `service_sso_test.go`'s
  `f.svc.store.GetIdentityByProviderSubject`).
- `apps/backend/internal/auth/service_sso_test.go` — existing
  `ResolveSessionToken(ctx, token)` calls gain `""`.
- `apps/backend/internal/auth/httpmw/middleware.go` — mechanical signature
  update: `svc.ResolveSessionToken(ctx, cookie)` →
  `svc.ResolveSessionToken(ctx, cookie, "")` (keeps current behavior; the
  `c.ClientIP()` wiring is Task 02).

## TDD sequence

1. RED: extend the store test first — `TestTouchSessionIPRefresh` with an
   `ip` argument and three transitions: `1.1.1.1` → touch `2.2.2.2` updates
   the stored value; `""` preserves it; and a session created with an EMPTY
   stored IP → touch `2.2.2.2` BACKFILLS it (spec scenario 4). ROW
   MAPPING (explicit — the transitions span TWO targets): transitions 1-2
   touch TARGET A (seeded IP `1.1.1.1`, seed timestamps relative to the
   first touch args); transition 3 touches TARGET B (seeded with EMPTY
   stored IP and its OWN seed timestamps — distinct from the touch args,
   using the same offsets as target A). Running all three
   touches against a single target would make the backfill assertion pass
   vacuously — the stored IP is already `2.2.2.2` when the backfill touch
   lands — silently dropping spec-scenario-4 coverage. Assert the decoy
   untouched after each of the THREE touches (both targets). Each
   transition MUST ALSO assert the timestamps moved: `LastSeenAt` and
   `ExpiresAt` equal the touch arguments after the `2.2.2.2`, `""`, and
   backfill touches. Truncate the touch ARGS to the second and use
   DISTINCT values per column — `lastSeen := now.Truncate(time.Second)`,
   `expires := lastSeen.Add(time.Hour)` (the existing `store_test.go`
   convention) — and assert `got.LastSeenAt.Equal(lastSeen)` /
   `got.ExpiresAt.Equal(expires)`. Identical args would let a
   `TouchSession` whose SET clause SWAPS the columns (`SET expires_at =
   last_seen-arg, last_seen_at = expires-arg`) write both columns to the
   same value and pass; distinct args make the swap fail deterministically.
   Advance the args per transition (`lastSeen2 := lastSeen.Add(time.Second)`,
   `expires2 := expires.Add(time.Second)`, and again for the backfill
   touch) so each transition's `Equal()` pin is FRESH — reusing one
   pair across all three transitions makes the `""` transition's timestamp
   pin vacuous (timestamps already equal the asserted args after
   transition 1).
   NEVER `==`, which additionally compares the location POINTER and any
   monotonic reading and is fragile across parses. `Equal()` compares
   instants only: it absorbs location and monotonic differences, and
   truncating the args (not the stored values) keeps the comparison valid
   even where a driver truncates sub-second precision on write. (SQLite
   alone would often pass `==` — mattn's default format parses UTC and
   `time.Now().UTC()` strips monotonic — do not let that tempt a
   "simplification".) Seed each TARGET and the DECOY with truncated,
   DISTINCT timestamps so the assertions cannot pass vacuously, all
   relative to the FIRST-transition arg pair — i.e.
   `touchArgs := (lastSeen, expires)` as defined above, the pair used by
   transition 1; the later transitions advance from it. Targets
   `LastSeenAt: touchArgs.lastSeen.Add(-time.Hour)`, `ExpiresAt:
   touchArgs.expires.Add(24*time.Hour)` — a target seeded with the same
   truncated `now` as the touch arg would leave a dropped-`last_seen_at`
   column equal to the asserted value; the decoy's `LastSeenAt`/`ExpiresAt`
   offset differently again (e.g. `touchArgs.lastSeen.Add(-2*time.Hour)` /
   `touchArgs.expires.Add(48*time.Hour)`) and its `IP` set to a value
   distinct from the touch's non-empty ip arg (e.g. `10.0.0.9`), so a
   missing-`WHERE` update that writes identical timestamps to the decoy
   still trips the untouched assertions. Mirrors `TestSessionLifecycle`'s seed-vs-touch
   distinctness (`store_test.go:106-131`; seed literal `:111-115`,
   `CreateSession` insert `:116-118`, touch `:129`).
   The "timestamps always update"
   half of the contract currently has ZERO coverage
   (`TestSessionLifecycle` touches and never asserts them), and a
   `TouchSession` that dropped the timestamp columns would pass an IP-only
   test while silently breaking sliding expiry. The
   fixture MUST ALSO insert an ADDITIONAL decoy session BEFORE any touch,
   and ALL THREE rows — target A, target B, and the decoy — MUST share ONE
   `user_id` (e.g. `u1`; the decoy's token hash distinct). There are two
   targets and one decoy, so "same user_id as the target" is only
   satisfiable this way: if target B were seeded under a different
   `user_id`, transition 3's touch could never hit the decoy and a
   `WHERE user_id = ?` regression would go undetected on the backfill
   transition. Assert, after
   each touch of the target, that the decoy's `ip`, `last_seen_at`, and
   `expires_at` are untouched — single-row fixtures cannot discriminate a
   `TouchSession` that drops `WHERE id = ?` (updating every `auth_sessions`
   row), and a decoy under a different `user_id` would let a `WHERE user_id
   = ?` regression slip; no other planned test ASSERTS on a second
   same-user row after a touch (the service-test cases create multiple
   sessions on one fixture but assert only on the just-created row).
   Read back the target JUST touched plus the decoy after EACH
   transition, by their DISTINCT token hashes via `GetSessionByTokenHash`
   — target A after transitions 1-2, target B after transition 3 (the
   fixture holds THREE rows: target A, target B, and the same-user_id
   decoy; "read both" is ambiguous and could leave one target's post-touch
   state unasserted) — never via `ListSessionsByUser` indexing: with a
   same-user_id decoy the list ordering is ambiguous (the decoy sits at
   index 1 only by virtue of its distinct seed, and the backfill session's
   `last_seen_at` ties the target's touch args) — consistent with the
   plan's "never by list index" rule. Build failure until the store
   changes.
2. Implement `TouchSession` (single UPDATE with
   `ip = CASE WHEN ? = '' THEN ip ELSE ? END`; positional arguments in
   order `lastSeen, expires, ip, ip, id` — the duplicated `ip` binds both
   the CASE test and the ELSE value); store tests GREEN.
3. RED: add the service test — build the fixture with
   `newServiceFixture(t, true)` and create the aged sessions DIRECTLY, do
   NOT call `setupEnabled` (it mints an extra `DefaultUserID` session that
   the TokenHash selection must then absorb — mirror task-02's
   no-setupAdmin recipe). For EACH case create a FRESH aged session
   (the store is reachable as `f.svc.store` from package auth — no fixture
   change; use `GenerateSessionToken()` — UNQUALIFIED: `service_test.go`
   is package `auth` and imports no `auth` alias, so the qualified form is
   a compile error (`undefined: auth`); the qualified `auth.` form is only
   correct in task-02's package-`httpmw` test — + `f.svc.store.CreateSession`
   with `CreatedAt: now`, `IP: "1.1.1.1"` — a NON-EMPTY stored IP for
   EVERY aged case (the different-IP, same-IP, and empty-request-IP
   bullets below; an empty seed would make the same-IP/empty-request-IP
   unchanged pins vacuous — only the BACKFILL case seeds an empty IP) —
   `LastSeenAt: now.Add(-2*time.Hour)` — at least 2
   minutes in the past, because the strict `now.Sub(LastSeenAt) >
   sessionTouchInterval` gate needs a seed ≥60s old or the aged cases fail
   spuriously RED as a fixture defect — `ExpiresAt: future` with the SAME
   `future := now.Add(24*time.Hour)` as the within-interval case — a
   narrow future expiry (e.g. 1m; a 1h expiry would only hazard a >1h
   stall) would let a >60s creation-to-resolve
   stall delete the session in `ResolveSessionToken` (delete before
   touch, `service_credentials.go:85-88`) and fail RED for the wrong
   reason, the same hazard every other fixture in this task pins.
   `UserID` must be `userstore.DefaultUserID`, the active row seeded by
   `userstore.Provide`; a fabricated user ID fails `activeUser` before the
   touch path runs, the same failure class as a past expiry. A fresh
   session per
   case is mandatory: the first resolve's touch bumps `last_seen_at` to now,
   so reusing one aged session across cases would throttle the later
   resolves into vacuous passes without ever reaching `TouchSession`.
   Every `svc.ListSessions` assertion MUST select its row by `TokenHash`
   (or the session ID captured at creation), never by list index: the cases
   accumulate sessions under `DefaultUserID`, and `ListSessionsByUser`
   orders by `last_seen_at DESC` with ties arbitrary — ordering is simply
   not relied on. In fact the within-interval row carries a FUTURE seeded
   `last_seen_at` and sorts ABOVE the touched rows (which carry `now`), so
   a `[0]`-indexed different-IP assertion would pick the wrong row and fail
   spuriously. Cases:
   different IP → `svc.ListSessions` shows the new IP; EMPTY stored IP +
   non-empty request IP → backfilled (spec scenario 4 — the discriminator
   for a `sess.IP != ""` guard bug that would break backfill while passing
   every other case); same IP and empty request IP → the stored IP is
   unchanged (and, being aged sessions, their `last_seen_at` ADVANCES — see
   the pin below; `expires_at` advancement is pinned by the store-level
   `TestTouchSessionIPRefresh` after every touch, so the service pins can
   focus on `last_seen_at`, the gate input); within-interval (session with `IP: "1.1.1.1"` — a
   NON-EMPTY stored IP matching the aged cases, so the "stored IP is
   unchanged" pin compares against a real recorded value; an empty stored
   IP would make the pin vacuous in the correct path — and
   `LastSeenAt: seed`, where
   `seed := now.Truncate(time.Second).Add(10*sessionTouchInterval)` —
   package `auth` CAN name the constant; truncate to the second for the
   `Equal()` assertion, per the store-test convention. `ExpiresAt: future`
   with `future := now.Add(24*time.Hour)`: pinned here so the
   aged-extension below has a defined expiry; the value is never asserted,
   only future-ness, so widening is free and consistent with the store
   test's target `ExpiresAt` offset (24h; the decoy offsets 48h). A
   narrow expiry (e.g. 1h) would let a >1h
   wall-clock stall between seed capture and the aged-extension's SECOND
   resolve delete the session (`ResolveSessionToken` deletes expired
   sessions before touching) and fail RED for the wrong reason — the same
   failure class a past expiry already warns about. With 24h the binding
   constraint becomes the aged `last_seen_at` (always open at D+2h > 60s).
   The strict
   `now.Sub(LastSeenAt) > sessionTouchInterval` gate keeps a seed 10
   intervals in the future closed for ~11 minutes (10 intervals of future
   seed plus one interval of strict-`>` threshold). Precisely, the gate
   opens when wall-clock elapsed `D` satisfies `D > 660s − f`, where
   `f ∈ [0,1s)` is the fractional second the truncation dropped from the
   seed, i.e. between 659s and 660s. The stall window is bounded; the
   practical risk is negligible, the test runs in ms. — resolve with a
   DIFFERENT IP (`2.2.2.2`),
   asserting `ok == true` FIRST (the pins pass vacuously if
   `ResolveSessionToken` returns false for the future-seed session, a
   fixture defect or a buggy impl rejecting future `last_seen_at` rows) →
   the stored IP is unchanged (spec scenario 5's observable, exercised
   directly) AND `LastSeenAt` STILL equals the truncated seed (capture it
   before insert; assert via `Equal()`; never a literal smaller seed,
   which would contradict the inserted value and go RED against a correct
   implementation). Read both pinned fields back via
   `GetSessionByTokenHash` or `svc.ListSessions` — either is touch-free
   and equivalent here; state the choice in the test so the mechanism is
   unambiguous. The different-IP resolve makes the always-touch
   variants self-revealing: a service that touches without the gate either
   writes `2.2.2.2` (the IP-unchanged assertion catches it) or bumps
   `last_seen_at` from the future seed to `now` (the pin catches it) — the
   last "no per-request writes" hole. Extend the same
   within-interval session to pin spec scenario 5's FULL transition
   (required, not optional — without it the scenario's second half is
   untested and the plan's scenario map overclaims): after
   the unchanged assertions, age the row via
   `f.svc.store.TouchSession(ctx, id, now.Add(-2*time.Hour), future, "")`
   and resolve AGAIN with the same different IP → the stored IP is now
   updated ("updates on the next request after the interval"). The
   aged-session SAME-IP (or empty-IP) case MUST ALSO assert via
   `svc.ListSessions` that `last_seen_at` advanced past the aged value AND
   is NOT after `now.Add(time.Minute)`, where `now` is RE-CAPTURED at
   assertion time — reusing the seed-capture `now` (from `LastSeenAt:
   now.Add(-2*time.Hour)`) would make a correct implementation fail
   spuriously on a >60s stall, the same time-sensitivity class the design
   hardens against elsewhere — AND that `ExpiresAt` is AFTER the seed's
   future expiry (e.g. `got.ExpiresAt.After(seedExpires.Add(time.Hour))`
   with `seedExpires := future` captured at insert): the correct
   implementation writes resolve-now + `SessionTTL()` (`now + 720h` in the
   fixture), while a no-sliding-extension bug (passing the seed's
   `ExpiresAt` through unchanged) fails this. This is the SOLE
   service-level pin on expiry EXTENSION — task-02's read-back 401 only
   catches the `expires = now` variant, and the store test pins fidelity
   to the passed arg, not the service's computation of it. The lower
   bound pins the
   "timestamps still touch" half of the service contract on the common
   production path (a buggy gate like
   `if ip != "" && ip != sess.IP { TouchSession(...) }` stalls sliding
   expiry and fails the lower bound), while the upper bound pins the ARG
   ORDER of the service's `TouchSession` call: a buggy
   `TouchSession(ctx, id, now.Add(s.SessionTTL()), now, refreshIP)` (args
   swapped) writes `last_seen_at = now + TTL` and fails the upper bound,
   since a correct implementation writes the resolve-time `now`. Existing
   `ResolveSessionToken` calls need the new argument, so this is a build
   failure until the service changes.
4. Implement `ResolveSessionToken(ctx, token, ip)` with the touch-gate
   refresh; update every `auth` package call site plus the production caller
   `httpmw.ResolveRequest` (passing `""`); service tests GREEN.
5. Run the full auth suite: `cd apps/backend && go test ./internal/auth/... -count=1`.

## Verification

```bash
cd apps/backend && go test ./internal/auth/store -run '^TestTouchSessionIPRefresh' -count=1 && go test ./internal/auth -run '^TestResolveSessionTokenRefreshesChangedIP' -count=1 && go test ./internal/auth/... -count=1
```

## Dependencies

None.

## Parallelism

`sequential`.

## Inputs

- Spec **What** (refresh on touch when different; empty never overwrites;
  login-time recording unchanged), **Scenarios** 1-6, **Failure modes**
  (empty IP, failed touch write, bounded refresh). The "touch write fails
  → resolution still succeeds" failure mode is DELIBERATELY untested:
  `Service.store` / `Deps.Store` are the concrete `*store.Store` type, so
  an error-injecting wrapper would require an interface change for
  test-only purposes. The contract holds by construction — the existing
  `_ = s.store.TouchSession(...)` best-effort call already ignores touch
  errors (unchanged behavior). Document, don't test.
- Plan "Backend → Store / Service" sections for the exact signatures and
  SQL.
- `apps/backend/internal/auth/service_credentials.go:76-97` (current
  `ResolveSessionToken` and touch gate at `:93-95`), `store.go:134-140`
  (current `TouchSession`), `store_test.go` (`TestSessionLifecycle` — touch
  call at `:129`), `service_test.go` (`newServiceFixture` builds
  `authStore` before `NewService`; the store is reachable as `f.svc.store`
  — no fixture struct change needed).

## Docs to update

- `store.go:134` — `TouchSession`'s comment "updates activity timestamps
  (sliding expiry)" becomes inaccurate once the IP is conditionally
  refreshed; update to "updates activity timestamps (sliding expiry) and
  refreshes the stored client IP when a non-empty one is passed".
- `service_credentials.go:75` — extend `ResolveSessionToken`'s comment:
  the passed `ip` refreshes the session IP on the touch path only when
  non-empty and differing from the recorded value.

## Risks

- Gin's default trusted-proxies behavior is irrelevant here: the service
  receives whatever IP the caller passes; `c.ClientIP()` wiring is Task 02.
- The within-interval case must use a fresh `LastSeenAt` so the assertion is
  about the throttle, not about the IP comparison.

## Output contract

Report the RED failure(s), GREEN result, exact files changed, the updated
call sites (enumerated: `ResolveSessionToken` — service_test.go:108,144,
152,184,189,192,220,264 (8, gain `""`), service_sso_test.go:68,114 (2,
gain `""`), httpmw/middleware.go:62 (1, mechanical `""`);
`TouchSession` — store_test.go:129 (1, gains `""`)), remaining risks, and
task/plan status sync. Record every exact
command and outcome in `## Results`.

## Results

RED (TDD step 1, store): build failure on the new signature —
`vet: internal/auth/store/store_test.go:129:88: too many arguments in call to s.TouchSession`.

GREEN (store): `cd apps/backend && go test ./internal/auth/store -run '^TestTouchSessionIPRefresh' -count=1` → `PASS`.

RED (TDD step 3, service): build failure until `ResolveSessionToken` gains the
`ip` parameter (the new `TestResolveSessionTokenRefreshesChangedIP` calls the
3-arg form).

GREEN (service): `cd apps/backend && go test ./internal/auth -run '^TestResolveSessionTokenRefreshesChangedIP' -count=1 -v` → `PASS`, all 5 sub-cases
(different IP, backfill, same IP + touch pins, empty request IP + touch pins,
within-interval deferral + post-interval transition).

Full suite: `cd apps/backend && go test ./internal/auth/... -count=1` → all
packages pass (`auth`, `httpapi`, `httpmw`, `store`; `authn` no tests).

Files changed: `store/store.go` (TouchSession signature + conditional-IP
UPDATE + comment), `service_credentials.go` (ResolveSessionToken signature,
touch-gate refresh, comment), `store/store_test.go` (new
`TestTouchSessionIPRefresh`, `TestSessionLifecycle` touch gains `""`),
`service_test.go` (new `TestResolveSessionTokenRefreshesChangedIP`; 8
`ResolveSessionToken(ctx, token)` call sites gain `""`), `service_sso_test.go`
(2 call sites gain `""`), `httpmw/middleware.go` (1 call site, mechanical
`""`).

Remaining risks: none for this task; the `c.ClientIP()` wiring is Task 02.
