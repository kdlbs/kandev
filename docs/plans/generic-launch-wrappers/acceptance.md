# Generic wrapper acceptance

Dates: 2026-10-18 through 2026-10-21. Source branch: `feature/investigate-pi-agent-6ac`.
The implementation is under review. PR #4538 stays draft. This record does not certify real sandbox enforcement.

## Integration with the current base

The subsequent CI run exposed a stale mixed-backend recovery fixture that listed an instance on an unused fixed port.
Its required retained-wrapper read could not complete. The actual retracked count was zero; the test expected one.
The fixture now serves a versioned wrapper snapshot on a dynamically allocated test endpoint.
The exact failing test reproduced locally. Recovery-summary, duplicate-outcome, and retained-wrapper race checks pass after correction, with the original outcome counts preserved.
The PostgreSQL 18 job separately failed during a Go dependency download from `proxy.golang.org`, before the task repository tests ran.
Fresh CI at `d3f266a13214` passed both the PostgreSQL 18 job and backend shard 2.
The download failure did not require a product or workflow change.


The same CI head exposed a browser-test synchronization error in the workflow move preview.
All four attempts selected a response whose headers reported success but whose body download was cancelled.
The trace also records a successful replacement request and a rendered preview.
The test now waits for a successful `requestfinished` event and reads the body before checking the preview.
All outcome, model, session, and visible-preview assertions remain in place.
This change preserves product behavior and does not change the requirements or system design.
The managed desktop spec passed all three cases. The matching phone drawer case also passed.
Both runs used one worker and disabled retries. Commands from the repository root:

```bash
pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/workflow/workflow-move-preview.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/workflow/mobile-workflow-move-preview.spec.ts -- --retries=0
```

The temporary host launcher supplied Chromium libraries, removed inherited launcher configuration,
and selected an unused E2E port block. The production assets were freshly built at `d3f266a13214`.
Only browser test source changed afterward, so these commands reused those assets.
The old helper passed an isolated local attempt, so that attempt did not reproduce the CI timing.
CI traces provide the failure evidence. Local fixture configuration and a port collision were resolved only in the temporary launcher.
The retained manual instance, evaluation instance, and main instance were not changed.

The 2026-10-20 integration preserves the base branch's terminal lifecycle fields and sanitized probe diagnostics.
The existing wrapper privacy test first failed because the new diagnostic logger retained arbitrary wrapper stderr.
Wrapped probes now log only a bounded failure classification. The regression also checks the relay message.

The base branch added SSH native terminal transport without a wrapper field.
A real-controller regression first demonstrated that a wrapped request started an unwrapped process.
That transport now refuses wrapped startup before sending a start request or recording an operation.
Its unwrapped positive control still starts the process. Local terminal and remote ACP support are unchanged.

Focused wrapper, probe, terminal, and lifecycle race checks passed after integration.
The additional `GenericWrapper|SSHPassthrough|RemotePassthrough` lifecycle race run also passed.
All 12 acceptance-runner safety tests, specification/catalog validation, six-work-order coverage, and public-doc validation passed.
Remote CI and review evidence must be checked on the pushed integration commit.

The user evaluation instance separately exposed an initial startup failure, unavailable recovery with no executor record,
and a Kandev MCP connection blocked by its explicit Greywall policy.
A replacement session completed the sales case, but that does not prove recovery or complete sandbox integration.
The queued Nono follow-up owns investigation of those gaps, supervisor integration, policy selection,
approvals, learning, and confinement tests. This PR does not complete issue #3812.


## CI and base refresh on 2026-10-21

CI completed on `e4fcd18e9ee88402c2da183f8a2ce1641cafe9ef`: 84 checks passed and 16 were skipped.
No check failed or remained pending. No review thread remained unresolved.
All three desktop preview cases and the phone preview case passed on their first attempt.
The full browser report contains 4,025 first-attempt passes, 14 passes after retry, and 40 skipped tests.
The blob audit found 17 retry attempts and no malformed report records. This is not a zero-retry result.

The latest base, `d5387f083850ffd1755854f637feaa428fca90a4`, then conflicted with the preview helper.
The resolution preserves completed-request observation and adds the base's destination-step matching and session readiness check.
It preserves all existing assertions. It does not change a product requirement or design contract.
Automatically merged runtime paths retain wrapper validation, restart state, retained snapshots, and execution-host preparation.

Focused wrapper, recovery-summary, SSH, and resume-projection race checks passed after integration.
Process, utility, API, and wrapper-package race checks also passed.
An expanded selection exceeded its 120-second package budget during the 49-case SQLite state matrix.
That run does not establish a full matrix pass. The focused lifecycle run passed in 7.240 seconds.
Changed-Go lint passed for all 66 PR paths relative to the merged base, with no findings.
All 12 runner safety tests, 63 public-doc tests, public-doc validation, catalog validation, and specification lint passed.

The fresh managed build passed all eight selected desktop cases with retries disabled.
All seven selected phone cases also passed. Both runs used one worker and disabled retries.
Commands from the repository root:

```bash
pnpm --dir apps/web e2e:run --host --project chromium tests/cli-mode/generic-launch-wrappers.spec.ts tests/settings/profile-capability-discovery.spec.ts tests/workflow/workflow-move-preview.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/cli-mode/mobile-generic-launch-wrappers.spec.ts tests/settings/mobile-profile-capability-discovery.spec.ts tests/workflow/mobile-workflow-move-preview.spec.ts -- --retries=0
```

The temporary host launcher supplied Chromium libraries and selected free E2E ports, as described above.
The retained manual instance, evaluation instance, main instance, and deprecated task remain unchanged.
Fresh remote CI must be checked on the pushed merge commit.

## Backend checks

The following focused checks passed from `apps/backend`:

```bash
go test -trimpath ./internal/agent/launchwrapper ./internal/agent/managedruntime ./internal/agent/settings/cliflags
go test -trimpath ./internal/agent/launchwrapper ./internal/agent/runtime/agentctl ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agentctl/server/utility ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'GenericWrapper|BuildAgentCommand_CommandPrefix|Launch_PromotesWorkspaceOnlyExecution|Manager_ProfileCLIFlagTokens|ProfileProbe|ProfileCapability' -count=1
go test -trimpath -race ./internal/agent/launchwrapper ./internal/agent/runtime/environment ./internal/agent/hostutility ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/agentctl/server/utility ./internal/agentctl/server/api -run 'GenericWrapper|LaunchWrapper|ResolveProfile|ProfileProbe|ProfileCapability|ACP.*Cancel' -count=1
go test -trimpath -race ./internal/agentctl/server/process -run 'GenericWrapper|StartOneShot|BuildFinalCommandPreparesOneShot' -count=1
```

A broad process run also exposed a nil-configuration regression in the new stderr filter. The existing stderr-exit test failed with a panic. The filter now preserves that case. `go test -trimpath -race ./internal/agentctl/server/process -run 'GenericWrapper|Stderr|StartOneShot|BuildFinalCommandPreparesOneShot' -count=1` passed after the correction.

The last listed command was run after correcting the one-shot startup path. Its regression test first failed because no wrapper receipt existed. It now proves both initial and continuation execution pass through the wrapper.

Broad hostutility, settings/handlers, runtime/agentctl, and runtime/environment packages passed. Broader API, utility, process, lifecycle, and controller runs did not pass in this host environment. These specific failures also occur on unchanged PR HEAD `e45477c896ae9326324b035d58bddf1e15b1eb5f`:

- `TestOpenCodeInstallPreparesSelectedManagedRuntime`.
- `TestRuntimeObservationRejectsOversizedVersionOutput`.
- `TestAgentctlResolverRemoteBinaryNotFoundErrorNamesPlatformAndEnv`.
- `TestRestartRecoveryRetiresOnlyProvenDeadLoadedConversation`, whose fixtures require unavailable `/bin/sleep`.
- `TestLSPStreamTerminatesWhenCredentialRotates`.
- `TestHandleLSPStreamBridgesFramesAndStopsOwnedProcess`.
- `TestHandleLSPStreamAutoInstallHandsFirstClientMessageToBridge`.
- `TestGitStatusHTTPReturnsRecoveredEnrichedCapture`.
- `TestGitStatusHTTPWarnsOnceAfterContinuousCaptureChanges`.
- `TestGitStatusMultiRetriesOnlyTheRepositoryWithChangedEvidence`.
- `TestWorkspaceGitMalformedNumstatDetailsUnavailable`.
- `TestWorkspaceGitEmptyNumstatDetailsReady`.

A later full process-package run completed in 107 seconds with only the two listed numstat test failures. The lifecycle suite exceeded a four-minute whole-package limit while running Kubernetes tests. It did not report a wrapper test failure. This run is incomplete, not a passing broad-suite result.

The baseline comparison used an isolated source archive. The controller comparison used an overlay containing unchanged HEAD files only. Neither comparison is acceptance evidence for a provider exception. Production implementation and browser builds use current source without an overlay.

Changed-Go lint passed with golangci-lint 2.9.0. A separate temporary lint directory avoided another worktree's lock. The commit hook runs the same lint after the final stderr correction. The changed-file lint command passes every changed Go path to `bash scripts/lint-go-changed`. Expanded command:

```bash
bash scripts/lint-go-changed \
  apps/backend/internal/agent/hostutility/manager.go \
  apps/backend/internal/agent/hostutility/manager_test.go \
  apps/backend/internal/agent/hostutility/profile_capability_cache.go \
  apps/backend/internal/agent/hostutility/profile_probe.go \
  apps/backend/internal/agent/hostutility/profile_probe_test.go \
  apps/backend/internal/agent/hostutility/public.go \
  apps/backend/internal/agent/managedruntime/npm_project.go \
  apps/backend/internal/agent/runtime/agentctl/client.go \
  apps/backend/internal/agent/runtime/agentctl/utility.go \
  apps/backend/internal/agent/runtime/lifecycle/command.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_refresh.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_launch.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go \
  apps/backend/internal/agent/runtime/lifecycle/types.go \
  apps/backend/internal/agent/runtime/lifecycle/utility.go \
  apps/backend/internal/agent/runtime/lifecycle/utility_test.go \
  apps/backend/internal/agent/settings/cliflags/tokenise.go \
  apps/backend/internal/agent/settings/cliflags/tokenise_test.go \
  apps/backend/internal/agentctl/server/api/auth.go \
  apps/backend/internal/agentctl/server/api/identity.go \
  apps/backend/internal/agentctl/server/api/server.go \
  apps/backend/internal/agentctl/server/config/config.go \
  apps/backend/internal/agentctl/server/process/interactive_lifecycle.go \
  apps/backend/internal/agentctl/server/process/interactive_runner.go \
  apps/backend/internal/agentctl/server/process/manager.go \
  apps/backend/internal/agentctl/server/process/manager_managed_git_handoff_test.go \
  apps/backend/internal/agentctl/server/utility/acp_executor.go \
  apps/backend/internal/agentctl/server/utility/acp_executor_process_unix_test.go \
  apps/backend/internal/agentctl/server/utility/codex_app_server.go \
  apps/backend/internal/agentctl/server/utility/handler.go \
  apps/backend/internal/agentctl/server/utility/types.go \
  apps/backend/internal/backendapp/main.go \
  apps/backend/internal/agent/hostutility/profile_environment.go \
  apps/backend/internal/agent/launchwrapper/codec.go \
  apps/backend/internal/agent/launchwrapper/environment.go \
  apps/backend/internal/agent/launchwrapper/executable.go \
  apps/backend/internal/agent/launchwrapper/prepare.go \
  apps/backend/internal/agent/launchwrapper/prepare_test.go \
  apps/backend/internal/agent/launchwrapper/spec.go \
  apps/backend/internal/agent/launchwrapper/spec_test.go \
  apps/backend/internal/agent/runtime/agentctl/generic_wrapper_test.go \
  apps/backend/internal/agent/runtime/agentctl/launch_wrapper.go \
  apps/backend/internal/agent/runtime/environment/profile.go \
  apps/backend/internal/agent/runtime/environment/profile_test.go \
  apps/backend/internal/agent/runtime/lifecycle/generic_wrapper_passthrough_test.go \
  apps/backend/internal/agent/runtime/lifecycle/generic_wrapper_recovery_test.go \
  apps/backend/internal/agent/runtime/lifecycle/generic_wrapper_utility_test.go \
  apps/backend/internal/agent/runtime/lifecycle/launch_wrapper_recovery.go \
  apps/backend/internal/agent/runtime/lifecycle/utility_environment.go \
  apps/backend/internal/agentctl/server/api/generic_wrapper_configure_test.go \
  apps/backend/internal/agentctl/server/api/generic_wrapper_test.go \
  apps/backend/internal/agentctl/server/api/launch_wrapper.go \
  apps/backend/internal/agentctl/server/process/generic_wrapper_interactive_test.go \
  apps/backend/internal/agentctl/server/process/generic_wrapper_test.go \
  apps/backend/internal/agentctl/server/process/launch_wrapper.go \
  apps/backend/internal/agentctl/server/utility/generic_wrapper_failure_test.go \
  apps/backend/internal/agentctl/server/utility/generic_wrapper_test.go \
  apps/backend/internal/agentctl/server/utility/launch_wrapper.go
```

## Browser and documentation checks

Desktop Chromium: the full settings and terminal selection passed (4 tests in 47.6 seconds). After adding the ACP chat restart case and refining the terminal fixture, both terminal/ACP tests passed again (2 tests in 23.1 seconds). All 5 mobile settings tests passed. The mobile terminal retry passed (1 test in 14.8 seconds), including touch targets, input, restart, and process cleanup. All 11 unique selected browser tests now pass. Tests use an unrelated temporary launcher with a path containing spaces. The launcher verifies the runtime working-directory reference and records its PID before it executes the child. It does not depend on the built-in utility allowlist.

Initial browser setup was blocked by inherited Kandev settings and Chromium shared-library lookup. The retry removes inherited Kandev/agentctl overrides and uses host-installed Nix libraries for the test process only. No manual runtime settings or processes were changed.

The terminal test uses short markers that fit the phone viewport. It waits for idle session state through the API because the desktop workflow stepper is absent on the phone.

The first web typecheck exhausted Node's default 4 GiB heap. `NODE_OPTIONS=--max-old-space-size=8192 pnpm --dir apps/web run typecheck` passed. Targeted ESLint for the six changed browser test/helper files also passed.

The specification validator, full spec lint, 36 spec-lint tests, public-doc tests, and public-doc validation passed. Local PR documentation coverage checks passed for all six work orders. Architecture lint passed. The commit hooks also validate the final documents.

## Fresh Pi/Greywall discovery

On 2026-10-18, the user authorized tool installation. Installed Pi 1.3.0, pi-acp 0.0.34, Greywall 0.3.7, and Greyproxy 0.4.5 in a new private directory outside the repository and `/tmp`. The Greywall and Greyproxy release archives passed their published SHA-256 checksums. The npm installation used exact package versions, disabled lifecycle scripts, and retained its lockfile in the private installation. No global npm package or service was changed.

The host runs NixOS 26.05, Linux 6.18.49, and Node 24.19.0. The installation uses the existing Bubblewrap 0.11.2 and an isolated Nix installation of socat 1.8.1.3. Greywall successfully launched a Node subprocess. This smoke check does not certify sandbox enforcement.

Built `acpdbg` and `agentctl` from commit `23eee8e8b6b673dd1b4921d542b75e30ef7b4142` with `go build -trimpath`, no Go overlay, and no inherited build flags. Fresh disposable homes used only a synthetic OpenAI key. No real provider credential was loaded and no model prompt was sent.

Both direct and Greywall-wrapped `acpdbg` discovery completed `initialize` and `session/new`. Each returned 44 models and 5 modes. The captured frames contained zero `session/prompt` requests.

A separate authenticated agentctl control process then repeated direct and wrapped discovery through `/api/v1/inference/probe`. Both returned success, 44 models, and 5 modes. The inner command used the managed npm marker and the pinned adapter. The generic wrapper envelope used `{{kandev.work_dir}}` and `{{kandev.managed_npm_dir}}`. The wrapped run produced exactly one nonce-bearing launcher receipt before executing Greywall. No provider-specific resolver exception was used.

The sandbox received explicit grants for installation reads and the disposable home, npm cache, temporary directory, working directory, and managed npm directory. It did not use learning mode, watch mode, a provider profile, or an allow-all network rule. Proxy endpoints used dynamically allocated local ports with no proxy listening. The adapter cache was prepared before the check; the probe used npm offline mode. Greyproxy was installed but was not started for these prompt-free checks.

The initial agentctl checks exposed test setup issues. Offline npm first needed its adapter cache prepared. Greywall's Unix socket exceeded Linux's path-length limit under the long private temporary path. A shorter, dynamically allocated temporary directory fixed that failure. Private links to the resolved Nix Node/npm/npx tools fixed a launcher path that was absent inside the sandbox. These corrections changed test setup only. Each agentctl process, disposable home, and temporary directory was removed after its run. The retained manual instance was not used.

## Fresh live-provider acceptance

On 2026-10-19, the user completed a new disposable OpenAI Codex login. The complete checked-in runner passed with `openai-codex/gpt-6.1-sol`. It built the production source at `4364a99bb02d5544629e9c4c9891d959b6468c42` without an overlay. No production code changed during acceptance setup.

The direct control, authenticated wrapped discovery, wrapped utility prompt, full backend ACP chat, and native Pi terminal all passed in one run. Discovery returned 9 models and 6 modes with the new login. Each model case performed four ordered tool calls and produced the independently checked output. Each wrapped operation produced one launcher receipt. Both backend sessions reached an idle state before cleanup.

Initial retries corrected harness setup: the workflow-step API path, offline npm profile environment, tool-evidence timing, and first-run questions. Greywall's provider-profile prompt and Pi's project-trust prompt could consume the task description before Pi was ready. The final run suppresses the Greywall question without applying a built-in profile and trusts only its empty Pi test repository. Private fd and ripgrep executables prevent optional tool downloads. These are disposable test settings, not Kandev provider exceptions.

All 12 credential-free runner tests pass. They cover no-spawn refusal, foreign-listener rejection, unrelated-process preservation, detached-child cleanup, private-log handling, public-result sanitization, required tool order, version parsing, and explicit wrapper configuration. New failure cases were observed before their harness corrections.

The runner removed its processes, temporary homes, database, repositories, and credential copies. The newly created source login was then removed. No acceptance-owned process remains. The installed tools and private diagnostic logs remain available. No credential was copied from the retained manual instance. The manual instance and deprecated #3812 task remain unchanged.

Task 06 is complete. The requirements are active and the system design is current. PR #4538 remains draft and unmerged. The optional handoff was unavailable. Historical overlay evidence in [reproduction.md](reproduction.md) remains separate from these results.

## Reproduce the live acceptance run

The opt-in runner is `scripts/check-generic-wrapper-real.py`. It supports Linux and an explicit disposable Pi OpenAI Codex login. It does not load credentials from a normal home or from an existing Kandev instance. The credential file must have mode `0600`. Use a fresh login for this test and remove that login after use.

Prepare a private tool installation outside `/tmp`. Its `bin/` directory must contain Pi, pi-acp, Greywall, Greyproxy, Bubblewrap, socat, Node, npm, npx, bash, git, fd, and ripgrep (`rg`). Install pi-acp 0.0.34. Prepare an npm cache under the installation's `cache/` directory for offline `npx pi-acp@0.0.34`. Go, git, and bash must also be available on the host PATH. The runner does not install software.

Set these variables to private local values. Do not put credential values in them:

- `WRAPPER_ACCEPTANCE_TOOLS`: installation directory.
- `WRAPPER_ACCEPTANCE_AUTH_FILE`: disposable Pi `auth.json` path.
- `WRAPPER_ACCEPTANCE_MODEL`: explicit `openai-codex/<model>` identity.
- `WRAPPER_ACCEPTANCE_PRIVATE_LOG_DIR` (optional): private directory for raw diagnostic logs. Without this variable, cleanup deletes the logs.

Run from the repository root:

```bash
python3 scripts/check-generic-wrapper-real.test.py
python3 scripts/check-generic-wrapper-real.py --check-prerequisites
python3 scripts/check-generic-wrapper-real.py --run --record docs/plans/generic-launch-wrappers/acceptance.md
```

The prerequisite check starts no processes and sends no prompts. The live run builds the current backend, agentctl, and acpdbg without an overlay. It creates its own database, repositories, homes, ports, proxy, and process ownership record. It refuses a port whose listener belongs to another process. Cleanup stops only owned processes, including detached descendants, and removes the temporary resources and credential copies. It preserves the explicitly supplied source credential file.

Each model case must call read, bash, write, and read in order. The runner checks the computed file and random nonce independently. Wrapped cases must also produce an outer launcher receipt before Greywall executes the inner process. Only sanitized results enter this record. Raw output stays private.

The test proxy has one allow rule: `chatgpt.com:443`. It has no allow-all rule. The sandbox receives explicit paths for the tool installation, disposable state, npm cache, temporary directory, working directory, and managed npm directory. Both Kandev runtime references remain in the configured wrapper prefix. Greywall uses the owned proxy endpoints and does not use its default proxy control service. Its built-in provider profile, watch mode, and learning mode are disabled. The disposable Greywall preferences suppress first-run profile questions. Pi trusts only its empty test repository and skips optional online update checks. This policy permits Pi to read the disposable credential file; it does not claim that the credential is hidden from the agent.

The restricted policy does not permit the sandboxed native Pi process to contact the Kandev MCP endpoint. This acceptance checks Pi's built-in tools and terminal transport. It does not establish MCP access through Greywall, sandbox confinement, or support on macOS or remote executors.

## Opt-in real-provider runner result

```json
{
  "date": "2026-10-19",
  "status": "passed",
  "platform": "linux/x86_64",
  "model": "openai-codex/gpt-6.1-sol",
  "versions": {
    "pi": "1.3.0",
    "greywall": "0.3.7",
    "greyproxy": "0.4.5",
    "bwrap": "0.11.2",
    "node": "24.19.0",
    "npm": "11.17.0",
    "fd": "10.4.2",
    "rg": "15.2.0",
    "pi-acp": "0.0.34",
    "socat": "1.8.1.3",
    "go": "1.26.6"
  },
  "checks": [
    {
      "scenario": "direct",
      "status": "passed",
      "tool_calls": 4
    },
    {
      "scenario": "discovery",
      "status": "passed",
      "models": 9,
      "modes": 6,
      "receipts": 1
    },
    {
      "scenario": "utility",
      "status": "passed",
      "receipts": 1,
      "tool_calls": 4
    },
    {
      "scenario": "chat",
      "status": "passed",
      "receipts": 1,
      "tool_calls": 4
    },
    {
      "scenario": "passthrough",
      "status": "passed",
      "receipts": 1,
      "tool_calls": 4
    }
  ]
}
```

## PR review remediation, 2026-10-21

The shared package moved from `internal/agent/launchwrapper` to
`internal/common/launchwrapper`. Earlier commands above record the original runs.
Current checks use the new path and `internal/common/managednpm`.
The old managedruntime project helpers delegate to the neutral implementation.

Regression evidence before fixes:

- Windows lookup selected extensionless `npx` instead of the adjacent `.CMD` shim.
- A custom environment resolver returning `(nil, nil)` caused a public utility prompt to panic when adding gateway authentication.
- Initial, fresh, and resume builders for Pi and Claude used a later execution-record wrapper when constructing an earlier command's request.
- A detached child created during TERM survived cleanup and produced `owned_process_cleanup_failed`.

Review dispositions:

- Fix the three inline findings: Windows shim selection, refreshed descendant cleanup, and neutral shared package ownership.
- Fix the aggregate findings about nil environment maps and mutable passthrough wrapper state.
- Correct the utility assertion message. Rename the profile launch-token test and assert the separately returned wrapper prefix.
- Bind the acceptance callback's loop variables explicitly.
- Retain the receipt assertion. The wrapper writes its receipt before exec. The test waits for output from the executed child before reading the receipt.
- Retain sanitized stderr behavior. An additional suppression-count diagnostic is optional and changes no required behavior.
- Retain the nonempty argv invariant. Interactive start validates child argv, and successful wrapper preparation always returns a nonempty command.
- Retain the authenticated instance identity endpoint required by the design. Add a negative unauthenticated request case. The public control identity endpoint has a separate adoption contract.
- Retain the required registry constructor dependency. Normalize only a successful nil custom resolver result, and preserve resolver errors.

The user marked PR #4538 ready for review on GitHub on 2026-10-21.
Preserve that user edit and leave the PR unmerged.

Current review-remediation verification:

- Focused `go test -trimpath -p=2 -race` passed across common wrapper preparation, common npm preparation, host utility, runtime agentctl, lifecycle, and agentctl API/process/utility packages.
- The selected tests cover generic wrappers, gateway authentication with nil custom environment maps, profile launch tokens, MCP passthrough commands, and interactive request construction.
- All 14 credential-free runner safety tests passed, including TERM-time child creation, PID birth rechecks, and unrelated-process preservation.
- Both desktop wrapper browser tests passed on a fresh build. The phone wrapper browser test also passed against those binaries.
- Changed-package Go lint reported zero issues. Architecture lint, the documentation catalog, and specification lint passed.
- Native Windows CI now runs `internal/common/launchwrapper` and `internal/common/managednpm`, including the actual Windows execution-host npm-shim case.
