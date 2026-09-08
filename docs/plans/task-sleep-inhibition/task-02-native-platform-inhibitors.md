---
id: "02-native-platform-inhibitors"
title: "Native platform inhibitors"
status: done
wave: 2
depends_on: ["01-lifecycle-service"]
plan: "plan.md"
spec: "../../specs/platform/requirements/task-sleep-inhibition.md"
---

# Task 02: Native platform inhibitors

## Acceptance

- Darwin, Windows, Linux, and fallback build-tagged implementations satisfy the lifecycle service's `Inhibitor`/`Lease` contracts without requesting display wakefulness.
- Each supported implementation releases and joins its owned OS resource, and unexpected helper/request failure reaches `Lease.Done()`.
- Linux system-bus/logind absence and unsupported platforms map to stable non-fatal issue codes; Darwin and Windows implementations compile from Linux CI.

## Verification

```bash
cd apps/backend && go test ./internal/system/sleepinhibition
```

```bash
tmp_dir=$(mktemp -d) && trap 'rm -rf "$tmp_dir"' EXIT && cd apps/backend && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go test -c ./internal/system/sleepinhibition -o "$tmp_dir/sleepinhibition-darwin.test" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c ./internal/system/sleepinhibition -o "$tmp_dir/sleepinhibition-windows.test.exe"
```

## Files likely touched

- `apps/backend/internal/system/sleepinhibition/inhibitor.go`
- `apps/backend/internal/system/sleepinhibition/inhibitor_darwin.go`
- `apps/backend/internal/system/sleepinhibition/inhibitor_linux.go`
- `apps/backend/internal/system/sleepinhibition/inhibitor_windows.go`
- `apps/backend/internal/system/sleepinhibition/inhibitor_other.go`
- `apps/backend/internal/system/sleepinhibition/inhibitor_linux_test.go`
- `apps/backend/internal/system/sleepinhibition/inhibitor_darwin_test.go`
- `apps/backend/internal/system/sleepinhibition/inhibitor_windows_test.go`
- `apps/backend/go.mod`
- `apps/backend/go.sum`

## Dependencies

Task 01.

## Parallelism

Sequential; it implements shared interfaces and changes the Go module graph.

## Inputs

- Spec section: Failure modes.
- Plan section: Native platform implementations.
- Platform contracts: macOS `caffeinate -i`, Windows `SetThreadExecutionState`, and `org.freedesktop.login1.Manager.Inhibit`.

## Risks

- Windows acquire/release calls must execute on the same locked OS thread.
- macOS child cleanup must handle both requested termination and prior unexpected exit without double-waiting.
- Do not make native API availability a prerequisite for backend startup or task execution.

## Output contract

Report adapters, dependency changes, cross-platform compile evidence and artifact cleanup, files changed, blockers/risks, and synchronized task/plan status.

## Results

- Added build-tagged macOS `caffeinate -i -w`, Windows thread-owned execution
  state, Linux logind D-Bus, and unsupported-platform adapters with injected
  seams.
- Linux adapter tests passed:
  `cd apps/backend && go test ./internal/system/sleepinhibition -count=1`.
- Compile-only Darwin and Windows artifacts were produced successfully with
  `CGO_ENABLED=0 GOOS=darwin/windows go test -c`; temporary artifacts were
  written outside the repository.
