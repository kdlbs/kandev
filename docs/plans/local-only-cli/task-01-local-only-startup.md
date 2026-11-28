---
id: "01-local-only-startup"
title: "Implement and verify local-only startup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-LAUNCHER-LOCAL-ONLY-001
acceptance_criteria:
  - AC-LAUNCHER-LOCAL-ONLY-001.1
  - AC-LAUNCHER-LOCAL-ONLY-001.2
  - AC-LAUNCHER-LOCAL-ONLY-001.3
  - AC-LAUNCHER-LOCAL-ONLY-001.4
  - AC-LAUNCHER-LOCAL-ONLY-001.5
system_design:
  - ../../specs/launcher/system-design/local-only-cli.md
---

# Task 01

## Summary

Select a loopback-only default for a standalone launch and verify it at the
socket boundary. Retain existing deployment compatibility.

## In scope

- Shared fallback, launcher probe and access URL, and official container marker.
- Focused regression tests and public documentation in the existing PR.

## Out of scope

- Authentication, deployment migration, UI changes, and GitHub merge actions.

## Acceptance

1. A real standalone process binds only IPv4 loopback and keeps the established
   localhost URL. The behavior survives restart with existing disposable data.
2. Environment and YAML overrides retain their precedence. An existing managed
   service and the official image keep their expected fallback.
3. The README and public guides explain the new default and the upgrade path.
   The tutorial still covers agent setup, the example prompt, and Done.

## Verification

Run Go tests in the backend directory and docs checks from the repository root.
Run heavy commands and their children under systemd with `MemoryMax=8G`.
Use CI for checks that exceed the cap.

```bash
systemd-run --user --wait --pipe --collect -p MemoryMax=8G --working-directory="$PWD/apps/backend" env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE go test -trimpath ./internal/common/config ./internal/launcher -count=1
systemd-run --user --wait --pipe --collect -p MemoryMax=8G --working-directory="$PWD" make build
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use disposable homes and free ports for the native smoke tests. Start the built
CLI with `KANDEV_HOME_DIR`, `KANDEV_BACKEND_PORT`, and `KANDEV_NO_BROWSER=1`.
Unset ambient bind and deployment markers. Check `curl --fail http://127.0.0.1:<port>/ready`,
the printed localhost URL, and `ss -Hltn "sport = :<port>"`. Verify that a
request to the machine's non-loopback address fails. Repeat after restarting
with the same home. Use isolated container or network namespaces for wildcard
tests. For both managed service and official image, prove application readiness
and verify that an explicit YAML loopback bind overrides the wildcard fallback.
Follow the exact-head PR checks and review through completion.

## Files likely touched

- `apps/backend/internal/common/config/{catalog,config,source}.go` and tests
- `apps/backend/internal/launcher/{network,bootstrap,supervisor}.go` and tests
- `Dockerfile`, `README.md`, and affected pages under `docs/public/`

## Dependencies

None.

## Results

- Configuration and launcher packages passed. The NixOS host lacks
  `/bin/bash`, so the launcher suite used a private mount namespace that
  provided that path for the existing launchd-shell fixture.
- The native build passed. A Linux v0.103.1 instance used disposable data.
  The updated binary preserved its workspace ID and localhost origin.
  HTTP readiness and the localhost WebSocket handshake passed. The only
  listener was IPv4 loopback, including after a supervisor restart.
  The source launcher also passed. An isolated service process retained
  wildcard access and honored a YAML loopback override.
- Built the official Dockerfile with the tested Linux binaries. Its container
  reached readiness on a wildcard and honored a YAML loopback override.
  The test mounted the build host's Nix libraries read-only.
- The public docs validators and specification checks passed. The builds,
  checks, and Docker descendants used an 8 GB systemd memory limit.
- Backend CI exposed a date-dependent fixture in the existing interruption
  prompt test. The upstream fix in #5520 gives the scoped markers fixed
  timestamps between the legacy timestamps. The focused orchestrator test
  passed under the 8 GB limit.
- The Docker-in-Docker E2E fixture now sets its backend bind explicitly. The
  outer Docker container still publishes only a host loopback port.
