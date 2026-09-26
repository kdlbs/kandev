---
id: "01-version-and-publisher"
title: "Version and npm publisher"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/release/requirements/npm-nightly-channel.md"
---

# Task 01: Version and npm publisher

- **Acceptance:** The helper deterministically emits `X.Y.(Z+1)-nightly.sha{12-hex}` and rejects
  malformed inputs.
- **Acceptance:** The publisher accepts stable release assets or local nightly assets, publishes
  all five exact runtime packages before `kandev`, and applies the requested dist-tag.
- **Acceptance:** Nightly retries accept only already-published versions whose `nightly` tags match.
- **Acceptance:** Scheduled preflight and publishing share one package inventory; an older partial
  publication is recoverable only when its embedded commit is an ancestor of current `main`.
- **Verification:** `node --test scripts/release/nightly-version.test.mjs`
- **Verification:** `node --test scripts/release/publish-npm.test.mjs`
- **Verification:** `cd apps && pnpm --filter kandev exec vitest run src/release-config.test.ts src/service/self_update.test.ts`
- **Files likely touched:** `scripts/release/nightly-version.mjs`, its test,
  `scripts/release/npm-packages.sh`,
  `scripts/release/publish-npm.sh`, `apps/cli/src/release-config.test.ts`,
  `apps/cli/src/service/self_update.test.ts`.
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** spec publication scenarios; current `publish-npm.sh` and
  `package-npm-runtime.sh` behavior.
- **Risks:** npm trusted-publisher OIDC cannot repair dist-tags outside `npm publish`.

## Verification results

- `node --test scripts/release/nightly-version.test.mjs` — passed, 6 tests.
- `node --test scripts/release/publish-npm.test.mjs` — passed, 8 tests.
- `cd apps && pnpm --filter kandev exec vitest run src/release-config.test.ts src/service/self_update.test.ts`
  — passed, 24 tests.
- `bash -n scripts/release/publish-npm.sh` — passed.
