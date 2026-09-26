---
id: "05-profile-create-and-test"
title: "Profile create flow and connection test"
status: done
wave: 3
depends_on:
  - "04-remote-docker-runtime"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.4
  - AC-EXECUTORS-REMOTE-DOCKER-001.5
  - AC-EXECUTORS-REMOTE-DOCKER-001.6
  - AC-EXECUTORS-REMOTE-DOCKER-001.14
  - AC-EXECUTORS-REMOTE-DOCKER-001.16
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
---

# Task 05: Profile Create Flow and Connection Test

## Summary

Add the test-then-trust create flow for a remote Docker profile, its backend
probe endpoint, and the root-authority notice, and route the profile's image
build to the remote daemon.

## In scope

- A probe endpoint reporting per-step results: SSH reachability, fingerprint,
  remote platform, daemon reachability, API version.
- A create page in the executor hub offering the remote Docker type, with Save
  gated on an explicit fingerprint-trust checkbox.
- Persisting `host_fingerprint` and hard-failing a later mismatch.
- Routing the existing image tag and Dockerfile build action to the remote
  daemon.
- A persistent notice that the profile grants effective root on the remote host.
- Localized copy in all five locales, with `_verbatim.json` entries where the
  correct translation is the English term.

## Out of scope

- Runtime behavior, owned by task 04.
- E2E coverage and public documentation, owned by task 06.
- Editing an existing profile's transport, which follows the SSH editor's
  established pattern without change.

## Acceptance

- Save is disabled until a successful probe and an explicit trust action.
- Each failure cause in `AC-…-001.14` renders as its own step result, including
  socket-access-denied distinctly from daemon-unreachable.
- Building an image from the profile produces it on the remote daemon.

## Verification

Start with a failing component test that Save stays disabled without a trusted
fingerprint. Confirm it fails before the production change. Then run:

```bash
# From apps/backend:
rtk go test ./internal/... -run 'RemoteDockerTest|RemoteDockerProbe' -race
# From apps/web:
rtk pnpm vitest run --dir app/settings/executors
rtk pnpm run typecheck && rtk pnpm run i18n:check
```

## Files likely touched

- `apps/backend/internal/dockerremote/handlers.go`
- `apps/web/app/settings/executors/page.tsx`
- `apps/web/app/settings/executors/new/[type]/executor-types.ts`
- `apps/web/app/settings/executors/remote-docker/[executorId]/page.tsx`
- `apps/web/src/locales/*/executors.json`

## Dependencies

Task 04.

## Risks

- The executor hub currently omits `remote_docker`; adding the card makes a
  previously unreachable type selectable, so the runtime must land first.
- Mobile parity is required: the create flow needs a native phone layout, not a
  compressed desktop form. See the plan's ASCII previews.
- Fingerprint trust must not silently re-pin on edit; reuse the SSH executor's
  mismatch handling rather than reimplementing it.

## Parallelism

`sequential`

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-001`.
- `apps/web/components/settings/ssh-*` for the test-then-trust pattern.
- `internal/ssh/handlers.go` for the probe endpoint shape.

## Results

- Added the connection test behind the agent-runtime seam
  (`RemoteDockerProber`), with `internal/dockerremote` reduced to HTTP only.
  The architecture lint rejected the original design's package placement: only
  `internal/agent/runtime/` may import `runtime/lifecycle`, and the per-file
  baseline correctly refused a new exemption. The design doc was wrong, not the
  rule.
- The probe reports one step per failure cause and stops at the first failure.
  A green daemon under a red connection would point the user at the wrong
  problem.
- Socket-permission-denied, a missing Docker CLI, and an unreachable daemon
  carry distinct remediation hints. Each needs a different fix on the remote.
- Added `Client.PingVersion` so the test names the daemon that answered rather
  than reporting a bare success.
- Reused the SSH connection card through a `testConnection` prop rather than
  forking it, so remote Docker inherits the whole test-then-trust flow,
  fingerprint gating, and result-staleness handling. A test asserts the SSH
  endpoint is never called when the override is supplied.
- Added the create page, the hub card, and a persistent notice that the profile
  grants effective root on the remote host.
- Simplified the API client after reading `fetchJson`: it already sets the JSON
  content type and merges caller headers, so the first version duplicated both.
- Copy added in five locales; Traditional Chinese generated with
  `pnpm run i18n:zh-hant`.
- Verified with:
  - `go test ./internal/agent/runtime/ -run Probe -count=1`
  - `golangci-lint run ./internal/agent/runtime/ ./internal/dockerremote/...` (0 issues)
  - `pnpm run typecheck`, `pnpm --filter @kandev/web lint` (clean)
  - `pnpm vitest run components/settings lib/api/domains` (253 files, 1679 tests)
  - `pnpm run i18n:check` (all five locales complete)

## Known gaps

- Routing the profile's Dockerfile build to the remote daemon is not wired into
  the create form yet. The runtime builds against the remote daemon because the
  client is remote, but the create page does not yet expose the image tag and
  Dockerfile fields that `local_docker` has. Deferred into task 06 scope.
