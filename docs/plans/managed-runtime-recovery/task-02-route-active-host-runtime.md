---
id: "02-route-active-host-runtime"
title: "Route active host runtime"
status: done
wave: 2
depends_on: ["01-build-exact-version-foundation"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 02: Route active host runtime

## Acceptance

- Standalone managed-agent launches use the persisted exact active version;
  no-selection launches preserve legacy unversioned behavior.
- Boot/manual probes, model-configuration resolution, profile prompts, and
  sessionless utility prompts use the same host command resolver.
- SSH and container launches receive no host version override; supported native
  binary preference remains unchanged.
- A saved-selection read failure fails the new host launch/probe visibly rather
  than silently selecting npm latest.
- Candidate probes remain able to supply a trusted exact command override
  without changing the active resolver.

## TDD sequence

1. Add lifecycle tests that expect exact standalone argv and unchanged SSH,
   container, native, and no-selection argv.
2. Add host-utility tests for every call site currently reading
   `InferenceConfig().Command` directly.
3. Add the active-version provider to lifecycle and host utility, make launch
   command building context-aware where required, and centralize utility
   command resolution.
4. Wire one store instance from backend startup into lifecycle, host utility,
   and the settings controller; add composition tests.
5. Run focused packages and backend lint.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/*_test.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/hostutility/public.go`
- `apps/backend/internal/agent/hostutility/manager_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/*_test.go`

## Verification

```bash
make -C apps/backend test ARGS='./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/backendapp'
make -C apps/backend lint
```

## Risks

- Cover restart/fresh-command rebuilding as well as initial launches.
- Do not rewrite static `Runtime().Cmd` globally; route only trusted host-local
  dynamic commands.
- Preserve command prefixes, CLI flags, resume arguments, and native preference.

## Output contract

Record RED/GREEN evidence, exact command assertions, checks, and risks in
Results. Update this task and `plan.md` status.

## Results

RED covered exact standalone/worktree command routing, legacy no-selection
behavior, SSH/container exclusions, and selection read errors at the lifecycle
and host-utility boundaries. GREEN verification:

- `rtk go test ./internal/agent/managedruntime ./internal/agent/agents ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/backendapp` — 2,321 tests passed across 7 packages.
- `rtk make -C apps/backend lint` — 0 issues.

One selection store instance is wired through backend startup into lifecycle,
host utility, and update orchestration. Host-local probes, model resolution,
profile prompts, sessionless prompts, and standalone launches share the exact
version resolver. Remote/container command ownership and native preference are
unchanged.
