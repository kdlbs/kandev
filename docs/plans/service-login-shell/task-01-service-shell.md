---
id: "01-service-shell"
title: "Implement persistent service shell selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CLI-SERVICE-LOGIN-SHELL-001
acceptance_criteria:
  - AC-CLI-SERVICE-LOGIN-SHELL-001.1
  - AC-CLI-SERVICE-LOGIN-SHELL-001.2
  - AC-CLI-SERVICE-LOGIN-SHELL-001.3
  - AC-CLI-SERVICE-LOGIN-SHELL-001.4
  - AC-CLI-SERVICE-LOGIN-SHELL-001.5
  - AC-CLI-SERVICE-LOGIN-SHELL-001.6
  - AC-CLI-SERVICE-LOGIN-SHELL-001.7
system_design:
  - ../../specs/cli/system-design/service-login-shell.md
---

# Task 01: Implement persistent service shell selection

## Summary

Implement the complete opt-in path from service installation through startup
and Apply update. Preserve direct startup and the existing shared PATH helper.

## In scope

- Parse both shell flag forms and an explicit `none` reset. Track argument presence.
- Validate Bash/Zsh executable paths and Darwin user-service eligibility.
- Preserve selection from a recognized managed plist during manual reinstallation.
- Persist and round-trip the selection through all metadata and intent types.
- Render the internal action and replace processes through the selected login shell.
- Restore explicit service environment values after shell initialization.
- Reject invalid or mismatched state before writes, reloads, or update commands.
- Cover legacy direct-start installations and all supported update install kinds.

## Out of scope

Native launchd acceptance and public documentation belong to Task 02.
System services, other platforms, and changes to agent discovery algorithms are excluded.

## Acceptance

1. The selection, reinstallation, reset, default, and update cases satisfy AC 001.1,
   001.2, 001.5, and 001.7. Failures preserve the existing service definition.
2. Shell exports reach real discovery and execution consumers under AC 001.3 and
   001.4. Configured values and account identity remain correct.
3. The internal action is hidden, cannot recurse, and uses process replacement
   under AC 001.6. Portable tests prove argv safety and failure propagation.

## TDD sequence

Start with `TestServiceShellSelection`: current parsing rejects
`install --shell /bin/zsh`. Record the expected red failure.
Then implement the narrow path and extend the scenario table.

Cover new/legacy installs, omission, both flag forms, empty values, `none`,
missing and non-executable paths, unsupported basenames, other platforms,
system mode, and non-install actions. Test explicit repair of invalid saved state.
Use real XML parsing for structured argv assertions, including metacharacters.

`TestServiceShellSubprocess` must assert fixed shell command text, distinct argv,
correct home, restored service configuration, exported PATH/marker propagation,
exit status, and no recursion. Use fixture processes with bounded contexts.
Never print a complete environment. Native `syscall.Exec` proof belongs to Task 02.

`TestLocalAgentPathLoginShellSeparateTools` uses distinct Node and shim directories.
It must test real `exec.LookPath` discovery and process execution, not only PATH strings.
Retain the Node-in-shims control. Authentication is outside this discovery test.

Update tests must read literal JSON metadata, write an Apply intent, decode it,
plan each package-manager path, and regenerate the selected plist.
Include missing and JSON-null shell fields, invalid strings, reset, and mismatch.
For a mismatch, assert no intent write or package-manager side effect.

## Verification

Run from the repository root. These commands cover both changed packages and
the required documentation/diff gates. Go dependency setup uses the existing module.

```bash
set -e
(cd apps/backend && go test -tags fts5 ./internal/launcher ./internal/system/updates -count=1)
git ls-files -m -o --exclude-standard -- 'apps/backend/*.go' | xargs -r bash scripts/lint-go-changed
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the focused red test before the package command. Do not interpret native
Darwin test skips on Linux as acceptance. Record each command's actual result.

## Files likely touched

- `apps/backend/internal/launcher/service.go`
- `apps/backend/internal/launcher/service_shell.go` (new)
- `apps/backend/internal/launcher/service_shell_darwin.go` (new)
- `apps/backend/internal/launcher/service_shell_other.go` (new)
- `apps/backend/internal/launcher/service_shell_test.go` (new)
- `apps/backend/internal/launcher/service_metadata.go`
- `apps/backend/internal/launcher/service_self_update.go`
- `apps/backend/internal/launcher/service_native_update_test.go`
- `apps/backend/internal/launcher/local_agent_path_test.go`
- `apps/backend/internal/system/updates/install_state.go`
- `apps/backend/internal/system/updates/install_state_test.go`
- `apps/backend/internal/system/updates/apply_test.go`
- Existing renderer tests when their fixtures need the new opt-in cases.

## Dependencies

None. Read the requirement, design, and ADR before implementation.

## Risks

Do not expose inherited secrets through generated argv. Do not trust user metadata
for system-account selection. Preserve existing supervisor and update guards.
Keep shell execution behind the explicit Darwin user-service boundary.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/cli/requirements/service-login-shell.md)
- [Design](../../specs/cli/system-design/service-login-shell.md)
- [Decision](../../decisions/2026-10-26-opt-in-service-login-shell.md)
- `local_agent_path_test.go` for real child discovery fixtures.
- `service_identity.go` for structured managed-plist parsing conventions.
- `TestWriteApplyIntentPreservesNativeNoBootStart` for metadata continuity.
- [Diagnostic evidence](plan.md#evidence-and-root-cause).

## Results

Implemented the install, managed reinstallation, reset, launchd argv, metadata,
Apply preflight, package-manager update, login-shell runner, and real local-tool
discovery coverage. The shell option remains opt-in; direct startup is preserved.

Verification passed on Linux:

- `go test -trimpath -tags fts5 ./internal/launcher ./internal/system/updates -count=1`
- `git ls-files -m -o --exclude-standard -- 'apps/backend/*.go' | xargs -r bash scripts/lint-go-changed`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

The macOS-only `syscall.Exec` and launchd acceptance run belongs to Task 02.
