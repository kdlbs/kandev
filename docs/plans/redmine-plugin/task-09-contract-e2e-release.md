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
   instance/database/credentials): config validation, permission failures, lifecycle
   restart, watcher/event delivery, and native UI registration all exercised and
   passing, including duplicate-event idempotency and a plugin disable/uninstall data
   lifecycle check (disable preserves state; uninstall removes it and cascades watcher
   task-tree deletion).
3. A GitHub Release exists on `yattdev/kandev-plugin-redmine` with the release asset
   named `kandev-plugin-redmine-<version>.tar.gz`, and `min_kandev_version` in the
   manifest is pinned to the Kandev version tested against.
4. `apps/web/e2e/tests/plugins/redmine-*.spec.ts` covers, at the contract level:
   install/enable, connection settings round-trip, task linking via the shared Link
   dialog, and a watcher-created task appearing — mirroring
   `apps/web/e2e/tests/plugins/bitbucket-*.spec.ts`'s coverage shape.
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
