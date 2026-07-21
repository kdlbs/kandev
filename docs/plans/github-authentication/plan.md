---
spec: docs/specs/integrations/github-authentication.md
created: 2026-07-19
amended: 2026-07-21
status: draft
---

# Implementation Plan: Workspace GitHub Authentication

## Overview

Tasks 01-10 delivered workspace PAT/CLI/App-installation routing, personal identity, renewable
executor credentials, and workspace-aware services. Tasks 11-16 delivered an unpublished singleton
GitHub App registration. This amendment replaces that singleton with a registration catalog chosen
per workspace, adds import/create/select onboarding, moves the UX out of System Settings, and makes
callbacks, webhooks, runtime clients, and personal OAuth registration-aware.

The branch is eight commits behind and nineteen commits ahead of `origin/main`. Resolve the known
merge conflict first, then rewrite the unpublished schema before building dependent runtime and UI
layers. Released `legacy_shared` migration behavior remains unchanged.

## Architecture

`github_app_registrations` is a deployment catalog, not a global active identity. Every registration
is manifest-created or imported and has an encrypted generation bundle. A workspace App connection
stores both registration ID and installation ID. Reusing a registration is explicit and shares
root App credentials; separate registrations provide independent work/personal trust boundaries.

The manifest flow preallocates a registration UUID so all generated callback and webhook URLs are
registration-specific. Import verifies an existing App before publishing it to the catalog.
Runtime clients are resolved from a registry keyed by registration ID and generation. Webhook paths
select one candidate registration and verify its HMAC before parsing or deduplicating.

## Conflict Resolution

Merge the latest `origin/main` before production edits. Resolve the current conflict as follows:

1. `apps/web/components/github/github-settings.tsx`: preserve upstream repository-scope UI behavior,
   then keep workspace-keyed GitHub auth wiring. Do not preserve the singleton System Settings
   handoff because Task 18 replaces it.

`apps/web/e2e/helpers/api-client.ts` and `docs/specs/INDEX.md` currently auto-merge. Confirm their
upstream additions and this branch's GitHub authentication entries are both retained after merging.

Do not resolve conflicts by checking out either side wholesale. Confirm `git ls-files -u` is empty
and run frontend typecheck before Task 12.

## Backend

### Registration Persistence

Rewrite the unpublished singleton DDL in `apps/backend/internal/github/store.go`:

- replace `github_app_registration` with plural `github_app_registrations` and the fields in the
  spec;
- replace singleton flow-head state with workspace-bound flows carrying a preallocated
  registration ID;
- add `app_registration_id` constraints/FKs to workspace connections, user connections, and auth
  flows;
- make webhook delivery identity composite by registration;
- retain the released old-workspace `legacy_shared` seeding path, but add no migration from the
  unpublished singleton tables.

Replace `deployment_app_store.go` with registration-ID repository operations. Secret IDs are
registration/generation-specific and deletion is restricted by all workspace and personal
references. Remove the unpublished `GitHubAppConfig`, `KANDEV_GITHUB_APP_*` bindings, singleton
source resolver, and configuration tests; there is no compatibility registration or fallback.

### Create And Import Protocol

Generalize `deployment_app_manifest.go` and `deployment_app_conversion_client.go` around a
preallocated registration ID. Generated webhook, redirect, setup, and OAuth callback URLs include
that ID. The manifest visibility is user-selected and private by default. Preserve exact
permission/event policy, owner-specific GitHub submission endpoints, one-hour single-use state,
public-origin validation, response bounds, and secret-safe errors.

Add an import verifier that accepts bounded credential material, builds an App JWT, verifies
`GET /app` identity/owner/slug, validates the configured callback/webhook policy as far as GitHub's
API exposes it, and atomically stores only after verification. Missing settings become actionable
diagnostics; duplicate host/App ID returns the known registration ID.

### Runtime Registry

Replace the one `DeploymentAppRuntime` generation in `service_app_auth.go`,
`deployment_app_config.go`, and backend application wiring with a concurrency-safe registry keyed
by registration ID. Loading or invalidating one registration must not swap another. Include
registration ID/generation in installation-token cache, resolver principal, credential broker, and
singleflight keys. Runtime App auth resolves only catalog records loaded from encrypted bundles.

Startup independently loads every valid catalog entry, reports invalid entries without preventing
PAT/CLI startup, reconciles orphan secret bundles, and hot-adds a successfully created/imported
registration without restarting Kandev.

### Webhook Routing

Change webhook handling to accept a route registration ID. Load exactly one webhook secret, verify
HMAC, then parse and claim `(registration_id, delivery_id)`. Installation/repository/user events may
only mutate workspace/personal rows whose registration ID and external installation/user identity
match. Registration-specific valid failures update only that registration's health.

### Workspace App Lifecycle API

Replace deployment singleton controllers/services with catalog list, manifest start/callback,
import, rename, and guarded delete endpoints. Installation start accepts registration ID; install
and personal callbacks verify the route ID against hashed flow state. A successful installation
atomically swaps workspace connection/generation and deletes incompatible personal secrets;
failure leaves current auth active.

Extend status, mocks, E2E reset, and stable error mapping. Remove `/settings/system/github-app`
callback redirects and return to `/settings/integrations/github?workspace_id=...`. Registration API
responses expose metadata, source, health, selection, and sharing warnings but no secret or
conversion code.

## Frontend

### API, Types, And State

Replace deployment singleton types/hooks with registration catalog contracts in
`apps/web/lib/types/github.ts`, `lib/api/domains/github-auth-api.ts`, and
`hooks/domains/github/`. Catalog queries are workspace-keyed. Add create-manifest, import, rename,
delete, select/install operations and stable callback-result parsing. Remove the System Settings
route, sidebar entry, page, and deployment-only hook/components.

Workspace status carries selected registration metadata. Changing active workspace clears catalog,
status, actor, health, and in-progress form state before refetch so no prior workspace identity
flashes.

### Workspace Authentication UX

Refactor `github-connection-dialog.tsx` into a method list rather than segmented tabs. PAT and CLI
keep focused forms with matched control heights. GitHub App first explains use cases and trade-offs,
then offers known registrations, **Add existing App**, and **Create new App**. Selection leads to an
explicit installation handoff; importing/creating alone never changes workspace auth.

Move owner, public URL, permission dialog, manifest submission, and callback status UI from System
Settings into workspace GitHub settings. Add the imported-App instruction/form flow with exact
copyable URLs, GitHub settings locations, required permissions/events, bounded secret inputs, and
validation. Visibility defaults private and explains public installability versus Marketplace and
repository grants. Known shared registrations disclose their shared root identity/revocation
boundary.

`Workspace automation` copy accurately distinguishes Kandev-managed automation from unmanaged
executor overrides. Permission chips become one details button/dialog. `My GitHub identity` is
connectable only for App automation; PAT/CLI show that their verified human automation identity is
also used for personal actions without presenting a selector.

### Mobile Contract

Desktop and mobile use the same catalog and forms. Mobile presents method selection and onboarding
as a single-column page/sheet with one scroll owner, safe-area spacing, 44px controls, no fixed
footer, and no horizontal overflow. Copyable URLs and long App names wrap; secret fields and primary
actions never overlap. External GitHub navigation returns to the initiating workspace route.

## Tests

- `deployment_app_store_test.go` (renamed as appropriate): multiple registrations, secret-generation
  compensation, per-registration deletion restriction, restart reload, and no singleton migration.
- `store_connections_test.go` and `personal_connection_repository_test.go`: registration FK
  invariants, atomic connection switch, personal token deletion, generation revocation, workspace
  delete, copy exclusion, and released legacy migration.
- `deployment_app_manifest_test.go` and conversion/import tests: route-specific URLs, default private
  and explicit public manifests, owner URLs, exact policy, state replay/expiry, response bounds,
  duplicate import, GitHub identity mismatch, and no secret errors.
- `service_app_auth_test.go`, resolver, token-cache, and broker tests: concurrent different
  registrations, intentional reuse, hot add/invalidate, registration-keyed caches, and no
  cross-registration fallback.
- `webhook_service_test.go`: wrong route/signature, composite dedupe, matching registration plus
  installation, health isolation, suspension/deletion/repository changes, and personal revocation.
- Controller/service tests: catalog CRUD, create/import/install callbacks, stale flow, guarded
  delete, status metadata, mock/reset isolation, and secret-free JSON.
- Frontend unit tests: workspace switching, method comparison, known registration selection,
  import/create validation, private/public disclosure, permissions dialog, callback outcomes,
  matched control heights, and conditional personal identity.

## E2E Tests

Update the GitHub authentication suites rather than retaining System Settings registration tests:

- workspace A creates a private App and workspace B imports another App; each status and installation
  remains isolated;
- two workspaces intentionally select the same known registration and show the shared-root warning
  while retaining different installation/repository status;
- creation/import cancellation and invalid/replayed callbacks preserve current automation;
- switching Apps removes incompatible personal identity;
- desktop and mobile complete PAT, CLI, select, import, create, install, and permissions flows with
  no clipping, overlap, or horizontal overflow.

Real GitHub staging remains necessary for one manifest conversion, imported-App verification,
installation, OAuth, and correctly signed webhook delivery.

## Public Documentation

Update `docs/public/integrations.md` and `docs/public/configuration.md` for method choice,
registration reuse versus strict isolation, import/create instructions, private/public meaning,
callback/webhook paths, and permissions/events. Delete the unpublished App environment variables and
instructions rather than documenting them as a supported path. Reconcile `docs/ARCHITECTURE.md`,
`apps/backend/AGENTS.md`, feature coverage metadata, and screenshots/terminology that still claim
one deployment App or System Settings ownership.

## Implementation Waves

Tasks 01-10 remain completed implementation history for the workspace credential foundation.
Superseded singleton Tasks 11-16 are replaced by the pending tasks below.

### Completed Workspace Credential Foundation

- [x] [Task 01: Persistence and legacy migration](task-01-persistence-migration.md) - done
- [x] [Task 02: GitHub App token primitives](task-02-app-token-primitives.md) - done
- [x] [Task 03: Workspace PAT and CLI resolver](task-03-workspace-credential-resolver.md) - done
- [x] [Task 04: App installation and personal OAuth lifecycle](task-04-app-oauth-webhooks.md) - done
- [x] [Task 05: Workspace-aware service routing](task-05-service-routing.md) - done
- [x] [Task 06: Renewable executor credentials](task-06-executor-credentials.md) - done
- [x] [Task 07: Connection API, health, and mocks](task-07-http-health-mocks.md) - done
- [x] [Task 08: Workspace and personal settings](task-08-frontend-settings.md) - done
- [x] [Task 09: Initial E2E and public docs](task-09-e2e-docs.md) - done
- [x] [Task 10: Initial QA and security](task-10-qa-security.md) - done

### Wave 0: Reconcile Main

- [x] [Task 11: Reconcile latest main](task-11-reconcile-main.md) - done

### Wave 1: Persistence Contract

- [x] [Task 12: Registration persistence](task-12-registration-persistence.md) - done

### Wave 2: Onboarding Protocol

- [x] [Task 13: Registration onboarding protocol](task-13-registration-onboarding-protocol.md) - done

### Wave 3: Registration-Aware Runtime (parallel)

- [x] [Task 14: Runtime registration resolution](task-14-runtime-registration-resolution.md) - done
- [x] [Task 15: Registration webhook routing](task-15-registration-webhook-routing.md) - done

Task 14 owns runtime clients, resolver, caches, and broker files. Task 15 owns webhook service and
delivery persistence. They share only Task 12 models and can run independently after Task 13.

### Wave 4: Backend API

- [x] [Task 16: Workspace App lifecycle API](task-16-workspace-app-lifecycle-api.md) - done

### Wave 5: Frontend Contract

- [x] [Task 17: Frontend registration client](task-17-frontend-registration-client.md) - done

### Wave 6: Workspace UX

- [x] [Task 18: Workspace authentication UX](task-18-workspace-authentication-ux.md) - done

### Wave 7: Product Proof (parallel)

- [x] [Task 19: Registration E2E coverage](task-19-registration-e2e.md) - done
- [x] [Task 20: Registration documentation](task-20-registration-documentation.md) - done

Task 19 owns Playwright fixtures/specs. Task 20 owns public and architecture documentation.

### Wave 8: Integrated Review

- [x] [Task 21: Security and QA verification](task-21-security-qa.md) - done

## Final Verification

Run formatting before lint:

```bash
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test
cd apps && rtk pnpm --filter @kandev/web lint
cd apps/web && rtk pnpm e2e:run tests/integrations/github-authentication.spec.ts -- --project=chromium
cd apps/web && rtk pnpm e2e:run --no-build tests/integrations/mobile-github-auth-settings.spec.ts -- --project=mobile-chrome
test -z "$(rg -l 'KANDEV_GITHUB_APP' apps docs/public || true)"
git diff --check
```

Also inspect desktop and mobile Playwright screenshots and verify document horizontal overflow is
zero. The final security pass scans logs, API responses, redirects, executor environments, process
arguments, persisted metadata, and E2E artifacts for PATs, App private keys, App client/webhook
secrets, personal tokens, refresh tokens, and live installation tokens.

## Risks

- Existing runtime construction assumes one swappable App. Any hidden singleton can route the right
  workspace through the wrong key; concurrency and negative isolation tests are mandatory.
- Imported GitHub App settings cannot all be read through GitHub APIs. The guide must separate
  verified identity from user-confirmed callback/webhook policy and keep capability diagnostics
  visible after installation.
- A manifest conversion returns the only generated private key. Persistence failure can leave an
  orphan App on GitHub, so recovery instructions must not claim Kandev can delete it.
- Reusing a registration is weaker isolation than separate Apps. The UI must state the shared root
  credential and revocation boundary before installation.
- The current runtime has no authenticated admin role. Registration catalog visibility and mutation
  remain acceptable only inside the documented trusted-single-user boundary.

## Approval

Awaiting approval of the amended spec, ADR, wave graph, and verification commands before Task 11
starts.
