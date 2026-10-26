---
id: "04-real-session"
title: "Real no-model session"
status: done
wave: 4
depends_on:
  - "03-native-connection"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMAD-001
  - REQ-EXECUTORS-NOMAD-004
acceptance_criteria:
  - AC-EXECUTORS-NOMAD-001.1
  - AC-EXECUTORS-NOMAD-001.3
  - AC-EXECUTORS-NOMAD-004.1
  - AC-EXECUTORS-NOMAD-004.2
  - AC-EXECUTORS-NOMAD-004.3
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 04: Real no-model session

## Summary

Prove real Kandev to Nomad to agentctl execution with an inert ACP child and grandchild. Exercise normal supported agentctl surfaces without a model call.

## In scope

- Build disposable backend/runtime and use fresh database plus exclusive Nomad namespace/job/node/ports/workspace, normal built-in API profile and inert ACP agent path.
- Assert Kandev inventory acceptance, actual allocation, pinned SSH control handshake, reserved instance, ACP initialize/session/prompt/result and real child/grandchild start identities.
- Exercise terminal/file/Git/HTTP/WebSocket and retained ordinary Stop/backend shutdown semantics; capture actual binary versions and sanitized evidence. Record idle Nomad-client/agentctl RSS and startup timing with native ps/timing tools, without a fabricated threshold or benchmark claim.

## Workspace ownership clarification

- Treat profile WorkspaceRoot as a base, persist a separate immutable derived
  WorkspacePath per environment/generation, and use it consistently in job,
  controller, instance validation/result, digest and recovery. Add no SQL schema.
- Existing pre-release fixtures missing the exact path fail closed for new
  continuation; preserve their original root/job/fence for cleanup. Never move
  a live operation or delete another workspace to make the scenario pass.
- Materialize a selected real test repository through existing agentctl
  materializeWorkspaceRepositories, including the primary for Nomad even when
  there is only one repository. Keep repository projection keys aligned. Prove
  branch/base/status/log/diff in that exact workspace; a manual no-repo git-init
  plus clearing session.base_commit_sha is not selected-repository Git proof.
- Add focused tests that different environment IDs/generations derive distinct
  exact paths, retained state keeps its path after profile edits, and one-repo
  Nomad launch materializes its primary. Repeat the actual Git scenario only
  after this ownership correction is implemented.

## Out of scope

- Rendered UI, Docker/VM execution, plugin implementation, provider SDK/protobuf changes, model-backed workloads, and production rollout.

## Acceptance

- Real durable environment and allocation reach an authenticated ACP turn/result with actual observed child/grandchild identities; health or prepare-only does not satisfy the test.
- Terminal, file and Git round trips cross the same authenticated SSH/runtime boundary; invalid host key/unknown port and runtime identity cannot connect.
- Prompt completion, ordinary Stop and backend shutdown preserve the environment/workspace; no model endpoint is contacted and no duplicate controller appears.

## Verification

All new paths/test names are future deliverables. Run the complete block from
any shell with the named absolute root and fixture-only config set. Use TDD for
changed logic; no-tests-to-run or a skipped required real scenario fails the
work order. Do not mark done based only on existing tests.

```bash
: "${KANDEV_ROOT:?Set absolute Kandev worktree path}"
: "${KANDEV_NOMAD_TEST_CONFIG:?Set absolute fixture-only JSON config path}"
make -C "$KANDEV_ROOT/apps/backend" build
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e preflight --config "$KANDEV_NOMAD_TEST_CONFIG")
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e run --require-real --scenario session --config "$KANDEV_NOMAD_TEST_CONFIG")
```

## Files likely touched

- `apps/backend/internal/agent/nomad/integration/session_test.go and fixture (new)`
- `apps/backend/internal/agent/nomad/integration/inertacp (new)`
- `scripts/nomad-native-e2e`
- `Nomad runtime/bootstrap files only for scenario-proven defects`

## Dependencies

[Task 03](task-03-native-connection.md).

## Risks

Existing Mac tooling, SSH server and dedicated worker must actually exist. The runner must fail on missing prerequisites and preserve uncertain cleanup, not count a skip as evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-native-executor.md), using frontmatter IDs.
- [System design](../../specs/executors/system-design/nomad-native-executor.md), matching implementation boundary and current source.
- [Plan](plan.md), including strict real-worker runner/prerequisites.
- [ADR](../../decisions/2026-10-24-native-nomad-cleanup-evidence.md).
- Read applicable scoped `AGENTS.md` before editing code.

## Results

### Intermediate fixture recovery evidence (not WO04 completion)

- A direct allocation stop triggered an unexpected successor allocation for
  the same immutable job. The successor reached `Started`, then terminated with
  exit code 73. In the submitted bootstrap, exit 73 is the existing worker
  start-fence refusal before the agentctl command. The record is retained as a
  cleanup regression case; allocation stop is not the fixture teardown path.
- Recovery used whole-job deregistration with `purge=false`. Nomad reported the
  job `dead` with `Stop=true`; both full allocation records were retained on the
  expected node: original allocation `complete/stop`, successor `failed/run`.
  Both `agentctl` task states were `dead`; the successor's exit-73 event
  confirmed the held fence rejected its bootstrap before agentctl. The failed
  allocation's historical `DesiredStatus=run` did not represent a live task
  after the immutable job was stopped.
- A fixture-scoped process scan found no agentctl/ACP processes. The recorded
  original leader/ACP/child/grandchild PID start identities were absent, both
  reserved ports were closed, and the known Nomad client node was ready. Only
  after these checks did an exact owner/mode/non-symlink compare-and-delete
  release the fixture fence. Sanitized machine-readable evidence is at
  `/private/tmp/nomad-wo04-rjr6dc8s/manual-teardown-evidence.json`.
- The drain receipt was validated before the original allocation stop, but the
  first one-shot attempt did not retain its digest. No token, nonce, or raw
  allocation event message was recorded.
- This is manual recovery evidence for an old fixture, not the required
  reproducible runner result. A reusable cleanup regression must deregister
  the whole immutable job (never stop one allocation as final cleanup), fetch
  every full allocation including successors, retain unexpected history,
  and prove job stopped, all tasks terminal, agentctl/ACP identities absent,
  ports closed, and exact fence ownership before release.

The fresh selected-repository session below has now been exercised through the
strict attached-fixture verifier. This does not recreate the task from a fresh
database; fresh setup automation and the remaining lifecycle matrix are still
separate work.

### Current-source single-worker session evidence

- Built current worktree binaries and used the ordinary Kandev executor,
  repository and task APIs against a fresh fixture database. The isolated
  Kandev backend build identity was `v0.97.0-409-g0b81ae48d4-dirty`; its
  binary SHA-256 was
  `fe25ed683b2d0cfa1e0d5078513a96fbd92925fcac50db04575bb29d5773e7b9`.
  The running Darwin/arm64 `agentctl` SHA-256 was
  `951b81baadfced1d6ae99e2c31b0dba011fbb9bc7163d82e308dfa57a7895077`.
- The real Nomad 2.0.7 allocation
  `4ecbbfe7-a412-6415-67cf-b1c793f7a338` for job
  `kandev-ba4725ddfbfada39b038b56d5ad58ff3` reached `running` on the ready,
  eligible Mac client `worker-node.example` (node UUID
  `00000000-0000-0000-0000-000000000001`). The node reported 48 CPU shares and
  49,152 MB memory; these are Nomad scheduler values, not measured throughput
  or hard process limits.
- Normal Kandev task/session creation produced task
  `<task-id>`, session
  `<session-id>`, and environment
  `0a13769a-72b3-4752-a24d-dee15053b611` generation 1. Its immutable workspace
  was `/private/tmp/nomad-wo04-current.5TW3Gn/nomad/workspace/environments/0a13769a-72b3-4752-a24d-dee15053b611/g1`.
  The selected fixture repository materialized under that workspace on branch
  `main`; its Kandev base/head and the actual checkout HEAD were all
  `e22759b1019e487224429b0d3c568a2d43451c76`.
- The inert ACP returned the requested result marker through a normal Kandev
  WebSocket prompt. The strict run added exactly two session messages (one
  prompt and one result); it did not call a model endpoint. Tree and file
  reads, the Git snapshot, and an ordinary shell terminal command all crossed
  the selected runtime. The terminal was destroyed after the marker returned.
  An unknown terminal identity failed to connect. SSH rejected both an
  unpinned host key and a closed adjacent port.
- Live process identities and RSS at the strict run were: Nomad client PID
  91078, 84,368 KiB; Kandev backend PID 94724, 58,272 KiB; `agentctl` PID
  13565, 54,784 KiB; inert ACP PID 13764, 7,136 KiB; child PID 13821,
  4,736 KiB; grandchild PID 13842, 4,320 KiB. The ACP/child/grandchild
  parent and process-group identities were verified over the pinned SSH
  control connection. These are one fixture observation, not a benchmark.
- Reproduction for this still-running fixture (paths are local to the original
  test machine; the config contains fixture identifiers and private SSH
  material, not Nomad/Kandev API tokens):

  ```bash
  cd <kandev-checkout>
  python3 -m unittest -v scripts/test_nomad_native_e2e.py
  scripts/nomad-native-e2e preflight --config /private/tmp/nomad-wo04-current.5TW3Gn/session-config.json
  scripts/nomad-native-e2e run --require-real --scenario session --config /private/tmp/nomad-wo04-current.5TW3Gn/session-config.json
  ```

  The final real run passed and reported message count 18→20, one ACP result,
  selected repo revision, running allocation, process identities/RSS, terminal,
  file/Git/tree reads, and the negative SSH/runtime identity checks. The runner
  is an attached-fixture verifier: it does not bootstrap a new task or services
  and must not be represented as fresh-install automation.
- After that recorded session run, the isolated backend was rebuilt with the
  repository `build-runtime` target, including the required `fts5` tag and the
  canonical `bin/kandev` child expected by the launcher. A pre-restart SQLite
  backup at `2026-10-24 18:16:18Z` is the before-state: this session had 23
  messages (11 user, 12 agent), cursor sequence 35, and nine completed
  delivery submissions. User message
  `6fe32d30-8f10-401c-8425-22498229b8ab` (prompt sequence 11, turn
  `e62184e0-d1c0-4f59-a7f6-23c5cf8f3f6d`) was still in queued entry
  `92ec9027-acaa-4c4d-a195-6a60f263d379` with `user_message_recorded=true`;
  it had no delivery submission yet.
- After the corrected backend restarted, Kandev automatically consumed that
  existing queue entry; no task or prompt was submitted by the operator. The
  only new submission was
  `prompt:queue:614ec577-daff-4404-96bd-152345d2b9a1`, completed on terminal
  sequence 38 for the same turn. The inbox records `turn_started`,
  `message_chunk`, and `complete` at sequences 36/37/38. Its resulting agent
  message is `agent-delivery-e981dec580160d1aaf34458b9c1ff9fc`; the queue is
  now empty. Comparing before and after confirms all nine earlier submission
  IDs retain the same completed state, outcome, and terminal sequence, and all
  23 earlier session-message IDs remain unchanged. The session now has 24
  messages (11 user, 13 agent). This is one first delivery of a previously
  undelivered queued message, with no replay/result for completed submissions;
  it is not the broader WO05 restart/failure matrix.
- The first replacement bundle omitted `fts5` and exited before API bind; the
  fixture DB integrity check was `ok`. It was not the running recovery binary.
  The corrected listener PID 84656 was verified against the canonical
  `build-runtime` bundle at `/private/tmp/nomad-wo04-current.5TW3Gn/bundle-ww0nnnr_/bin/kandev`
  (SHA-256
  `72f70ce0de399ee20acafdd40d5b27be31756713b8db9db110d3b6e71ae3b0b9`). The
  earlier `fe25ed68…5773e7b9` hash above is the historical binary from the
  preceding attached-session run, not the corrected recovery listener.
- The preceding run used a pre-fix Stop implementation. A second fresh fixture
  database and current canonical `build-runtime` bundle were used for the
  ordinary Stop check below; the earlier run's state and evidence were left
  untouched.

### Current-source ordinary Stop and backend-shutdown retention

- Current build identity was `v0.97.0-409-g0b81ae48d4-dirty` from source
  `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`. The isolated backend binary
  SHA-256 was
  `cf84ab55f541c0d4924e8cd3c268c49c45cccf5dc27fb9b844ceb2a88aee8905`;
  the Darwin/arm64 `agentctl` SHA-256 was
  `38a3404ca206bd622f90f95f465023280615152400d01653a19a94210cfba7b3`.
- Normal APIs created task `6d4a40e5-2be6-472e-9593-915bf4c636a1`, session
  `ef05bc50-e9f9-4fa7-a2ae-768c442e0ac7`, and environment
  `07b69796-8d23-4941-b8a4-c5c75e55ec7b` generation 1. The selected fixture
  repository was `5cfb4dad-7047-47cd-880e-87a8a299b4be`, with checkout/base
  revision `e22759b1019e487224429b0d3c568a2d43451c76` on `main`. Its immutable
  workspace was
  `/private/tmp/nomad-wo04-current.5TW3Gn/nomad/workspace/environments/07b69796-8d23-4941-b8a4-c5c75e55ec7b/g1`.
- The strict attached-session runner passed for this current-source session:
  the inert ACP returned one result, the message count advanced 4→6, and
  terminal, file, Git, tree, and repository checks passed. The run made no
  model request. It remains an attached-fixture verifier, not fresh-install
  setup automation.
- The normal `session.stop` WebSocket action returned success and then the
  session reached `CANCELLED`; its durable executor row reached `stopped`.
  The row kept the same Nomad job/allocation/node/workspace/server identity and
  both secret references, while its local agentctl URL and port were cleared.
  The selected repository workspace remained present. This exercises the
  actual `stopped via API` reason, not an empty or synthetic Stop reason.
- After Stop, Nomad still reported the exact job running with `Stop=false`,
  its allocation `running/run`, the `agentctl` task `running`, and the worker
  `ready/eligible`. The agentctl leader PID 42717 remained and owned control
  port 23665. Its runtime instance port 22044 and ACP/child/grandchild PIDs
  42861/42871/42876 were absent. Kandev's local forward port 63872 was closed.
  The mode-0600 worker fence still matched the same installation, node,
  environment, generation, operation, input digest, job, and allocation
  (owner-record SHA-256
  `fa155fde740d83ab7cd4ec06fb220b604dfa36128f35a1cf54c28235c1f928dc`).
- The isolated Kandev backend was then stopped through its exact owned-process
  registry entry. Its API port 43840, helper port 39429, and forward port 63872
  were closed. After shutdown, SQLite still contained the cancelled session,
  stopped execution row, workspace, and secret references. Read-only Nomad
  checks still showed the same running job/allocation and ready node; the
  exact agentctl leader still owned control port 23665, the ACP descendants
  remained absent, and the worker fence digest was unchanged. No Nomad service,
  worker process, workspace, or fence was stopped or removed.
- An idle sample after Stop/backend shutdown reported Nomad client RSS 85,968
  KiB and retained agentctl RSS 45,728 KiB. Startup durations were not captured:
  these process-start identities do not include a trustworthy matching
  service-ready timestamp, so no duration is inferred.
- Fixture-only teardown is now driven by a private captured identity manifest
  rather than fixed task/session/PID/status constants. Capture is read-only:
  `python3 scripts/capture_nomad_fixture_teardown.py --config <session-config> --database <fixture-db> --out <new-private-manifest>`.
  It records exact row `updated_at`, a SHA-256 of the exact row metadata bytes,
  Nomad state revision and immutable job/allocation/node/workspace identity,
  captured session state, and agentctl PID/start/runtime path; it contains no
  secret values. The opt-in teardown helper rejects any row/state/process drift
  before acting and preserves the captured session state. On the current
  fresh2 fixture, the manifest was captured and its row/state snapshot passed
  read-only validation; destructive teardown was not run, leaving this runtime
  retained for the next lifecycle work.
- Focused regression command after the Stop-reason fix:
  `GOFLAGS=-p=1 GOCACHE=/private/tmp/kandev-go-build-cache go test ./internal/agent/runtime/lifecycle -run '^(TestNomadExecutorOrdinaryStopClosesOnlyMatchingLocalTransport|TestNomadExecutorDestructiveStopReasonFailsClosed|TestNomadExecutorForceStopFailsClosedAndCloseOnlyDetaches|TestOrdinaryNomadStopRetainsTypedInventoryForEnvironmentRecovery|TestNomadBackendShutdownRetainsInventoryAndClosesOnlyLocalTransport)$' -count=1`
  passed (`ok`, lifecycle package 0.934s). The earlier session's persisted
  queued delivery evidence remains separate; this fresh session did not replay
  it or submit any model-backed prompt.
