---
title: "Host-owned WebAuthn passkeys for user login"
description: "Feasibility study and scoped decision for adding FIDO2 WebAuthn / passkeys as a user login method to Kandev, grounded in the current opt-in auth architecture."
---

# 0053 — Host-owned WebAuthn passkeys for user login

- Status: proposed (decision under review; study basis for a feature spec)
- Date: 2026-08-28
- Area: backend, frontend, security, desktop
- Related: [0050 — Plugins provide OIDC/SAML login via a capability-gated, host-minted session](0050-plugin-external-auth-capability.md),
  [Opt-in authentication](2026-07-24-opt-in-authentication.md),
  [docs/specs/auth/spec.md](../specs/auth/spec.md)

## Verdict

Feasible, with the scope and boundary below. Kandev already ships an opt-in
multi-user auth layer (setup wizard, email+password login, DB-backed session
cookie, invites, admin user management, plugin SSO) gated by the
`features.auth` runtime flag. Passkeys slot into that layer as an additional
login method: host-owned ceremony endpoints, a credential table, a challenge
table, the login screen, and an enrollment surface in Settings > Account.

Conditions:

1. **Host boundary**: WebAuthn is host-owned, matching ADR 0050. The host
   keeps local users, passwords, passkeys, WebAuthn credentials and
   challenges, RP ID / trusted-origin configuration, sessions, cookies, PATs,
   recovery, authorization, and enrollment/account-management UI. External
   identity systems (OIDC, SAML, provider SSO) stay plugins that validate an
   external identity and send a normalized assertion; the host links and
   creates the session. An IdP that uses passkeys internally remains an
   OIDC/SAML plugin: the host never manages that provider's passkeys.
2. **Independent capability**: passkeys are controlled separately from
   `features.auth` (see "Capability model" below). Password login remains
   available in the first version.
3. **Recovery stays**: passkeys complement password login; there are no
   passkey-only accounts in v1 and no removal of the password recovery path.
   Kandev currently has **no admin password-reset operation** (invites create
   new accounts and do not recover existing ones), so break-glass behavior
   must be defined before passkey-only login is ever considered.
4. **Desktop is not supported in v1 as shipped**: the desktop shell pins the
   loopback origin to `http://127.0.0.1:<port>` and rejects `localhost`
   (`apps/desktop/src-tauri/src/backend.rs`), and WebAuthn RP IDs must be
   valid domain strings, so the IP-literal origin cannot host WebAuthn even
   on webviews that support it. Desktop passkeys require changing the desktop
   origin contract to a valid loopback hostname plus per-platform webview
   verification (see "Platform matrix").

Recommended path if approved: password + optional passkeys, host-owned,
feature-flagged with `features.auth` plus a separate passkey control, phased
backend-then-frontend, E2E with Playwright virtual authenticators in the
existing `auth` test project. Rough estimate: 2 to 3 weeks of focused work.

## 1. What WebAuthn / passkeys are (30-second grounding)

WebAuthn (W3C) is the web API for FIDO2 public-key authentication. The server
never holds a shared secret: registration stores a public key and the private
key never leaves the authenticator (platform or roaming). Two ceremonies:

- **Registration** (`navigator.credentials.create` with `publicKey` options):
  the client sends an attestation; the server stores the credential
  (credential ID, public key, sign counter, transports, AAGUID, credential
  flags).
- **Authentication** (`navigator.credentials.get`): the client signs a server
  challenge; the server verifies the signature and the sign counter.

A **passkey** is a *discoverable credential* (resident key): the authenticator
can enumerate the user's credentials without the server naming one first,
which enables passwordless sign-in and syncing via iCloud Keychain, Google
Password Manager, or Windows Hello. Key properties that matter here:

- **Secure context required**: HTTPS, or `localhost` / loopback hostnames
  (which browsers treat as secure). Note that a *loopback IP literal*
  (`127.0.0.1`) is a secure context but is **not** a valid RP ID: the W3C
  algorithm requires a valid domain string, so `localhost` and `*.localhost`
  work, `127.0.0.1` does not.
- **RP ID**: a registrable domain suffix (or the loopback hostname) that the
  origin must match; the RP ID is what binds a credential to one site, which
  is the phishing-resistance property. Ports are not part of the RP ID.
- **User verification**: biometric/device unlock; configured as
  `required` / `preferred` / `discouraged`.
- **Attestation**: normally `none` for passkeys (privacy preserving; the RP
  does not need to verify the authenticator model).
- **Sign counter**: monotonic counter checked on each assertion to detect
  cloned authenticators, with the caveats in "Credential state" below.
- **Credential flags**: WebAuthn Level 3 authenticator data carries
  `backup_eligible` (BE: whether the credential can be synced; permanent) and
  `backup_state` (BS: whether it has actually been backed up; changes over
  time). The selected Go library exposes both plus `UserVerified` per
  assertion (see "Credential state").
- **Conditional UI**: `mediation: "conditional"` plus
  `autocomplete="username webauthn"` (both tokens on the same username/email
  field; the `webauthn` token alone is not recognized) shows saved passkeys in
  the browser's autofill dropdown.
- **Passkeys on `localhost`/loopback origins are device-local**: sync
  providers do not sync credentials for testing origins. This is fine for the
  local-first case (the OS account is the boundary) but means cross-device
  passkey roaming only matters for hosted deployments with a real domain.

## 2. Current auth architecture (evidence from the repo)

Auth is opt-in and lives in `apps/backend/internal/auth/` plus
`apps/backend/internal/user/`. Relevant facts:

- **Enablement**: the `features.auth` runtime flag
  (`KANDEV_FEATURES_AUTH`, Settings > System > Feature Toggles), off by default
  in every profile (`apps/backend/internal/profiles/profiles.yaml`). The auth
  service derives a mode state machine from it:
  `disabled` (single-user, synthetic admin identity) /
  `setup` (flag on, no admin yet) / `enabled`
  (`internal/auth/service.go` `refreshMode`). Off-by-default is a tested
  invariant (`config_test.go`).
- **Credentials today**: argon2id password hashes
  (`internal/auth/password.go`, OWASP parameters); opaque session tokens
  (`kandev_session` cookie) and personal access tokens (`kandev_pat_*`),
  stored as SHA-256 digests (`internal/auth/tokens.go`,
  `service_credentials.go`).
- **Storage**: SQLite (Postgres-capable DDL with TIMESTAMPTZ substitution),
  tables `users`, `auth_identities`, `auth_sessions`, `auth_api_tokens`,
  `auth_invites` (`internal/auth/store/schema.go`, `models.go`;
  `internal/user/store/sqlite.go`). DDL is replay-safe.
- **Enforcement**: global middleware `internal/auth/httpmw.Middleware`
  (installed in `backendapp.buildHTTPServer`), WS gateway
  `requireConnectionAuth` / `Client.authorizeAction`
  (`internal/gateway/websocket/access.go`, `dispatch_scope.go`),
  service-layer `authorize*` helpers, and MCP scoping
  (`internal/mcp/scope`). Identity travels in the request context
  (`authn.Identity`, `RequireRealIdentity`, `RequireAdmin`).
- **HTTP surface**: `/api/v1/auth/{setup,login,logout,me,password,sessions,
  tokens,invites/*}` plus `/api/v1/users` admin routes
  (`internal/auth/httpapi/handlers.go`). `GET /api/v1/auth/me` and the SPA
  boot payload share one shape: `StatePayload {mode, authenticated, user,
  ssoProviders}` (`internal/auth/state.go`).
- **SSO**: plugin-contributed OIDC/SAML buttons on the login screen;
  `ExternalIdentity` + `AuthenticateExternal` with JIT provisioning/linking
  (`service_sso.go`), per ADR 0050. This is the existing precedent for "a
  second way to sign in" and the boundary this study keeps: the plugin
  asserts identity, the host mints the session.
- **Frontend**: pre-app auth gate (`apps/web/src/auth-gate.tsx`,
  `useAuthGateDecision` -> setup/login/invite/app), email+password
  `LoginPage` with an SSO button block (`apps/web/app/auth/login-page.tsx`),
  setup wizard and invite pages. Identity hydrates from the boot payload into
  an in-memory Zustand `auth` slice (`lib/state/slices/auth/`,
  `lib/state/default-state.ts`); every fetch sends the session cookie
  (`credentials: "include"`); a 401 with `WWW-Authenticate: Bearer` clears
  the slice and redirects to `/login` (`lib/api/client.ts`, `src/main.tsx`).
  WS connects to `ws(s)://<host>/ws` with no handshake param, relying on the
  same-origin cookie.
- **Account surfaces**: Settings > Account > Security (change password,
  device sessions list/revoke), API tokens, admin user management
  (`components/settings/account/security-settings.tsx`, `api-tokens.tsx`).
- **Desktop**: the Tauri shell (`apps/desktop`) has no auth of its own and no
  secure-storage usage: it spawns the Go binary `--headless` on
  `127.0.0.1:38430` and loads the same SPA. `validate_desktop_url` requires
  plain `http` + host exactly `127.0.0.1` (it rejects `localhost` and https);
  the owned-origin contract and CSP/connect-src capabilities pin
  `http://127.0.0.1:*` (`src-tauri/src/backend.rs`, `tauri.conf.json`).
- **Transport**: plain HTTP everywhere. Dev backend on `localhost:38429`,
  desktop on `127.0.0.1`, k8s ingress `kandev.example.com` with no `tls:`
  block (`k8s/ingress.yaml`), agentctl plain HTTP on 39429.
- **Existing WebAuthn code**: none. Zero matches for
  webauthn/fido/passkey/u2f/authenticator across `apps/backend`,
  `apps/web`, and `apps/desktop`; no 2FA/TOTP either. The only credential
  crypto is argon2id from `golang.org/x/crypto`.
- **Docs drift**: `docs/public/security.md` says Kandev has no multi-user
  login; the shipped opt-in auth layer contradicts that. This is corrected as
  a standalone docs fix alongside this study (independent of whether the
  passkey work proceeds), not deferred to the feature timeline.

## 3. Capability model

Passkeys are independent from the `features.auth` choice. Required controls:

1. `features.auth` enables the user and authentication system.
2. A separate passkey control (new runtime flag, e.g. `features.passkeys` /
   `KANDEV_FEATURES_PASSKEYS`, registered in
   `internal/runtimeflags/registry.go` following the `features.auth`
   pattern) enables direct WebAuthn login.
3. Each user decides whether to enroll a passkey.
4. Password login remains available in the first version.

Effective passkey capability requires **all** of:

- Authentication is enabled (`features.auth` in mode `enabled`).
- Passkey login is enabled (`features.passkeys`).
- The RP ID and allowed-origin configuration are valid (see
  "RP ID and origin configuration").
- The current origin supports WebAuthn (secure context, RP ID matches origin,
  and the browser/webview exposes `navigator.credentials`).

The boot payload exposes this as `passkeysAvailable` (a new field on
`StatePayload` / `initialState.auth`). The frontend must hide the passkey
interface when the value is false.

If an administrator disables passkeys, Kandev must retain existing credentials
without allowing their use. Re-enabling the feature restores them.

## 4. What the change would look like

### Backend (`apps/backend/internal/auth/`)

- **Credential table** `auth_webauthn_credentials`, same replay-safe DDL
  pattern as the existing auth tables (SQLite + Postgres):

  `id`, `user_id`, `credential_id` (unique), `public_key` (COSE bytes),
  `sign_count`, `transports`, `aaguid`, `user_handle`, `backup_eligible`,
  `backup_state`, `last_used_user_verified`, `name`, `created_at`,
  `last_used_at`.

  **User handle**: a random opaque value (uuid or 64 random bytes, WebAuthn
  Level 2 §6.1: no PII, SHOULD be 64 random bytes), stored on the user row
  via a migration `ADD COLUMN webauthn_user_handle ... DEFAULT NULL`, set
  once at first enrollment. Do **not** use a deterministic hash of the user
  ID: it is observable by the platform that stores the authenticator
  credential and reversible when the ID format is known.

- **Challenge table** `auth_webauthn_challenges`: `id`, `user_id` (null for
  anonymous login), `challenge` (base64), `ceremony` (`register` | `login`),
  `expires_at`, with an index on `expires_at` for TTL sweeps. **SQLite by
  default**: an in-memory store silently invalidates in-flight ceremonies on
  any backend restart (upgrade, crash, container redeploy) and adds a
  migration later; the DB is the same pattern as `store/schema.go` and costs
  about 10 lines. No code change needed for multi-instance deployments.

- **Ceremony endpoints** under `/api/v1/auth/webauthn/`:
  - `POST register/options` (authenticated, requires recent reauthentication):
    generate challenge, build `PublicKeyCredentialCreationOptions` with
    `excludeCredentials` from the user's existing credentials; store the
    pending challenge bound to the session's user.
  - `POST register/verify` (authenticated): verify the attestation against
    the **server-controlled expected origin, RP ID, stored challenge,
    ceremony type, and the authenticated user binding**; store the
    credential; atomically consume the challenge.
  - `POST login/options` (public): generate challenge. Accept an optional
    email hint; for discoverable credentials the assertion itself carries the
    user handle. Respond uniformly to avoid user enumeration.
  - `POST login/verify` (public): verify the assertion against the same
    expected origin / RP ID / stored challenge / ceremony type inputs,
    atomically consume the challenge, resolve the user, and mint a session
    exactly like password login (`service_credentials.go` already has the
    session-minting path to reuse): same `kandev_session` cookie, same
    lifecycle.

  Challenge consumption is **atomic and single-use** for both ceremonies: the
  first verification attempt wins and deletes the challenge; a replay of the
  same assertion fails.

- **RP ID and origin configuration**: new `ServerConfig`-adjacent fields,
  e.g. `webauthn.rpId` and `webauthn.allowedOrigins` (env
  `KANDEV_WEBAUTHN_RP_ID`, `KANDEV_WEBAUTHN_ALLOWED_ORIGINS`), because
  `ServerConfig` today only describes bind addresses and an internal web URL
  (`apps/backend/internal/common/config/config.go`). The trust boundary must
  come from this explicit trusted configuration, never from untrusted request
  or forwarded-host headers. This matters whenever TLS terminates at an
  ingress and the browser's public origin differs from the backend
  connection. The desktop/dev defaults derive from the loopback hostname.
- **Gating**: everything behind `features.auth` AND `features.passkeys`
  (mode `enabled`), so single-user/local deployments are untouched. Register
  options/verify require a real identity (`RequireRealIdentity`); login
  options/verify are public routes added to the middleware allowlist.
- **Verification inputs** (both ceremonies, server-controlled): expected
  origin, RP ID, stored challenge, ceremony type, and (for registration) the
  authenticated user binding. No value derived from client-supplied fields.

### Frontend (`apps/web/`)

- **Login page**: a "Sign in with passkey" button (mirroring the existing SSO
  button block), rendered only when the boot payload's `passkeysAvailable` is
  true, that runs the login ceremony and, on success, hard-reloads so the
  boot payload re-hydrates with the new session cookie, exactly like password
  login today. Follow-up: conditional UI on the email field with
  `autocomplete="username webauthn"` and `mediation: "conditional"`.
- **Enrollment**: Settings > Account > Security gains a "Passkeys" section
  (list, add, rename, remove). Enrollment and removal are security-sensitive
  account changes: a valid long-lived session alone is not sufficient. Both
  must require **recent reauthentication** with the current password or an
  existing passkey (a reauthentication timestamp checked per operation, with
  a bounded window).
- **Setup wizard**: optionally offer passkey enrollment for the first admin.
  Not required for feasibility.
- All new copy through `t()` (i18n ratchets apply).

### Session, WS, and authorization

No new auth surface: after a successful assertion the user gets the same
DB-backed session cookie, and WS/MCP/HTTP enforcement keep working unchanged.
Session issuance is **reused**; credential storage, challenge storage, and
WebAuthn verification are new security-critical components with their own
review and tests.

## 5. Credential state

Synced passkeys (iCloud Keychain, Google Password Manager, 1Password)
commonly return `sign_count = 0` on every assertion because they cannot
maintain a monotonic counter across device copies. Rules:

- Accept `stored_count == 0 && new_count == 0` as "counter not supported"
  (go-webauthn already models this); do not flag it as cloning.
- Compare counters when **either** value is non-zero. Treat a non-increasing
  value as a **clone signal, not proof**.
- Update the stored counter only when the new value is greater.

Persist per-credential:

- **Immutable**: credential ID, public key, AAGUID, transports,
  `backup_eligible` (BE is permanent per WebAuthn L3), user handle, created
  at.
- **Per-assertion**: `sign_count` (only ever increases), `backup_state` (BS
  can change over time), `last_used_user_verified`, `last_used_at`.

The selected Go library version's `CredentialFlags` (`BackupEligible`,
`BackupState`, `UserVerified`) maps directly onto these columns; record the
library version in the implementation notes so the flag semantics match.

## 6. Library options

| Layer | Option | Assessment |
|---|---|---|
| Go server | `github.com/go-webauthn/webauthn` (v0.14, 2025) | **Recommended.** Actively maintained, FIDO2-conformant, handles CBOR/COSE, challenge, and verification; `CredentialFlags` covers BE/BS/UserVerified; minimal storage interface that maps to the existing store. |
| Go server | `github.com/duo-labs/webauthn` | Avoid: archived; superseded by go-webauthn. |
| Go server | `github.com/fxamacker/webauthn` | Lighter alternative if go-webauthn's surface feels heavy; less ecosystem traction. |
| Browser | `@simplewebauthn/browser` (v13) | **Recommended.** Thin wrappers (`startRegistration`, `startAuthentication`) over `navigator.credentials`, handles base64url and browser quirks. |
| Browser | Hand-rolled `navigator.credentials` | Doable but error-prone (base64url, CBOR, browser differences); not worth it. |

No server-side secret storage changes: the existing AES-256 master key
(`<home>/data/master.key`) is for provider/agent secrets and is untouched.

## 7. Platform matrix (the main feasibility driver)

| Surface | Origin | Secure context | RP ID usable | Passkey sync/roaming | Verdict |
|---|---|---|---|---|---|
| Web, local dev | `http://localhost:*` | Yes | `localhost` | Device-local only | Works; dev/test only |
| Web, desktop shell (as shipped) | `http://127.0.0.1:38430` | Yes (loopback) | **No: IP literal is not a valid RP ID** | n/a | **Not supported in v1**; origin contract must change first |
| Web, desktop shell (after change) | `http://localhost:38430` (or `*.localhost`) | Yes | `localhost` | Device-local only | Candidate, pending per-platform webview verification |
| Web, hosted (k8s/docker/VM) | `https://<domain>` | Requires TLS at ingress | Domain | Full (iCloud Keychain, Google Password Manager, Windows Hello) | The case that actually benefits; **TLS + RP ID/origin config prerequisite** |
| Web, LAN/plain HTTP host | `http://<host>:port` | No | n/a | n/a | Passkeys unavailable; password/SSO only |

Desktop webview specifics:

- **Origin contract work**: the desktop shell pins `127.0.0.1` everywhere:
  `validate_desktop_url` rejects `localhost` and https, the owned-origin
  check (`accepts_url`) matches `http://127.0.0.1:<port>` exactly, and the
  Tauri CSP/connect-src capabilities allow only `http://127.0.0.1:*`.
  Enabling desktop passkeys requires switching this contract to a valid
  loopback hostname (`localhost` or a `*.localhost` name), including the
  health-token and URL-validation paths.
- **Windows**: WebView2 (Chromium) supports WebAuthn. Passkeys usable once
  the origin is a valid loopback hostname; verify in the shipped runtime.
- **macOS**: WKWebView WebAuthn/passkey support is tied to an Associated
  Domains (`webcredentials`) entitlement plus an AASA file on the RP domain;
  without that two-way association, treat passkeys in the desktop webview as
  **unverified**. Verify against the shipped entitlements and macOS version
  before promising support.
- **Linux**: WebKitGTK has historically incomplete WebAuthn support (WebKit
  bug 205350, open for years). **Unverified**; do not promise desktop
  passkeys on Linux without testing the shipped webkit2gtk version.

Practical consequence: passkeys add real value where Kandev is exposed over a
real HTTPS domain (hosted/remote deployments) and as a local convenience in
the browser on the developer's own machine. On localhost/loopback they are
per-device by design, which is acceptable because the OS account is already
the security boundary there. Desktop passkeys are a distinct workstream with
its own origin-contract and webview verification.

## 8. Security analysis

- **Phishing resistance**: the RP ID binding is the point; credentials are
  unphishable relative to passwords. This is the strongest argument for the
  hosted deployment case, where the security doc currently recommends an
  "authenticated TLS access proxy" as the boundary.
- **Verification contract**: every ceremony verifies against server-controlled
  expected origin, RP ID, stored challenge, and ceremony type; registration
  additionally binds to the authenticated user. Challenges are random
  (>= 16 bytes, 32 recommended), single-use (atomic consumption), and expire
  within minutes.
- **Sign counter / credential flags**: see "Credential state". Cloned
  authenticators surface as counter regression or BS/UserVerified
  inconsistencies, logged and alerted, not silently accepted.
- **RP ID / origin trust**: from explicit configuration only
  (`webauthn.rpId`, `webauthn.allowedOrigins`); never derived from request or
  forwarded-host headers.
- **User enumeration**: `login/options` must not reveal whether an email has
  credentials; keep the response uniform and rely on discoverable
  credentials where possible.
- **Recovery**: keep password + invites as fallback; passkey loss (device
  reset) must not lock out the admin. Kandev has **no admin password-reset
  operation today**, so the recovery/break-glass path must be defined in the
  feature spec before passkey-only login is considered. v1 has no passkey-only
  accounts and never removes the password recovery path. Invariant: **at
  least one usable login method remains after every credential change** (a
  user cannot remove their last usable method).
- **Sensitive operations**: passkey enrollment and removal require recent
  reauthentication (password or existing passkey), not merely a long-lived
  session.
- **Rate limiting**: extend the existing login rate limiting
  (`service.go`) to the passkey login endpoints.
- **Attestation**: `none` for passkeys; no device-fingerprint data stored.
- **Session reuse**: after assertion the user gets the same session-cookie
  machinery. Session issuance is reused; credential storage, challenge
  storage, and WebAuthn verification are new security-critical components.

## 9. Testing feasibility

- **E2E**: Playwright can drive a WebAuthn virtual authenticator over CDP
  (`WebAuthn.enable`, `WebAuthn.addVirtualAuthenticator` with resident keys
  and user verification), so passkey registration and login are testable
  end to end. Kandev already has a dedicated `auth` Playwright project
  (`apps/web/e2e/tests/auth/`) that restarts the backend with
  `KANDEV_FEATURES_AUTH=true` per worker (`backend.restart({...})`) - new
  `auth/passkeys.spec.ts` fits there, adding `KANDEV_FEATURES_PASSKEYS=true`.
  Recent Playwright versions also ship a cross-browser passkey seeding API.
- **Unit**: go-webauthn ships protocol test fixtures; ceremony verification
  (expected origin/RP ID/challenge/ceremony/user binding, single-use
  consumption), counter/flag rules, reauthentication gating, and the
  credential/challenge stores are unit-testable in Go following the existing
  `internal/auth` test conventions (service tests with in-memory/SQLite
  stores).
- **i18n**: new copy must go through `t()`; the ratchets apply as for any UI
  change.

## 10. Risks and open questions

| # | Risk / question | Impact | Mitigation |
|---|---|---|---|
| 1 | Hosted passkeys need TLS at the ingress (no `tls:` block today) plus explicit RP ID/origin config | Hosted passkey sync unavailable until both land | Add `tls:` block / document an authenticated TLS proxy; ship `webauthn.rpId` + `webauthn.allowedOrigins` config as a prerequisite |
| 2 | Desktop origin is an IP literal; RP ID must be a domain string; desktop rejects `localhost` | Desktop passkeys unsupported as shipped | Change the desktop navigation/owned-origin/CSP contract to a valid loopback hostname; verify per platform (WebView2, WKWebView associated domains, WebKitGTK) |
| 3 | WKWebView passkeys need Associated Domains + AASA | macOS desktop passkeys may be gated on signing/entitlements work | Verify in the spec phase; desktop passkeys may ship after web/hosted |
| 4 | Linux webview WebAuthn unverified | Desktop Linux users may not get passkeys | Scope desktop passkeys as best-effort or web-only on Linux |
| 5 | No admin password reset exists; invites do not recover accounts | Break-glass undefined until designed | Define recovery/break-glass in the feature spec; v1 keeps password login |
| 6 | Sensitive-operation reauthentication window | Policy choice | Pick a bounded window (e.g. minutes) in the spec phase; reuse for other sensitive ops if a pattern emerges |
| 7 | Challenge lifecycle in the DB | TTL sweep + atomic consumption | `auth_webauthn_challenges` with `expires_at` index; single-use consumption in the same transaction as verification |
| 8 | User enumeration on login/options | Minor | Uniform responses; discoverable credentials |
| 9 | `security.md` doc drift (claims no multi-user login) | Public docs wrong today | Standalone docs correction, landed with this study, independent of the feature decision |
| 10 | Conditional UI browser coverage | UX polish only | Ship button first; conditional UI as follow-up |

## 11. Phasing and effort (rough)

- **Phase 1 (backend, ~4-6 days)**: `features.passkeys` runtime flag +
  `webauthn.rpId`/`webauthn.allowedOrigins` config; credential + challenge
  tables; ceremony endpoints with the explicit verification contract
  (expected origin, RP ID, stored challenge, ceremony type, user binding);
  atomic single-use challenge consumption; counter/credential-flag rules;
  reauthentication gate for enrollment/removal; login rate limiting on the
  public passkey endpoints from day one (extending the existing
  password-login limiter, since these routes are public); unit tests for all
  of the above. No UI change yet.
- **Phase 2 (frontend, ~2-3 days)**: `passkeysAvailable` in `StatePayload` /
  boot payload; login-page passkey button; Settings > Account passkey
  enrollment/management with reauthentication; i18n; E2E
  `auth/passkeys.spec.ts` with a virtual authenticator.
- **Phase 3 (desktop, hosted, docs; ~2-3 days)**: desktop origin-contract
  change to a valid loopback hostname + per-platform webview verification;
  hosted TLS guidance and RP ID/origin configuration docs; `security.md`
  correction; feature-status docs.
- Total: roughly 2 to 3 weeks of focused effort by one engineer, using the
  standard Go/TS test conventions. No new infra, no new long-lived secrets.

## 12. Recommendation

Proceed, scoped as **host-owned password + passkey login**, passkeys gated by
a separate `features.passkeys` control alongside `features.auth`, in the three
phases above. The architecture has every seam needed (auth service, state
payload, login screen, account settings, `auth` E2E project), the libraries
are mature, and the security properties are a net improvement for hosted
deployments.

Decisions to confirm before implementation:

1. Password + passkey scope (recommended) vs passkey-only (requires the
   recovery/break-glass design first).
2. Hosted TLS + RP ID/origin configuration in the same workstream (prerequisite
   for hosted passkey sync, independent of the feature flag).
3. Desktop passkeys in v1 (requires the origin-contract change and webview
   verification) vs web/hosted first with desktop as a follow-up.

If approved, the repo workflow continues with a feature spec under
`docs/specs/webauthn-passkeys/spec.md`, an implementation plan under
`docs/plans/webauthn-passkeys/plan.md`, and task files, per the
spec-driven-development process.
