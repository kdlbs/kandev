---
id: "13-contract-e2e-release-marketplace"
title: "Cross-repository contract E2E, release, and marketplace"
status: in_progress
wave: 4
depends_on:
  [
    "03-protocol-manifest-actions",
    "04-frontend-plugin-registry",
    "05-dynamic-composer-reference-sources",
    "06-plugin-owned-task-lifecycle",
    "07-provider-neutral-git-credentials",
    "08-native-repository-provider",
    "09-native-link-review-surfaces",
    "10-cloud-dc-domain-auth",
    "11-plugin-workflows-watches",
    "12-plugin-ui-native-registrations",
    "12b-github-parity-page",
    "12c-exact-code-host-ui-parity",
    "12d-host-native-task-link-parity",
    "12h-native-create-unlink-indicators-saved-queries",
  ]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 13: Cross-repository contract E2E, release, and marketplace

## Intent

Prove packaged host/plugin integration and publish only after every required capability
and security contract passes. Marketplace registry is final mutation.

## Owned paths

- `apps/backend/cmd/plugin-fixture/`
- `apps/backend/internal/plugins/runtime/testdata/fixtureplugin/main.go`
- `apps/web/e2e/tests/plugins/bitbucket-plugin-contract.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-bitbucket-plugin-contract.spec.ts`
- `plugin-registry/plugins.yaml`
- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md`
- `docs/public/plugins-marketplace.md`
- `docs/public/integrations.md`
- Attached `kdlbs/kandev-plugin-bitbucket` release/package/docs paths.

## Dependencies

Tasks 03 through 12, integrated.

## Acceptance

1. Desktop/mobile E2E covers install/enable, authenticated connection action,
   repository picker, PR import, **Link → Bitbucket Pull Request**, plugin native
   review, composer `#` selection/submission, watch-created task, unload/reload, and
   secret-free errors.
2. Containers E2E proves real HTTPS clone/push through helper leases for host and
   remote executor paths. Packaged plugin installs against released minimum host
   version.
3. A public plugin release with the mandatory internal checksums and no false signing
   or provenance claim precedes the final `plugin-registry/plugins.yaml` entry; public
   docs accurately describe setup, security, compatibility, and live `api_write`
   behavior.

## Verification

```sh
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e --grep "Bitbucket plugin contract"
cd apps/web && pnpm e2e --grep "mobile Bitbucket plugin contract"
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers --grep "Bitbucket plugin contract"
node --test plugin-registry/build-index.test.mjs
```

## Risks

Marketplace publication before host release/package/container evidence breaks minimum-
version compatibility guarantees. Treat every capability-matrix row and all secret
leakage checks as release gates.

## Progress and remaining gates

- The packaged fixture passes the desktop and native-mobile host contract, including
  authenticated action dispatch, repository selection, native Link and review
  surfaces, `#` search with submit-time rejection, plugin-owned task provenance, and
  disable/re-enable cleanup. The layout-owned desktop review tab now adopts the
  registered provider's review title; the exact packaged Bitbucket contract passes
  against a freshly built host bundle.
- Mobile review selection now persists provider-neutral review IDs. Exact selections
  from built-in topbar actions and plugin/built-in review choosers share one path; a
  two-MR GitLab regression E2E proves the selected review is not replaced by the
  provider's primary item.
- Plugin unit/race/vet/build checks and host/all-platform package checksum verification
  pass. The real package installs active on an isolated current development host;
  unconfigured `connection.get`, repository, and pull-request actions return safe empty
  states; disable/re-enable revokes and restores its route; and its desktop/mobile
  workbench, mobile drawer target, and overflow checks pass.
- The real plugin accepts the host's canonical `id`/`key` composer reference, derives
  repository and pull-request identity, rejects mismatched identities, and still
  live-fetches the PR before authorizing submission. A configured Cloud/Data Center
  cross-repository run still requires disposable provider credentials and fixtures.
- The plugin repository now provides an opt-in, fail-fast `make e2e-live` runner for
  one configured product at a time. It installs the real package, saves the token
  connection through the authenticated host action, exercises health/repositories/
  branches/PR/review reads, optionally performs explicitly authorized comment,
  approve/unapprove, and decline mutations, then disconnects in cleanup. Its live
  Playwright config disables traces, screenshots, and video. Cloud API-token and OAuth
  authorization-code/PKCE acceptance passed on 2026-08-05 against the disposable
  `kdbls-kandev/kandev-plugin-live-test` repository. OAuth callback replay was rejected,
  and the credential remained healthy across plugin disable/re-enable. Data Center
  remains pending a disposable target.
- The current Cloud package additionally passed host-native manual linking, exact-branch
  auto-link recovery, idempotent watch creation, persisted-provider branch selection,
  desktop/mobile native Review, and task-topbar CI status. Live 90-second polling tracked
  an actual build-status transition from successful to in-progress and back, and the
  disposable marker was restored to successful.
- Public docs and the marketplace index baseline validate.
- On 2026-08-14 the exact Docker and SSH container credential specs passed against a
  temporary public HTTPS broker endpoint. Both paths cloned and pushed through the
  exact provider lease, exposed no credential in process output or logs, and rejected
  Git after connection-generation revocation. The run also found and fixed a generic
  SSH boundary bug: plugin-provider helpers are now rewritten to the uploaded remote
  `agentctl` path for both checkout preparation and the long-lived instance runtime.
- Kandev `v0.88.0` is released. The independently published
  [`kandev-plugin-bitbucket` v0.2.0](https://github.com/kdlbs/kandev-plugin-bitbucket/releases/tag/v0.2.0)
  declares `min_kandev_version: 0.88.0`; its five platform packages, internal
  checksums, build/verify jobs, and packaged desktop/mobile contract are green.
- The final marketplace registry mutation is prepared in the Kandev marketplace PR.
  Live Data Center remains an explicit post-publication acceptance follow-up because
  no disposable installation was available; Data Center API, auth, context-path, and
  capability behavior remains covered by product-specific fixtures and contract tests.
- Signing is intentionally deferred by
  [ADR-2026-08-01-bitbucket-initial-release-remains-unsigned](../../decisions/2026-08-01-bitbucket-initial-release-remains-unsigned.md).
  The initial package uses mandatory internal checksums, remains visibly unsigned,
  and makes no cryptographic publisher-provenance claim. Signing is no longer a Task
  13 blocker. The released host, package, desktop/mobile, and container gates are now
  satisfied; merging the marketplace registry PR is the remaining publication step.
