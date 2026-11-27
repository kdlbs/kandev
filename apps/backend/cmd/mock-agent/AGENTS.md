# mock-agent — scenario library for reproducing agent output locally

Scoped guidance for `apps/backend/cmd/mock-agent/`. The mock agent exposes an ACP surface to the rest of the stack. The `dev` profile sets `KANDEV_MOCK_AGENT=true`; the `e2e` profile sets it to `only` to isolate tests to the mock agent. See `profiles.yaml`.

## Two ways to drive it

For mock-agent scenario creation, directives, emitters, and failure fixtures, read [scenario guidance](SCENARIOS.md).

## Rebuild before running e2e (easy to miss)

The web e2e suite runs the **prebuilt host binary** `apps/backend/bin/mock-agent` (resolved via `PATH`). The `containers` project also uses `bin/mock-agent-linux-amd64`. `global-setup.ts` checks required artifacts against backend sources or a validated immutable build identity. After changing anything under `cmd/mock-agent`:

```bash
make -C apps/backend build                     # host backend and mock agent
make -C apps/backend build-mock-agent-linux    # only for the `containers` project
```

## Tests

`mock_agent_test.go` exercises the top-level handler dispatch. `script_test.go` covers the inline-script parser end-to-end. New scenarios don't typically need a unit test — the assertion is that the scenario name is registered in `scenarioRegistry` and the payload looks right when triggered manually. If a scenario contains conditional logic worth pinning, add a focused test that constructs an emitter with a fake `sessionUpdater` and asserts on the emitted notifications.
