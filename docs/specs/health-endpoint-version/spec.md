---
status: building
created: 2026-08-07
owner: nova28
---

# Health Endpoint — Surface the Running Version

## Why

`GET /health` is the endpoint every operator already polls. Kubernetes liveness and
readiness probes (`k8s/deployment.yaml:39,47`), the CLI (`apps/cli/src/health.ts`),
the Go launcher (`internal/launcher/health.go:44`), the Homebrew service wrapper
(`scripts/release/kandev.rb`), the Tauri desktop shell
(`apps/desktop/src-tauri/src/backend.rs:555`), and the Playwright fixture
(`apps/web/e2e/fixtures/backend.ts:461`) all hit it. It answers "is this process up?"
but not "*which build* is up?", so an operator watching a rollout, a bad canary, or a
stuck upgrade cannot tell from their monitoring which version is answering.

The version is not reachable from an equivalent place today. `GET /api/v1/system/info`
does return it, but that route is **not** on the unauthenticated allowlist
(`internal/auth/httpmw/middleware.go:85-99`), while `/health` **is**. Once auth is
enabled, a monitoring system must be issued a credential purely to read a version
string. Adding the field to `/health` closes that gap for every unauthenticated prober
without weakening any existing boundary.

**Correction to the originating request.** The request stated that `kandev_version` is
"already on `/system/info`". That is not accurate, and the difference matters because it
determines the field name this spec freezes. `GET /api/v1/system/info` returns a field
named **`version`** (`internal/system/info/info.go:19`), not `kandev_version`. The
identifier `kandev_version` does exist in this codebase, but for two unrelated things:
the `kandev_meta.kandev_version` database key that drives upgrade-backup safety
(`internal/persistence/meta.go:83`, ADR `0008-db-upgrade-safety.md`), and the
share-snapshot payload field (`internal/task/share/snapshot.go:19`). Both names
therefore had prior art. The name was resolved by explicit decision — see
[Decisions](#decisions).

## Current State (verified 2026-08-07)

There are **two** distinct health endpoints. This spec changes exactly one of them.

| Endpoint | Registered at | Auth | Purpose | In scope |
|---|---|---|---|---|
| `GET /health` | `internal/backendapp/helpers.go:675` | Public (`middleware.go:87`) | Readiness probe for orchestrators, CLI, desktop, launcher | **Yes** |
| `GET /api/v1/system/health` | `internal/health/handler.go:13` | Authenticated | Diagnostics/issue list behind Settings > System > Status | No |
| `GET /api/v1/system/info` | `internal/system/system.go:175` | Authenticated | Build + runtime detail for System > About | No |
| WS action `health.check` | `internal/gateway/websocket/handler.go:74` | Session-bound | In-band WS liveness echo | No |

The current `/health` handler in full (`helpers.go:675-684`):

```go
p.router.GET("/health", func(c *gin.Context) {
    if !ready.Load() {
        c.JSON(http.StatusServiceUnavailable, gin.H{"status": "starting", "service": "kandev"})
        return
    }
    if token := desktopHealthToken(); token != "" {
        c.Header(desktopHealthTokenHeader, token)
    }
    c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "kandev", "mode": "websocket+http"})
})
```

`ready` is an `atomic.Bool` (`main.go:171`) flipped only after all routes are mounted,
the agent registry is seeded, and the listener is accepting. Before that the handler
returns 503 so probers keep polling instead of racing ahead onto unmounted routes.

**The version value is already in scope at this exact line.** `routeParams` carries a
`version string` field (`helpers.go:545`), populated from the package-level `Version` at
`main.go:1830`, which is set by `setBuildInfo` from the ldflag-injected
`cmd/kandev/main.go:13`. It is already consumed nearby at `helpers.go:1180`. No new
plumbing, DI, or constructor change is required.

`Version` defaults to the literal `"dev"` (`cmd/kandev/main.go:13`,
`internal/backendapp/main.go:129`), and `setBuildInfo` only overwrites it when the
injected value is non-empty (`main.go:251`). The value is therefore **never the
empty string** at runtime.

**No test asserts the `/health` response body today.** A repo-wide search for the
current payload finds exactly two occurrences: the handler itself and a public doc
(`docs/public/operations.md:70`). This spec's acceptance criteria close that gap.

## What

- `GET /health` SHALL include the running Kandev version in its JSON response body.
- The field SHALL be named **`version`**, matching `GET /api/v1/system/info`.
- The field SHALL be present in **both** the ready (200) and not-ready (503) responses,
  so an operator can identify the build of a backend that is stuck starting.
- The field value SHALL be the same string `GET /api/v1/system/info` reports as
  `version` for the same running process.
- The field SHALL be served to unauthenticated callers, exactly as the rest of the
  `/health` payload already is. This is a deliberate, accepted disclosure — see
  [Security](#security-and-permissions).
- Every existing field (`status`, `service`, `mode`) SHALL retain its current name,
  value, and semantics. This is a purely additive change.
- The existing HTTP status semantics (200 when ready, 503 while starting) SHALL be
  unchanged.
- The desktop health-token response header SHALL continue to be set on the 200 path only,
  unchanged.

### API surface

Ready (HTTP 200), current → new:

```json
{"status":"ok","service":"kandev","mode":"websocket+http"}
{"status":"ok","service":"kandev","mode":"websocket+http","version":"1.2.3"}
```

Starting (HTTP 503), current → new:

```json
{"status":"starting","service":"kandev"}
{"status":"starting","service":"kandev","version":"1.2.3"}
```

On an un-stamped local build the value is `"dev"`:

```json
{"status":"ok","service":"kandev","mode":"websocket+http","version":"dev"}
```

## Acceptance Criteria

Numbered, pass/fail, EARS-form. Each is directly testable against the handler.

**Ready-path payload**

1. WHEN a `GET /health` request is received AND the readiness flag is set, the backend
   SHALL respond with HTTP 200 and a JSON body containing a `version` key.
2. WHEN a `GET /health` request is received AND the readiness flag is set, the backend
   SHALL respond with a body whose `version` value equals the process's configured build
   version string.
3. WHEN a `GET /health` request is received AND the readiness flag is set, the backend
   SHALL respond with a body in which `status` equals `"ok"`, `service` equals
   `"kandev"`, and `mode` equals `"websocket+http"`.
4. WHEN a `GET /health` request is received AND the readiness flag is set, the response
   body SHALL contain exactly the keys `status`, `service`, `mode`, and `version` and no
   others.

**Starting-path payload**

5. WHILE the readiness flag is unset, a `GET /health` request SHALL receive HTTP 503.
6. WHILE the readiness flag is unset, a `GET /health` request SHALL receive a JSON body
   containing a `version` key whose value equals the process's configured build version
   string.
7. WHILE the readiness flag is unset, a `GET /health` response body SHALL contain exactly
   the keys `status`, `service`, and `version`, with `status` equal to `"starting"` and
   `service` equal to `"kandev"`.

**Value correctness**

8. WHERE the binary is built without version ldflags, a `GET /health` response SHALL
   report `version` as `"dev"`.
9. WHERE the binary is built with a version ldflag, a `GET /health` response SHALL report
   `version` as that injected value.
10. The `version` value returned by `GET /health` SHALL be byte-identical to the
    `version` value returned by `GET /api/v1/system/info` from the same running process.
11. A `GET /health` response SHALL NOT report `version` as an empty string, absent, or
    JSON `null` under any readiness state.

**Access and compatibility**

12. WHEN authentication is enabled AND a `GET /health` request arrives with no credential,
    the backend SHALL respond with the full body including `version`, and SHALL NOT
    respond 401 or 403.
13. The change SHALL NOT alter the response status code, headers, or the `status` field
    value for any readiness state, so that an existing prober asserting only HTTP status
    or `status` continues to pass unmodified.
14. WHEN authentication is enabled AND a `GET /api/v1/system/info` request arrives with no
    credential, the backend SHALL continue to reject it, confirming this change does not
    widen the allowlist.

**Documentation**

15. `docs/public/operations.md` SHALL show the `/health` example body including the
    `version` field, so the documented payload matches the served payload byte-for-byte.

## Security and Permissions

`/health` is on the unauthenticated allowlist (`internal/auth/httpmw/middleware.go:87`,
which carries a pinning test at `middleware_test.go:162`). Adding `version` therefore
publishes the exact running version to anyone who can reach the port.

This is a **deliberate accepted tradeoff**, decided explicitly (see
[Decisions](#decisions)). The rationale: the stated goal is monitoring and alerting, and
external monitors are precisely the unauthenticated callers. Gating the field behind auth
would defeat the purpose for exactly the security-conscious deployments most likely to run
real alerting. Version disclosure on a health endpoint is a mild and widely accepted
exposure; it aids version fingerprinting but grants no access, leaks no tenant data, and
reveals nothing an attacker could not infer from behavior.

This spec introduces no new permission concept and changes no existing one:

- The public allowlist is **not** modified. `/health` was already public.
- `/api/v1/system/info` and `/api/v1/system/health` remain authenticated. AC-14 pins this.
- No per-user or per-workspace scoping is involved; the value is process-global and
  identical for every caller.

## Failure Modes

| Condition | Behavior |
|---|---|
| Built without ldflags | `version` is `"dev"` (the compiled-in default), never empty. AC-8. |
| ldflag injected as empty string | `setBuildInfo` (`main.go:251`) skips empty values, so the `"dev"` default is retained. AC-11. |
| Request arrives before readiness | 503 with `status: "starting"` plus `version`. AC-5, AC-6, AC-7. |
| Request arrives during shutdown | Unchanged from today; this spec adds no new shutdown behavior. |
| Consumer strictly parses the JSON body | See the compatibility audit below. No in-tree consumer breaks. |

The handler performs no I/O, no database access, and no locking beyond the existing
`atomic.Bool` read. Reading an already-populated string field cannot fail, so this change
introduces no new error path and cannot make `/health` slower, flaky, or dependent on
another subsystem. That property matters: `/health` gates container start, so it must
never acquire a new dependency.

## Consumer Compatibility Audit

Every in-tree `/health` consumer was checked against an additive body field.

| Consumer | File | How it reads `/health` | Impact |
|---|---|---|---|
| K8s liveness + readiness | `k8s/deployment.yaml:37-47` | HTTP status only | None |
| Go launcher | `internal/launcher/health.go:44-80` | Status code; body drained and discarded | None |
| CLI | `apps/cli/src/health.ts` | `res.ok` only | None |
| Desktop shell (Tauri) | `apps/desktop/src-tauri/src/backend.rs:541-567` | Raw socket; parses status line + `x-kandev-desktop-health-token` header. Its own test body is literally `"ignored"` (`backend.rs:1032`) | None |
| Desktop smoke test | `apps/desktop/e2e/desktop-launch-smoke.mjs` | URL match on a fake runtime | None |
| Playwright fixture | `apps/web/e2e/fixtures/backend.ts:461` | Waits for HTTP readiness | None |
| Homebrew service | `scripts/release/kandev.rb` | Polls the URL | None |
| Docker docs example | `docs/docker.md` | `curl -f`, status only | None |
| Public operations doc | `docs/public/operations.md:70` | **Pins the exact JSON body** | **Must be updated — AC-15** |

No consumer deserializes the body into a fixed struct that would reject an unknown field,
so the change is backward compatible for every known caller.

## Testing Plan

| Layer | What | Count |
|---|---|---|
| Unit (Go) | Ready path: 200, `version` present, correct value, exact key set (AC-1..4) | +1 |
| Unit (Go) | Starting path: 503, `version` present and correct, exact key set (AC-5..7) | +1 |
| Unit (Go) | Default `"dev"` when unstamped; injected value when stamped; never empty (AC-8,9,11) | +1 |
| Unit (Go) | `/health` and `/api/v1/system/info` agree on the value (AC-10) | +1 |
| Unit (Go) | Auth enabled: `/health` unauthenticated returns body with `version`; `/system/info` still rejected (AC-12,14) | +1 |

These are the first tests to assert the `/health` body at all, so they also lock in
`status`, `service`, and `mode` against future accidental drift (AC-3, AC-13).

The exact-key-set assertions (AC-4, AC-7) are deliberate: they turn any future silent
addition to this public payload into a failing test, which is the behavior a frozen public
contract wants.

No new E2E test is required. The existing Playwright fixture already exercises `/health`
on every run and would fail if the endpoint regressed.

## Out of Scope

- `GET /api/v1/system/health` (the diagnostics/issues endpoint). Untouched.
- `GET /api/v1/system/info`. Untouched; it remains the detailed build-info surface and
  keeps its `commit`, `build_time`, `go_version`, `os`, `arch`, `boot_id`, `started_at`
  fields.
- The WS `health.check` action (`internal/gateway/websocket/handler.go:74`), which returns
  a similar `{status, service, mode}` shape. Adding the version there is a reasonable
  follow-up but is not required by the monitoring/alerting goal and is not specced here.
- Adding `commit`, `build_time`, uptime, or any other build metadata to `/health`. The
  endpoint stays minimal; `/system/info` is the place for detail.
- The agentctl server's own `/health` (`internal/agentctl/server/api/server.go:90`) and
  the office test harness `/health` (`internal/office/testharness/routes.go:94`). Separate
  servers, separate contracts.
- Renaming, moving, or changing the auth posture of any endpoint.
- Changing the `kandev_meta.kandev_version` database key or the share-snapshot
  `kandev_version` field. Same words, unrelated contracts.

## Decisions

Resolved with the requester on 2026-08-07, before this spec was frozen.

1. **Field name: `version`** (not `kandev_version`, not both). It matches
   `/api/v1/system/info` exactly, so the two endpoints agree on one name for one value.
   The payload already carries `"service":"kandev"`, so `version` unambiguously reads as
   "version of the service named kandev"; `kandev_version` beside `service: kandev` would
   be redundant. AC-10 pins the agreement.
2. **Present in both 200 and 503.** A backend stuck in startup is exactly when an operator
   most needs to know which build is stuck — it distinguishes a bad rollout from a slow
   disk. AC-6 pins this.
3. **Always exposed, never gated.** No auth condition, no runtime feature flag. Gating
   would defeat the stated monitoring goal, and a runtime flag would add a registry entry,
   profile defaults, and a disabled-path test for a single string field. AC-12 pins this.

## Documentation Impact

- `docs/public/operations.md:70` pins the exact `/health` body and MUST be updated
  (AC-15). It is the only doc that reproduces the payload verbatim.
- `docs/docker.md`, `docs/k8s.md`, `docs/run-as-a-service.md`, `docs/public/cli.md`, and
  `docs/public/authentication.md` reference `/health` but do not reproduce its body, so
  they need no change.
- Consider noting in `docs/public/operations.md` that `/health` is now the
  credential-free way to read the running version, since that is the operator-visible
  benefit.

## Rollback

Revert the handler change and the doc change. No migration, no persisted state, no config
key, no feature flag, and no cross-service contract is involved, so a straight revert is
complete and instant. A consumer that starts depending on the new field would see it
disappear, which is the ordinary cost of reverting an additive API field.

## Files Reference

| File | Change |
|---|---|
| `apps/backend/internal/backendapp/helpers.go:675-684` | Add `version: p.version` to both the 503 and 200 JSON bodies |
| `apps/backend/internal/backendapp/*_test.go` | New tests for the `/health` body (none exist today) |
| `docs/public/operations.md:70` | Update the documented example payload |

## Related

- ADR `docs/decisions/0008-db-upgrade-safety.md` — the `kandev_meta.kandev_version` key,
  a different contract that shares the name.
- `docs/public/authentication.md` — documents the unauthenticated allowlist that `/health`
  belongs to.

## Implementation Notes

Recorded 2026-08-07, during the build against this frozen spec.

- **`healthHandler` extraction (documented assumption).** The spec's Files Reference
  called for editing `helpers.go:675-684` in place. Implementing it that way would have
  left the handler as an inline closure with no way to unit-test AC-1..11 without
  standing up the full `registerRoutes` dependency graph (auth service, task service,
  gateway, lifecycle manager, etc. — far outside what a handler-body change warrants).
  Extracted the closure to a named `healthHandler(p routeParams) gin.HandlerFunc` in the
  same file, immediately below `registerRoutes`, with `p.router.GET("/health", ...)`
  unchanged at the call site. Behavior, route path, and JSON shape are identical; this is
  a pure testability refactor, not a contract change.
- **AC-10 test strategy.** Rather than an HTTP integration test hitting both live routes,
  the test constructs `healthHandler` and `info.Handler`/`info.NewService` with the same
  input string and asserts the JSON bodies agree. This is sufficient because both
  handlers are proven (by reading `main.go:801,1830`) to be fed the same package-level
  `Version` var, sampled after `setBuildInfo` runs in both cases — the test pins that
  invariant at the handler layer rather than re-deriving it via a full boot.
- **AC-14 test placement.** Added a `system info` case (`blocked: true`) to the existing
  `TestEnabledModeAllowlistMatrix` table in `internal/auth/httpmw/middleware_test.go`
  rather than standing up the real `/api/v1/system/info` route (which needs a DB pool,
  event bus, and the full `systemsvc.Provide` graph) — that test already exists
  specifically to pin the allowlist and is the natural home for a new blocked-path
  assertion.
- **Environment: pre-existing, unrelated test/lint failures observed in this sandbox.**
  Confirmed via `git stash` (reproduces identically on unmodified branch HEAD, so not
  caused by this change):
  - `go test ./...`: `internal/agentctl/server/config` (`TestCollectAgentEnvGitHubCLIShimSurvivesLoginShell`,
    depends on this machine's `gh` CLI/login-shell PATH), `internal/gitlab`
    (`TestPollerStartRecordsHealthForEveryConfiguredWorkspace`,
    `TestEnvironmentTokenAllowsImmutableStartupHost`,
    `TestResolveGitLabExecutionCredentialsByAuthMethod`, host-matching config depends on
    local env), `internal/repoclone`
    (`TestEnsureWorkspaceClonedWithBasicAuthKeepsCredentialScopedToGitChild`, passes in
    isolation — resource contention under full-suite parallelism), and intermittently
    `internal/agentctl/server/process` and `internal/agent/runtime/routingerr`
    (subprocess/timing-sensitive). None touch `backendapp`, `auth/httpmw`, or
    `system/info`; `go test ./internal/backendapp/... ./internal/auth/httpmw/...
    ./internal/system/info/...` is green.
  - `make lint` → `lint-harness`: `.github/scripts/lint-harness-files.py` uses
    `str | None` type syntax requiring Python 3.10+; this sandbox has Python 3.9.6.
    `lint-backend` (golangci-lint, 0 issues), `lint-web` (eslint, 0 warnings), and
    `lint-architecture` all pass when run directly.
