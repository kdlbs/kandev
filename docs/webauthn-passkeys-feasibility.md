---
title: "WebAuthn / passkeys for user login: feasibility study"
description: "Analysis of adding FIDO2 WebAuthn / passkeys as a user login method to Kandev, grounded in the current opt-in auth architecture."
---

# WebAuthn / passkeys for user login: feasibility study

**Status**: Study (no code changed). **Scope**: assess feasibility only.

## Verdict

Feasible, with three conditions. Kandev already ships an opt-in multi-user auth
layer (setup wizard, email+password login, DB-backed session cookie, invites,
admin user management, plugin SSO) gated by the `features.auth` runtime flag.
Passkeys slot cleanly into that layer as an additional login method: the
backend ceremony endpoints, a credential table, the login screen, and an
enrollment surface in Settings > Account.

Conditions:

1. **Passkeys should complement password login, not replace it.** Password
   stays as the recovery fallback (plus admin reset and invites). Passkey-only
   login is a product decision with account-recovery implications, not a
   technical blocker.
2. **Hosted deployments need TLS at the ingress first.** WebAuthn requires a
   secure context; the shipped k8s ingress has no `tls:` block. Without HTTPS,
   passkeys work only in the device-local browser-on-localhost sense.
3. **Desktop Linux is a verification risk.** The Tauri webview on Linux
   (WebKitGTK) has historically incomplete WebAuthn support; Windows (WebView2)
   and macOS (WKWebView) are supported. Confirmed by a platform matrix below.

Recommended path if approved: password + passkey login, feature-flagged
together with `features.auth`, phased backend-then-frontend, E2E with Playwright
virtual authenticators in the existing `auth` test project. Rough estimate:
1.5 to 2 weeks of focused work.

## 1. What WebAuthn / passkeys are (30-second grounding)

WebAuthn (W3C) is the web API for FIDO2 public-key authentication. The server
never holds a shared secret: registration stores a public key and the private
key never leaves the authenticator (platform or roaming). Two ceremonies:

- **Registration** (`navigator.credentials.create` with `publicKey` options):
  the client sends an attestation; the server stores the credential
  (credential ID, public key, sign counter, transports, AAGUID).
- **Authentication** (`navigator.credentials.get`): the client signs a server
  challenge; the server verifies the signature and the sign counter.

A **passkey** is a *discoverable credential* (resident key): the authenticator
can enumerate the user's credentials without the server naming one first,
which enables passwordless sign-in and syncing via iCloud Keychain, Google
Password Manager, or Windows Hello. Key properties that matter here:

- **Secure context required**: HTTPS, or `localhost` / loopback (which browsers
  treat as secure).
- **RP ID**: a registrable domain suffix (or an IP literal) that the origin
  must match; the RP ID is what binds a credential to one site, which is the
  phishing-resistance property. Ports are not part of the RP ID.
- **User verification**: biometric/device unlock; configured as
  `required` / `preferred` / `discouraged`.
- **Attestation**: normally `none` for passkeys (privacy preserving; the RP
  does not need to verify the authenticator model).
- **Sign counter**: monotonic counter checked on each assertion to detect
  cloned authenticators.
- **Conditional UI**: `mediation: "conditional"` + `autocomplete="webauthn"`
  on the username field shows saved passkeys in the browser's autofill
  dropdown, so the login page can offer passkeys without a dedicated button.
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
- **HTTP surface**: `/api/v1/auth/{setup,login,logout,me,invites/*}` plus
  session/token management and `/api/v1/users` admin routes
  (`internal/auth/httpapi/handlers.go`). `GET /api/v1/auth/me` and the SPA
  boot payload share one shape: `StatePayload {mode, authenticated, user,
  ssoProviders}` (`internal/auth/state.go`).
- **SSO**: plugin-contributed OIDC/SAML buttons on the login screen;
  `ExternalIdentity` + `AuthenticateExternal` with JIT provisioning/linking
  (`service_sso.go`). This is the existing precedent for "a second way to
  sign in".
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
  `127.0.0.1:38430` and loads the same SPA (`src-tauri/src/backend.rs`,
  forces `KANDEV_SERVER_HOST=127.0.0.1`).
- **Transport**: plain HTTP everywhere. Dev backend on `localhost:38429`,
  desktop on `127.0.0.1`, k8s ingress `kandev.example.com` with no `tls:`
  block (`k8s/ingress.yaml`), agentctl plain HTTP on 39429.
- **Existing WebAuthn code**: none. Zero matches for
  webauthn/fido/passkey/u2f/authenticator across `apps/backend`,
  `apps/web`, and `apps/desktop`; no 2FA/TOTP either. The only credential
  crypto is argon2id from `golang.org/x/crypto`.
- **Docs drift**: `docs/public/security.md` says Kandev has no multi-user
  login; the shipped opt-in auth layer contradicts that and the doc needs an
  update when auth behavior changes (separate follow-up).

## 3. What the change would look like

### Backend (`apps/backend/internal/auth/`)

- **New table** `auth_webauthn_credentials`, same DDL pattern as the existing
  auth tables (replay-safe, SQLite + Postgres):
  `id`, `user_id`, `credential_id` (unique), `public_key` (COSE bytes),
  `sign_count`, `transports`, `aaguid`, `user_handle`, `created_at`,
  `last_used_at`. Optionally `name` for the "Add passkey" management UI.
- **Ceremony endpoints** under `/api/v1/auth/webauthn/`:
  - `POST register/options` (authenticated): generate challenge, build
    `PublicKeyCredentialCreationOptions` with `excludeCredentials` from the
    user's existing credentials; store the pending challenge.
  - `POST register/verify` (authenticated): verify attestation, store the
    credential, clear the challenge. Same audit/ownership rules as other
    account mutations.
  - `POST login/options` (public): generate challenge. Accept an optional
    email hint; for discoverable credentials the assertion itself carries the
    user handle. Respond uniformly to avoid user enumeration.
  - `POST login/verify` (public): verify assertion, resolve the user, mint a
    session exactly like password login (`service_credentials.go` already has
    the session-minting path to reuse), set the same `kandev_session` cookie.
- **Challenge store**: short-lived (TTL minutes), single-use. In-memory map
  is fine for the single-replica default; if multi-instance deployments
  matter, reuse the DB (Postgres-ready DDL already exists). All ceremony
  state is server-side; no new long-lived secrets.
- **Gating**: everything behind `features.auth` (mode `enabled`), so
  single-user/local deployments are untouched. Register options/verify
  require a real identity (`RequireRealIdentity`); login options/verify are
  public routes added to the middleware allowlist.
- **User handle**: stable per-user bytes for the passkey identity (the
  existing user ID, hashed to fixed size, is the natural source).

### Frontend (`apps/web/`)

- **Login page**: a "Sign in with passkey" button (mirroring the existing SSO
  button block) that runs the login ceremony and, on success, hard-reloads so
  the boot payload re-hydrates with the new session cookie, exactly like
  password login today. Optional later: conditional UI on the email field
  (`autocomplete="webauthn"` + `mediation: "conditional"`).
- **Enrollment**: Settings > Account > Security gains a "Passkeys" section
  (list, add, rename, remove) next to the existing sessions/tokens
  management.
- **Setup wizard**: optionally offer passkey enrollment for the first admin.
  Not required for feasibility.
- All new copy through `t()` (i18n ratchets apply).

### Session and WS

No new auth surface: after a successful assertion the user gets the same
DB-backed session cookie, and WS/MCP/HTTP enforcement keep working unchanged.

## 4. Library options

| Layer | Option | Assessment |
|---|---|---|
| Go server | `github.com/go-webauthn/webauthn` (v0.14, 2025) | **Recommended.** Actively maintained, FIDO2-conformant, handles CBOR/COSE, challenge, and verification; minimal storage interface (user with credentials) that maps to the existing store. |
| Go server | `github.com/duo-labs/webauthn` | Avoid: archived; superseded by go-webauthn. |
| Go server | `github.com/fxamacker/webauthn` | Lighter alternative if go-webauthn's surface feels heavy; less ecosystem traction. |
| Browser | `@simplewebauthn/browser` (v13) | **Recommended.** Thin wrappers (`startRegistration`, `startAuthentication`) over `navigator.credentials`, handles base64url and browser quirks. |
| Browser | Hand-rolled `navigator.credentials` | Doable but error-prone (base64url, CBOR, browser differences); not worth it. |

No server-side secret storage changes: the existing AES-256 master key
(`<home>/data/master.key`) is for provider/agent secrets and is untouched.

## 5. Platform and origin matrix (the main feasibility driver)

| Surface | Origin | Secure context | RP ID | Passkey sync/roaming | Verdict |
|---|---|---|---|---|---|
| Web, local dev | `http://localhost:*` | Yes (browser special-case) | `localhost` | Device-local only | Works; dev/test only |
| Web, desktop shell | `http://127.0.0.1:38430` | Yes (loopback) | `127.0.0.1` | Device-local only | Works on Windows/macOS webviews; Linux risk (below) |
| Web, hosted (k8s/docker/VM) | `https://<domain>` | Requires TLS at ingress | Domain | Full (iCloud Keychain, Google Password Manager, Windows Hello) | The case that actually benefits; **TLS prerequisite** |
| Web, LAN/plain HTTP host | `http://<host>:port` | No | n/a | n/a | Passkeys unavailable; password/SSO only |

Desktop webview specifics (Tauri loads the SPA in the platform webview):

- **Windows**: WebView2 (Chromium) supports WebAuthn. Passkeys usable.
- **macOS**: WKWebView supports WebAuthn. Passkeys usable.
- **Linux**: WebKitGTK has historically incomplete WebAuthn support (WebKit
  bug 205350, open for years). **Must verify against the shipped webkit2gtk
  version before promising desktop passkeys on Linux**; fallback is a
  native-webview-independent path or web-only enrollment for Linux desktop
  users.

Practical consequence: passkeys add real value where Kandev is exposed over a
real HTTPS domain (hosted/remote deployments) and as a local convenience in
the browser on the developer's own machine. On localhost/loopback they are
per-device by design, which is acceptable because the OS account is already
the security boundary there.

## 6. Security analysis

- **Phishing resistance**: the RP ID binding is the point; credentials are
  unphishable relative to passwords. This is the strongest argument for the
  hosted deployment case, where the security doc currently recommends an
  "authenticated TLS access proxy" as the boundary.
- **Sign counter**: store and check it per assertion (clone detection).
  go-webauthn does this if the credential's `SignCount` is maintained.
- **Challenges**: random, >= 16 bytes, single-use, TTL minutes. Standard.
- **User enumeration**: `login/options` must not reveal whether an email has
  credentials; keep the response uniform and rely on discoverable
  credentials where possible.
- **Recovery**: keep password + admin reset + invites as fallback; passkey
  loss (device reset) must not lock out the admin. This argues against
  passkey-only deployments unless recovery UX is designed deliberately.
- **Rate limiting**: extend the existing login rate limiting
  (`service.go`) to the passkey login endpoints.
- **Attestation**: `none` for passkeys; no device-fingerprint data stored.
- **Session reuse**: after assertion the user gets the same session-cookie
  machinery; no new trust boundary is introduced.

## 7. Testing feasibility (good news)

- **E2E**: Playwright can drive a WebAuthn virtual authenticator over CDP
  (`WebAuthn.enable`, `WebAuthn.addVirtualAuthenticator` with resident keys
  and user verification), so passkey registration and login are testable
  end to end. Kandev already has a dedicated `auth` Playwright project
  (`apps/web/e2e/tests/auth/`) that restarts the backend with
  `KANDEV_FEATURES_AUTH=true` per worker (`backend.restart({...})`) - new
  `auth/passkeys.spec.ts` fits there. Recent Playwright versions also ship a
  cross-browser passkey seeding API.
- **Unit**: go-webauthn ships protocol test fixtures; ceremony verification
  and the credential store are unit-testable in Go following the existing
  `internal/auth` test conventions (service tests with in-memory/SQLite
  stores).
- **i18n**: new copy must go through `t()`; the ratchets apply as for any UI
  change.

## 8. Risks and open questions

| # | Risk / question | Impact | Mitigation |
|---|---|---|---|
| 1 | Hosted passkeys need TLS at the ingress; none today | Hosted passkey sync unavailable until fixed | Add `tls:` block / document an authenticated TLS proxy as prerequisite (security doc already recommends a proxy for remote use) |
| 2 | Linux desktop webview WebAuthn support unverified | Desktop Linux users may not get passkeys | Verify against shipped webkit2gtk; scope desktop passkeys as best-effort or web-only on Linux |
| 3 | Scope: password+passkey vs passkey-only | Recovery and product shape | Decide with product; study assumes password+passkey |
| 4 | Challenge store lifecycle (memory vs DB) | Multi-replica deployments | Start in-memory; DB-backed if multi-instance is a requirement |
| 5 | User enumeration on login/options | Minor | Uniform responses; discoverable credentials |
| 6 | `security.md` doc drift | Public docs wrong today | Separate docs-maintainer pass when the feature ships |
| 7 | Conditional UI browser coverage | UX polish only | Ship button first; conditional UI as follow-up |

## 9. Phasing and effort (rough)

- **Phase 1 (backend, ~3-5 days)**: credential table + challenge store,
  ceremony endpoints behind `features.auth`, session reuse, unit tests
  (verification, store, enumeration behavior, gating). No UI change yet;
  the login/verify endpoint is exercised by unit tests.
- **Phase 2 (frontend, ~2-3 days)**: login-page passkey button, Settings >
  Account passkey enrollment/management, i18n, E2E `auth/passkeys.spec.ts`
  with a virtual authenticator.
- **Phase 3 (hardening + docs, ~1-2 days)**: rate limiting, sign-counter
  edge cases, TLS guidance for hosted deployments, `security.md` and
  feature-status updates.
- Total: roughly 1.5-2 weeks of focused effort by one engineer, using the
  standard Go/TS test conventions. No new infra, no new long-lived secrets.

## 10. Recommendation

Proceed, scoped as **password + passkey login behind `features.auth`**, in
the three phases above. The architecture has every seam needed (auth service,
state payload, login screen, account settings, `auth` E2E project), the
libraries are mature, and the security properties are a net improvement for
hosted deployments. Two decisions to make before implementation:

1. Confirm password+passkey scope (recommended) vs passkey-only.
2. Confirm whether hosted TLS is in scope for the same workstream (it is a
   prerequisite for hosted passkey sync, independent of the feature flag).

If approved, the repo workflow continues with a feature spec under
`docs/specs/webauthn-passkeys/spec.md`, an implementation plan under
`docs/plans/webauthn-passkeys/plan.md`, and task files, per the
spec-driven-development process.
