---
id: "09-contract-e2e-release"
title: "Contract E2E, release, registry pointer"
status: completed
wave: 4
depends_on: ["06-task-linking-bidirectional-sync", "07-issue-watchers", "08-settings-ui-native-registrations"]
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 09: Contract E2E, release, registry pointer

## Intent

Prove the packaged plugin artifact end-to-end against a disposable Kandev instance,
add host-repo contract E2E coverage, cut a release, and add the marketplace catalog
pointer once a valid release exists.

## Owned paths

- `apps/web/e2e/tests/plugins/redmine-*.spec.ts` (host repo, `kdlbs/kandev`)
- `plugin-registry/plugins.yaml` (host repo, added only after step 3 below)
- Attached `yattdev/kandev-plugin-redmine` worktree: release workflow / tag

## Dependencies

Tasks 06, 07, 08.

## Acceptance

1. `make package-host` produces an archive containing `manifest.yaml`, the host
   executable, UI assets, and a generated `checksums.txt`.
2. Installed into a disposable dev Kandev instance (never the developer's primary
   instance/database/credentials): package install/enable, native UI registration,
   and deterministic disable/enable lifecycle checks exercised through the host.
   Redmine-dependent config validation, watcher delivery, duplicate-event idempotency,
   and watcher-created-task cascade behavior are verified in the plugin repository's
   fake-Redmine tests and the generic host plugin contracts; they are not required in
   this repository's packaged-plugin E2E unless a live Redmine fixture is explicitly
   provided.
3. A GitHub Release exists on `yattdev/kandev-plugin-redmine` with the release asset
   named `kandev-plugin-redmine-<version>.tar.gz`, and `min_kandev_version` in the
   manifest is pinned to the Kandev version tested against.
4. `apps/web/e2e/tests/plugins/redmine-*.spec.ts` covers, at the contract level:
   install/enable, safe zero-network action defaults, native settings registration,
   task linking via the shared Link dialog, and disable/enable lifecycle. It must not
   require an outbound Redmine call in CI; watcher-created task appearance remains
   plugin-repository coverage unless a live Redmine fixture is added.
5. `plugin-registry/plugins.yaml` gains a `redmine` entry with `id` equal to the
   manifest `id`, added only after step 3's release exists.

## Verification

```sh
cd apps/web && pnpm e2e:raw -- e2e/tests/plugins/redmine-
node scripts/validate-public-docs.mjs   # if this task touches plugins-authoring.md claims
```

## Risks

Do not test against a developer's primary Kandev instance or real Redmine credentials.
The catalog pointer step is release-gated by design (per the create-kandev-plugin
skill) — do not add it speculatively before a release exists.

## Current status

2026-08-16: Complete. `yattdev/kandev-plugin-redmine` v0.1.0 is published at
https://github.com/yattdev/kandev-plugin-redmine/releases/tag/v0.1.0 with assets
`kandev-plugin-redmine-0.1.0.tar.gz` and `checksums.txt`; `min_kandev_version` is
`0.88.0`. The host contract E2E is
`apps/web/e2e/tests/plugins/redmine-packaged-plugin.spec.ts` and intentionally stays
zero-network: it installs the real release artifact, checks safe unconfigured action
defaults, verifies the native settings and shared Link-dialog contracts, and exercises
disable/enable registration lifecycle. Redmine-dependent watcher creation, sync, and
write-back behavior is covered by the plugin repository's tests rather than by a host
E2E without a live Redmine instance.
