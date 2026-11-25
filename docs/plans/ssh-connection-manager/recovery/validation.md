# Local validation after recovery

Date: 2026-10-20. Baseline: `e34761417d28e561ce9bd616cfa78c1b723df9ad`.
These results apply to the reconciled local implementation. Historical reports
are recorded separately in [recovery.md](recovery.md).

## Completed checks

- `go test -trimpath ./internal/sshtransport ./internal/sshconnections ./internal/secrets ./internal/runtimeflags ./internal/profiles -count=1`: passed.
- `go test -trimpath ./internal/common/config -count=1`: passed with inherited `KANDEV_*` overrides removed and fixture-compatible umask `022`. Earlier runs failed because the parent environment set server port 9998 and umask `077`.
- `go test -trimpath ./internal/agent/runtime/lifecycle ./internal/backendapp ./internal/ssh ./internal/executors/reachability ./internal/task/repository/sqlite -run 'Test(ManagedSSH|SSH|ConnectionReady|ManagedSelectors|ExecutorBinding|ManagedProbe|ExecutorOwner)' -count=1`: passed. This is a targeted selection, not all tests in these packages.
- `pnpm exec vitest run components/settings/ssh-connections lib/api/domains/ssh-connections-api.test.ts lib/state/slices/features/features-contract.test.ts 'app/settings/executors/new/[type]/ssh-config.test.ts' lib/settings-discovery/catalog.test.ts`: passed, six files and 30 tests after the namespace fix and additional legacy-config regression test.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`: passed, including rich-output catalog validation. Repeated after the final production edits; passed again, followed by all localization checks. An earlier run exhausted the default 4 GiB heap.
- `pnpm run i18n:zh-hant --namespace settings`, `pnpm run i18n:pseudo`, `pnpm run i18n:check`: passed after Korean translations were added. Unrelated pseudo-catalog regeneration was removed from this change.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed after initial design recovery.
- Focused ESLint with `--max-warnings 0`: passed after extracting the inline-config builder and enrollment-status component.
- `bash scripts/lint-go-changed <reconstructed Go paths>`: passed after extracting the imported-key parser. The initial cold-cache lint reported one complexity warning and timed out; the final run exited 0.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`: passed after adding the settings route to coverage metadata.
- PostgreSQL: `go test -trimpath ./internal/sshconnections ./internal/secrets -run 'TestPostgres(ConnectionVersionsAndSingleHopGuards|SSHIdentityVault)$' -count=1 -v`: both tests passed against a private temporary Unix-socket PostgreSQL 17 instance. The server was stopped and the directory removed. This covers the recovered PostgreSQL cases, not every proposed transaction scenario.

Go commands run from `apps/backend` with `GOENV=off`, `GOCACHEPROG=` and a
disposable cache under `/tmp`. Tests that bind local sockets ran outside the
restricted sandbox. Web commands run from `apps/web` after frozen-lockfile
installation. No dependency manifests were changed.

- Legacy SSH compatibility: lifecycle tests selected by `Test(InheritFromSSHConfig|ApplyTargetDefaults|BuildAuthMethods|BuildClientConfig|DialSSH|DialViaJump|ResolveBastion|BastionHostKey|IdentityAgent|EffectiveSSHConfig)` passed with `-trimpath -count=1`.

- Required-store catalog, sweep assignment, adapter identity and owner-coverage checks passed. The SSH SQLite conformance case passed fresh schema, replay, CRUD, conflict and transaction scenarios: `go test -trimpath ./internal/persistence/storeconformance -run '^TestStoreConformance$/^sqlite3$/^ssh-connections$' -count=1 -v`. An initial selector used the wrong engine/store names and ran no subtests; it is not counted as acceptance.

## Browser evidence

The first managed Chromium run built the backend, web bundle and fixtures, but
all four tests failed during backend fixture startup before browser interaction.
An inherited server bind setting selected the host's Tailscale address while the
fixture checked localhost. The next run removes inherited `KANDEV_*` settings
and uses only disposable local services. This is not a feature pass or a
regression-test RED result.

The clean run then found missing Chromium shared-library lookup paths on Nix.
A temporary process-only `LD_LIBRARY_PATH` points at existing libraries; no
system installation was changed. This exposed a real startup failure:
`handlers are already registered for path '/api/v1/ssh/identities'`.

`TestSSHManagementAndDiscoveryRoutesCoexist` reproduced that panic (RED), then
passed after moving only new management routes to `/api/v1/ssh/managed` (GREEN).
The frontend client and managed fixtures use that namespace. Existing discovery
retains its path and authorization contract.

The phone test initially used the visible picker text as its accessible name;
the associated label is `Identity`. Correcting that test selector reached the
intended check: drawer scroll containment failed with `auto` instead of
`contain` (RED). The picker now uses `overscroll-contain`; the rebuilt mobile-chrome run passed
(1 test, 15.1 seconds). It also verified 44px targets, identity selection,
save/create-bastion/return navigation, draft retention and no horizontal overflow.

A breadcrumb regression test also reproduced untranslated `Ssh Connections`
(RED). The route now resolves the existing translated `settings:sshManager.title`
key. The full breadcrumb test file passed (28 tests), followed by lint with
zero warnings.

## Initial recovery checks and limitations

All seven focused browser cases passed across the final runs:

- Chromium: saved drafts and bastion-return navigation at 390, 767, 768 and
  1280px. The recovered 390/767px tests incorrectly inferred a touch pointer
  from width; corrected selectors passed both cases (15.8 seconds).
- Mobile Chrome: 390px phone and 820px coarse-pointer tablet. Both passed
  (25.6 seconds), including touch geometry, drawer containment and draft return.
- Feature disabled: direct managed mutation returned 404; the settings route
  showed its unavailable state and made no managed credential-list requests.

The full managed runner rebuilt native/web/fixture artifacts before product
checks. Subsequent test-only corrections used the guarded `e2e:raw` runner
against those same fresh artifacts. Projects ran sequentially with one worker.
Later fixture runs explicitly bound the backend to loopback. The complete
TypeScript/localization checks, changed-package Go lint, focused ESLint, public
doc checks, specification validators, and tracked-file whitespace checks passed.
No current validation process remains running.


The earlier broad Go runs skipped PostgreSQL cases without a DSN; the explicit
disposable-database run above executed both recovered cases. Docker-backed real
SSH tests have not run because no Docker/Podman runtime is available.
At this checkpoint, full unattended restart, real-host enrollment,
authorization browser flows, and the complete desktop/phone acceptance matrix
remained open. The continuation below advances these checks. No work order was
marked complete.


## Continuation: protocol fixtures and UI corrections

The current user explicitly requested continued implementation. No additional
old-instance requests or mutations were needed. Source recovery provenance and
immutable reconstruction artifacts remain unchanged.

### Protocol and process evidence

New tests use generated disposable keys, temporary HOME directories, loopback
listeners, and the production enrollment script. The Go SSH server accepts
only that exact script. Bastion forwarding permits only the configured fixture
destination. An invalid destination DNS name prevents accidental direct access.
No real host, account authorization, or user SSH directory is modified.

- `TestEnrollmentWireUsesSelectedIdentityAndRoute`: direct/bastion routes with
  password/existing-identity bootstrap. Preserves existing authorization bytes,
  appends the selected key once, verifies through a new key-authenticated
  connection, and never replays the bootstrap credential on an identical retry.
- `TestEnrollmentWireRetainsPartialOutcomes`: fresh-auth rejection and transport
  loss retain the installed public key, clear readiness, and do not replay after
  recovery. Authentication rejection retains its specific diagnostic.
- `TestEnrollmentWireChangedHostKeyBlocksBootstrap`: changed direct destination,
  forwarded destination, and bastion keys block credentials before installation.
  Enrollment results identify the failed hop.
- `TestEnrollmentWireAuditFailureBoundsRemoteEffects`: failed initial audit
  prevents remote effects. Failed final audit leaves a recoverable unknown
  outcome and does not replay installation.
- `TestManagedSSHWireRestartWithoutAgent`: a new test process reopens the same
  SQLite database and protected storage without SSH_AUTH_SOCK or SSH_AGENT_PID.
  It authenticates retained direct and bastion runtime snapshots.
- `TestManagedSSHWireProtectionLossDoesNotReplaceIdentity`: missing ciphertext
  and a different protection key fail before authentication and preserve the
  original identity metadata.

Command: `GOENV=off GOCACHEPROG= GOCACHE=/tmp/ssh3735-go-cache
GIN_MODE=release GOMAXPROCS=2 go -C apps/backend test -trimpath -race
./internal/sshconnections -count=1`.
Result after the final diagnostic changes: PASS (15.949 seconds). The helper
process test skips when invoked without its fixture environment. PostgreSQL
cases in this command skip without a DSN; the earlier explicit database run is
separate evidence. The fixture is an SSH protocol server, not OpenSSH. This
process test exercises the connection service, not full desktop startup or an
agent workload reconnect.

### Regressions reproduced and fixed

All behavior changes below had focused RED results before the fix:

1. Same-tick management mutations both reached the API. Immediate admission now
   rejects the duplicate and remains busy through the original refresh.
2. Older initial data and errors replaced a newer successful refresh. Only the
   latest load revision may publish either result.
3. A dismissed identity form navigated after creation finished. Its completion
   callback now checks whether the form still owns the screen.
4. Cancelling desktop or phone enrollment left its password in the form.
   Closing confirmation now clears it. Positive tests confirm that an explicit
   confirmation still submits the selected password once.
5. A changed route retained credentials and displayed the old enrollment
   success. Enrollment state is now scoped to the route stamp; old outcomes are
   retained in storage but hidden from the new setup view.
6. Enrollment discarded the failed hop and converted fresh authentication
   rejection into `ssh_connection_failed`. The optional persisted `hop` field
   and existing bounded diagnostic now survive to the UI. Legacy records
   without `hop` remain readable.

The real confirmation test also exposed invalid nested paragraph markup.
Enrollment confirmation now uses phrasing elements inside the shared description.
The identity method selector was extracted to meet the function-size limit.
The duplicate `verified` outcome literal was consolidated for Go lint.

Command from `apps/web`: `node node_modules/vitest/vitest.mjs run
hooks/domains/settings/use-ssh-connections.test.tsx
components/settings/ssh-connections
lib/api/domains/ssh-connections-api.test.ts`.
Result after final production changes: PASS, 16 tests in five files.
`NODE_OPTIONS=--max-old-space-size=8192 node node_modules/typescript/bin/tsc
--noEmit` also passed.

### Browser and final checks

- Rebuilt Chromium run: six tests passed (35.3 seconds). Covers disabled
  management, draft retention at 390/767/768/1280px, key generation, public-key
  copy, wrong-passphrase rejection, protected import, cancellation, and explicit
  enrollment confirmation with a simulated partial-outcome API response.
- Fresh-artifact auth/mobile run: the administrator/member boundary test passed.
  Members received 403 for identity/connection reads, identity creation/deletion,
  observation, preflight, authentication, trust, and enrollment. The browser hid
  management from the member and allowed the administrator to view the key.
- The same run passed the 820px coarse-pointer draft flow and the phone identity
  and confirmation flow. Its 390px geometry assertion reported 43.99994px for a
  44px drawer row. This floating-point test issue is separate from product sizing.
  The assertion now rounds to 0.01px. A deliberate temporary 43px DOM mutation
  failed at 43px (RED), then was removed. The restored 390px phone passed
  (15.2 seconds), and the affected 820px tablet passed (15.5 seconds).
  Ten distinct browser cases have passing results across these final runs.
- Full TypeScript and localization checks passed. Scoped Go lint reported zero
  issues. Focused ESLint passed with zero warnings after test constants were
  extracted. Public documentation validation, specification validation, and
  tracked-file whitespace checks passed.

Browser commands used `e2e:run --host` for the build and Chromium checks, then
`e2e:raw` with explicit projects for current-artifact auth/mobile checks. Every
run used one worker. The environment removed inherited KANDEV_* values and
bound the disposable backend to loopback. Existing Nix browser libraries were
selected through process-only LD_LIBRARY_PATH. The first import test used the
wrong accessible label (`Method` instead of `Identity source`); fixing the test
allowed it to reach the intended flow. No dependency package was added.

The UI enrollment fixture simulates API responses and performs no SSH access.
Its positive and cancellation assertions cover confirmation behavior, exact
route/public-key selection, password clearing, and visible partial outcomes.
The Go protocol fixtures supply separate real-network/script evidence.

### Remaining acceptance gaps

At this checkpoint, OpenSSH/container-managed executor launch and reconnect
were blocked by the missing Docker runtime. The later authorized-environment
section below supersedes that environment limitation. Existing proposed test names are not themselves requirements:
identity browser cases now live in `ssh-connections.spec.ts` and its mobile
counterpart, with helpers under `e2e/helpers/ssh-*.ts`.

The full adoption/executor browser matrix and PostgreSQL transaction matrix
remain incomplete. Identity deletion still needs explicit connection/runtime
dependency details (AC-001.6). Inspect and resolve these gaps before marking any
work order complete. All six orders remain `in_progress`.

Kandev rich-output discovery returned `unsupported call`; this durable note is
the validation record. No commit, push, PR, GitHub message, live enrollment, or
old-instance mutation was performed.

Final state: all validation handles have completed. The temporary 43px mutation
is absent from the source. The corrected tablet test passed, its ESLint check
passed with zero warnings, and the updated specification/plan records passed
validation. No work order is marked complete.

## Authorized OpenSSH test environment (2026-10-20)

The user explicitly authorized installing the tools needed for the remaining
container tests. Docker 29.7.2 and Buildx 0.31.1 were obtained with
`nix build nixpkgs#docker nixpkgs#docker-buildx --no-link --print-out-paths`.
No persistent system service or user profile was enabled. The temporary daemon
used its own data directory, client configuration, socket, and `k3735test0`
bridge at `172.28.73.1/24`. Tests used the repository's Alpine/OpenSSH image,
fresh fixture keys, disposable databases, and no inherited SSH agent variables.
The old instance and live remote SSH access were not used.

The first managed build failed before tests because `git describe` produced a
commit hash, which the compact-helper manifest rejected as a release version.
The local test build now sets `VERSION=0.0.0`; no release metadata was changed.
The next run passed five legacy authentication cases and three ProxyJump cases.
Its managed workload timed out while the NixOS host firewall blocked the Git
fixture on the test bridge. A temporary INPUT rule allows only TCP from
`172.28.73.0/24` on `k3735test0` to `172.28.73.1`, destination ports
32768 through 60999. An HTTP probe returned 200 before rerunning workload tests.

New test-only coverage in `managed-adoption.spec.ts` checks direct and bastion
migration, preserved targets and pins, unchanged inline settings before explicit
adoption, rejected unverified routes, concurrent configuration conflicts, and
post-restart authentication. Its first bastion run reported
`ssh_bastion_not_ready`: the fixture had trusted the bastion but omitted its
required authentication check. The corrected fixture asserts this rejection,
then authenticates the bastion before testing the destination.

`managed-enrollment.spec.ts` adds real OpenSSH password bootstrap on direct and
bastion routes, plus identity bootstrap through a bastion. It checks exact
preservation of existing authorized-key bytes, installation of only the selected
public key, no bastion key changes, idempotent request retries, and authentication
after a backend restart. Passwords belong only to disposable containers and
are supplied through stdin for fixture setup.

The managed executor restart case now checks health through the restored local
SSH tunnel. A surviving database row alone is not its reconnect assertion.
Its cleanup now stops test workloads before disabling the feature; the earlier
order caused the expected disabled-path refusal during fixture cleanup.

### Startup recovery regression found by stronger validation

The original managed container assertion checked only the persisted remote port.
That case passed, but it did not establish a working reconnect. The added health
request through the restored tunnel exposed a missed current-main recovery path:
`executor_ssh_recovery.go` resolved inline legacy metadata and tried the default
key file instead of using the retained managed identity. Its test cleanup also
failed after this unavailable recovery; the final Playwright error was the
cleanup fetch failure. Backend logs recorded the missing default identity file.

A focused Go regression exercised the real `RecoverInstancesDetailed` boundary
with a reachable disposable inline SSH server. All four cases failed RED: valid
managed metadata, missing route stamp, stale identity generation, and disabled
manager. Each was incorrectly classified as `no_matching_instance` after using
the legacy path. Recovery now calls the existing `dialSession` with admission
disabled and a cloned metadata map. This retains route/generation validation,
managed vault ownership, and fail-closed behavior without changing legacy dialing.

The final pre-fix container run passed both adoption cases and all three new
enrollment cases. Managed reconnect remains unaccepted until the rebuilt run
with the production fix passes.

The Go regression and existing recovery contracts passed GREEN with race checks:
`GOENV=off GOCACHEPROG= GOCACHE=/tmp/ssh3735-go-cache go -C apps/backend test
-trimpath -race ./internal/agent/runtime/lifecycle -run
'Test(ManagedSSH|SSHRecovery)' -count=1` (1.182 seconds).
Changed-package lint passed with zero issues using the existing Nix-installed
`golangci-lint` 2.12.2. Full TypeScript and ESLint on all three managed SSH E2E
files passed after their final edits.

Additional current-main integration evidence: managed executors cause the orphan
sweep scheduler to report a missing legacy host fingerprint.
`executor_ssh_orphan_sweep_scheduler.go:runSweep` still calls
`SSHTargetFromExecutorConfig` and `dialSSH` directly. It refuses these managed
selectors before dialing. Managed orphan-sweep integration remains a separate
acceptance gap; the startup recovery fix does not change that scheduler.

### Final OpenSSH results and cleanup

PASS: the final rebuilt run passed all ten tests in 3.3 minutes, with one worker
and no skips. Command, with the temporary Docker socket and the test-only build
version described above:

```bash
pnpm --dir apps/web e2e:run --host \
  tests/ssh/managed-adoption.spec.ts \
  tests/ssh/managed-connections.spec.ts \
  tests/ssh/managed-enrollment.spec.ts \
  tests/ssh/recovery.spec.ts --project=containers
```

This is post-fix evidence for both adoption topologies, managed executor
launch/restart with an actual restored tunnel, three enrollment cases, and four
legacy recovery cases. The managed case now completes its teardown while the
feature is enabled. No production host-verification rule was weakened.

Across the focused runs, 34 distinct OpenSSH/container cases have final passing
evidence. The earlier unchanged cases cover five authentication paths, three
ProxyJump paths, four configuration-inheritance paths, three host-key-rotation
paths, and nine executor launch/lifecycle paths. This is the selected SSH matrix,
not the entire containers project. Failed setup and pre-fix runs remain recorded
in [the machine-readable check record](openssh-checks.json); they are not counted
as successful runs.

All test handles completed. The dedicated Docker daemon contained no remaining
containers and exited successfully on SIGTERM. Cleanup removed six rules scoped
to the test bridge, the bridge, the temporary daemon state, and the temporary
runtime pointer. The downloaded Nix store packages remain available. No persistent
system service was enabled, and no live remote SSH access was changed.

Specification/document-index validation and tracked-file whitespace checks
passed. No commit, push, PR, or GitHub message was made. Remaining issue work:
identity-deletion dependency details, the full browser adoption matrix, managed
orphan-sweep integration, desktop-specific acceptance, the complete PostgreSQL
transaction matrix, and final acceptance reconciliation. All work orders remain
in progress.

## Local commit preparation (2026-10-20)

The user requested a local commit and an isolated manual test instance. The first
hook run rejected three new imports across the runtime facade boundary and
repeated Go literals. The new management handlers now use the shared SSH
transport directly. Backend wiring uses the runtime facade for managed SSH
contracts and failure classification. Repeated protocol values use named
constants without changing their values. Architecture lint and the SSH service,
transport, and handler package tests pass after these changes. The final commit
hook receipt and manual-instance evidence are stored with the disposable lab
outside the source tree. The feature remains disabled in shipped profiles.


## Remaining acceptance completed (2026-10-25)

The user requested implementation after draft PR #4778. The remaining orphan
cleanup, identity dependency details, browser adoption, native desktop startup,
and PostgreSQL transaction work is complete locally. The [dated acceptance
record](acceptance-2026-10-25.md) contains current evidence, the requirement
mapping, failed setup attempts, validation limits, and fixture cleanup. Earlier
remaining-gap statements above describe their dated handoffs.

The changes were uncommitted at the 2026-10-25 handoff. The user authorized
committing and pushing them on 2026-10-26. CI and review comments remain deferred.
No main instance, seeded instance, or live remote authorization was changed.
