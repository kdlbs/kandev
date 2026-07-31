---
id: "01-preserve-system-service-user"
title: "Preserve system service user"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/native-kandev-cli/spec.md"
---

# Task 01: Preserve System Service User

## Acceptance

- `kandev service install --system --run-as <user>` explicitly selects a valid service account;
  invalid command/flag combinations fail as usage errors.
- Reinstall without `--run-as` preserves the account in an existing root-controlled,
  Kandev-managed systemd unit or launchd plist, independent of the current login.
- A first root-shell install without a non-root `SUDO_USER` requires explicit `--run-as`; root is
  available only as `--run-as root`.
- Service-owned install metadata cannot select a different or more privileged account.
- An existing system-home owner mismatch stops installation before service-file replacement or
  restart. No recursive chown or Git trust mutation occurs.

## TDD Sequence

1. Add parser and identity-resolution tests for valid/invalid `--run-as`, preserve-on-reinstall,
   first-install sudo selection, and explicit root; record RED.
2. Add root-controlled managed-definition readers for systemd and launchd plus account validation.
3. Add owner-UID preflight tests proving mismatch stops before installer side effects; record RED.
4. Wire the resolved identity and preflight into native system installation and metadata writing.
5. Run the focused and package commands below; record GREEN.

## Verification

```bash
cd apps/backend && go test ./internal/launcher -run 'Test.*(RunAs|ServiceUser|SystemServiceIdentity|SystemHomeOwner|InstallSystemd|InstallLaunchd)' -count=1
cd apps/backend && go test ./internal/launcher -count=1
```

## Files Likely Touched

- `apps/backend/internal/launcher/service.go`
- `apps/backend/internal/launcher/service_test.go`
- `apps/backend/internal/launcher/service_metadata.go`
- `apps/backend/internal/launcher/service_metadata_root_unix.go`
- `apps/backend/internal/launcher/service_native_update_test.go`
- a focused OS-specific service identity helper/test file if extraction keeps launcher complexity
  within lint limits

## Dependencies

None.

## Parallelism

Production files are disjoint from Task 02, but execution remains in the primary conversation
unless the user explicitly authorizes delegation.

## Recorded Results

- RED: `TestParseServiceArgsAcceptsSystemRunAs`, `TestInstallSystemdPreservesExistingManagedUser`,
  and `TestInstallSystemdRejectsManagedHomeOwnerMismatchBeforeWrite` failed before the identity
  resolver and owner preflight existed.
- GREEN: targeted identity tests passed 5 cases; `go test ./internal/launcher -count=1` passed 114
  tests.
- The root-controlled systemd/launchd definition is now authoritative on reinstall. First installs
  from a root login require explicit `--run-as`, and system homes must already exist, be real
  directories, and have the selected account's UID.

## Output Contract

Report the RED failures, identity precedence, service-definition trust check, owner preflight
behavior, exact GREEN results, and any platform-specific limitation. Update this task and the plan
status without changing the ADR's security boundary.
