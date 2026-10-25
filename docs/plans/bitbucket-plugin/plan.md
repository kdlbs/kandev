---
spec: docs/specs/integrations/requirements/bitbucket-plugin.md
created: 2026-07-31
status: in_progress
---

# Implementation Plan: Bitbucket Connector Plugin

## Overview

Release generic host seams before plugin behavior: authenticated actions, plugin
registry contracts, dynamic composer sources, richer task ownership, and a provider-
neutral credential broker. Build Cloud/Data Center adapters and plugin workflows only
after those seams land, then prove native task, review, composer, desktop, mobile, and
credential behavior through cross-repository acceptance.

The host remains provider-neutral. `kdlbs/kandev-plugin-bitbucket` owns Bitbucket
payloads, auth, product probes, watches, and UI. The required native hooks remain
**Link → Bitbucket Pull Request**, host-rendered review/status surfaces supplied by
plugin adapters, and composer `#` source search with submit-time authorization.

## Host contracts

- Add additive plugin RPC/SDK/manifest shapes and authenticated action dispatch.
- Add revocable repository-provider, task-action, and review-provider frontend
  registrations; migrate built-ins through the same registry.
- Register/unregister dynamic backend composer reference sources and reauthorize every
  reference at submission.
- Extend plugin-owned task lifecycle and provider-neutral descriptor validation.
- Replace GitHub-only broker mechanics with composite short-lived credential leases.

## Plugin repository

- Bootstrap `kdlbs/kandev-plugin-bitbucket` from official template in its attached
  Kandev workspace; do not nest a clone under this repository.
- Implement separate Cloud and Data Center adapters, workspace-scoped encrypted auth,
  health, capabilities, task/link/Git/watch workflows, and native registrations.
- Publish only after source/package, host/plugin desktop/mobile, and container broker
  acceptance pass; marketplace mutation is last.

## Tests

- Host protocol/action tests cover declaration, normal authentication, resource
  authorization, body caps, timeout/cancellation, response headers, and SDK
  compatibility.
- Registry/task/review tests cover ownership collisions, unload cancellation, complete
  provider descriptors, Link-group actions, layout aliases, desktop/mobile review
  selection, and plugin teardown.
- Composer tests cover dynamic source lifecycle, canonical identity, workspace
  isolation, search and submission authorization, disabled plugins, and generic
  presentation.
- Credential tests cover exact scope, expiry/revocation, refreshed resolution,
  leakage, custom host/path, and local/remote executor helper use.
- Plugin tests cover Cloud/DC fixture mapping, auth rotation/PKCE, capability probes,
  health/retry, watch concurrency/recovery, ownership-safe deletion, and responsive UI.
- Plugin page parity tests compare `/bitbucket` with the first-party `/github`
  list-first structure: scope bar, compact toolbar, bordered rows, focused review URL,
  and native mobile list/detail navigation.

## E2E tests

- Install/enable fixture package; call a declared authenticated action without exposing
  a webhook as a browser RPC.
- Select a plugin provider in native task creation, import/inspect a PR, invoke
  **Link → Bitbucket Pull Request**, then open the plugin native review panel on
  desktop and mobile.
- Select a `#` Bitbucket pull request and prove submitted metadata is authorized.
- Verify watch-created task, unload/reload cleanup, secret-free failure UI, and real
  HTTPS clone/push in the containers project.

## Implementation waves and task files

Wave 0 (parallel-safe, docs/external repository only):

- [x] [task 01 — design package](task-01-design-package.md)
- [x] [task 02 — plugin repository bootstrap](task-02-plugin-repository-bootstrap.md)

Wave 1 (parallel-safe after task 01):

- [x] [task 03 — protocol, manifest, authenticated actions](task-03-protocol-manifest-actions.md)
- [x] [task 04 — frontend plugin registry contracts](task-04-frontend-plugin-registry.md)

Wave 2 (parallel-safe after contract foundations):

- [x] [task 05 — dynamic composer reference sources](task-05-dynamic-composer-reference-sources.md)
- [x] [task 06 — plugin-owned task lifecycle](task-06-plugin-owned-task-lifecycle.md)
- [x] [task 07 — provider-neutral git credential broker](task-07-provider-neutral-git-credentials.md)
- [x] [task 08 — native repository provider task creation](task-08-native-repository-provider.md)
- [x] [task 09 — native Link and review surfaces](task-09-native-link-review-surfaces.md)

Wave 3 (plugin repository after declared dependencies):

- [x] [task 10 — Cloud/DC domain and authentication](task-10-cloud-dc-domain-auth.md)
- [x] [task 11 — task, Git, linking, and watch workflows](task-11-plugin-workflows-watches.md)
- [x] [task 12 — plugin UI and native registrations](task-12-plugin-ui-native-registrations.md)

Wave 3b (corrective UI pass after live evaluation):

- [x] [task 12b — GitHub-parity Bitbucket page](task-12b-github-parity-page.md)

Wave 3c (exact shared interaction parity after user evaluation):

- [x] [task 12c — exact shared code-host UI parity](task-12c-exact-code-host-ui-parity.md)

Wave 3d (native task-link parity after manual evaluation):

- [x] [task 12d — host-native task-link dialog parity](task-12d-host-native-task-link-parity.md)

Wave 3e (corrective host-owned task status after manual evaluation):

- [x] [task 12e — registry-backed shared task status](task-12e-shared-task-status.md)

Wave 3f (corrective host-owned review presentation; parallel with 12e):

- [x] [task 12f — shared change-request detail](task-12f-shared-review-detail.md)

Wave 3g (after 12e and 12f):

- [x] [task 12g — Bitbucket adapters and live parity acceptance](task-12g-bitbucket-status-detail-adapters.md)

Wave 3h (corrective native workflow pass after final manual evaluation):

- [x] [task 12h — native create, unlink, task indicators, and saved queries](task-12h-native-create-unlink-indicators-saved-queries.md)

Wave 3i (corrective sidebar hover parity after manual evaluation):

- [x] [task 12i — shared task-indicator hover summary](task-12i-shared-task-indicator-hover-summary.md)

Wave 4:

- [ ] [task 13 — contract E2E, release, and marketplace](task-13-contract-e2e-release-marketplace.md)

## Current status

Tasks 01–12i are implemented. Task 13 remains the
release/marketplace gate.
Live Cloud evaluation showed that task 12's permanent
desktop queue/review split diverged from the first-party `/github` page despite being
functionally complete; task 12b replaced it with the approved list-first parity model
and passed the live Cloud-connected desktop/mobile suite. User evaluation then found
that its equal **Task**/**Review** buttons and intermediate launch modal still diverged
from GitHub/GitLab interaction behavior. Task 12c replaced copied presentation and
plugin-specific launch/review placement with shared host primitives and native task
creation. Its first pass passed the complete host suite (1,010 files / 7,712 tests) and
seven live Cloud desktop/mobile checks. Manual testing then found that scope pills did
not commit their filter into the visible query and plugin-drawn SVGs did not match the
host's semantic icons. The corrected evaluation package commits scope selections
into the visible query, uses host-owned semantic Tabler icons, and passes eight live
Cloud desktop/mobile checks. It also registers the exact **Bitbucket Pull Request**
submenu label and invokes the shared host task-link dialog. Live evaluation caught the
modal host outside the host toast-provider tree; the corrected topology now mounts
plugin modals inside `AppShell`, and a production-topology regression test protects the
real form. Empty validation, authenticated Cloud linking, close-on-success, toast,
desktop geometry, and mobile no-overflow behavior pass against the disposable account.
Version 0.1.7 also proves exact preset glyphs, persisted-provider branch selection,
manual linking, exact-branch auto-link recovery, restart-safe watch associations, native
desktop/mobile Review, and live CI polling. The CI marker was changed
`SUCCESSFUL -> INPROGRESS -> SUCCESSFUL`; each 90-second poll updated the shared host
glyph and the remote fixture was restored.
Manual evaluation then established that the topbar body and Review panel were still
plugin-owned approximations and that no Bitbucket CI chip existed above the composer.
Tasks 12e–12g replaced those approximations with registry-backed, host-owned surfaces
used by GitHub itself. Package 0.1.10 passed desktop/touch live acceptance: the shared
topbar/composer anatomy, shared review detail, manual linking, exact-branch auto-link,
and a real `SUCCESSFUL -> INPROGRESS -> FAILED -> SUCCESSFUL` pipeline cycle all passed;
the fixture was restored.
Task 12h closed the final manual-evaluation gaps. Package 0.1.12 proved native Create PR
through the registered provider after a real Git push, shared unlink with reload-safe
detachment, workspace-hydrated sidebar indicators, and saved-query persistence and
restoration. The native Create PR control remains Kandev's existing Changes-panel
control; no duplicate plugin-specific create button was added.
Task 12i removed the association indicator's count-only tooltip. Registered providers
now reuse GitHub's host-owned structured pull-request summary and load task detail only
when the desktop indicator opens; the existing mobile Status/Review surfaces remain the
touch path. A manual follow-up also moved task-indicator color derivation into the shared
provider-neutral status palette, so normalized pipeline/review state changes the glyph
without provider CSS tokens. Unit, packaged-plugin desktop E2E, mobile contract E2E,
typecheck, and lint pass. The public authoring reference now includes a code-host hook
surface map and focused captures of the shared dashboard, repository picker, Link flow,
task status, CI, Review, and composer reference search.
The packaged generic host contract passes on desktop and mobile. The actual package
passes its unconfigured action, disable/re-enable, desktop, and mobile lifecycle checks;
its canonical composer reference rehydrates repository/PR identity and performs live
submit-time authorization. Plugin unit, race, build, and five-platform archive checks
pass. Cloud acceptance is complete for the available disposable target. On 2026-08-14,
the Docker and SSH container specs both passed real HTTPS clone, push, secret
non-disclosure, and connection-generation revocation through the provider-neutral
credential broker. That run also closed the generic remote-helper path used by future
plugin providers on SSH executors.

Kandev `v0.88.0` and `kandev-plugin-bitbucket` `v0.2.0` are published; the plugin
declares `min_kandev_version: 0.88.0`, and exact build, verify, packaged-host, checksum,
desktop, and mobile gates are green. Task 13 now waits only for the marketplace registry
PR to merge. A configured live Data Center run remains a documented follow-up because
no disposable target was available; Data Center behavior is covered by its separate
adapter fixtures and contract suite. The initial plugin release intentionally follows
the current checksum-verified, unsigned marketplace contract under
[ADR-2026-08-01-bitbucket-initial-release-remains-unsigned](../../decisions/2026-08-01-bitbucket-initial-release-remains-unsigned.md);
no signature or cryptographic publisher-provenance claim is made.

## Risks

- Full parity is complete only when every capability-matrix row passes; alpha sideloads
  may be partial but must be marked partial.
- BYO OAuth registration and Data Center network/version variation require clear
  failure states, fixture coverage, and a release-time compatibility matrix.
- Generic host seams must pass a second-provider test: Bitbucket types, URL parsing,
  and auth rules in host code are a design failure.
- Cross-repository release ordering is strict: host contract release precedes plugin
  compatibility gate, package release, and marketplace entry.
- Same-version development copies keep the plugin bundle URL stable. Live evaluation
  must use a fresh document or hard reload after UI-only asset replacement; released
  plugin updates change the manifest version and therefore the bundle URL.
