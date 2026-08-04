---
id: "03-backend-channel-foundation"
title: "Backend channel foundation"
status: completed
wave: 2
depends_on: ["01-version-and-publisher"]
plan: "plan.md"
spec: "../../specs/npm-nightly-channel/spec.md"
---

# Task 03: Backend channel foundation

- **Acceptance:** Stable is the durable default, Nightly persists install-wide, and stable/nightly
  caches never leak across channels.
- **Acceptance:** npm metadata resolution validates `dist-tags.nightly`, its exact version record,
  and SemVer before returning a target.
- **Acceptance:** availability handles unequal nightly SHAs without lexical ordering and preserves
  normal stable comparisons.
- **Verification:** `cd apps/backend && go test -v ./internal/persistence ./internal/system/updates`
- **Files likely touched:** `apps/backend/internal/persistence/meta.go`,
  `apps/backend/internal/system/updates/channel.go`, `npm.go`, `service.go`, `poller.go`, and
  colocated tests.
- **Dependencies:** Task 01 defines the recognized nightly version.
- **Parallelism:** sequential in the primary conversation.
- **Inputs:** spec data model, failure modes, persistence guarantees, and channel scenarios.
- **Risks:** stale registry data and SHA prerelease ordering must fail safely.

## Verification results

- `cd apps/backend && go test -v ./internal/persistence ./internal/system/updates` — passed;
  the local PostgreSQL cases skipped because `KANDEV_TEST_POSTGRES_DSN` was unset.
- `Backend Postgres` CI — exercises stable/nightly cache isolation with
  `KANDEV_TEST_POSTGRES_DSN` through `internal/persistence/postgres_meta_test.go`.
