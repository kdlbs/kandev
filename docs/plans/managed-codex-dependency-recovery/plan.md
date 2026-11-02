---
created: 2026-10-19
status: draft
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-005
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs: []
---

# Fix plan: Missing bundled Codex dependency recovery

## Overview

Automatically prepare a private copy of the exact selected managed Codex ACP
runtime after independently confirming a missing bundled platform dependency,
then retry once. Agents owns this correction because it owns managed runtime
identity, preparation, and capability recovery; Tasks and the profile editor
consume the outcome.

Implement the executor-local evidence/preparation boundary first, then task
startup recovery, then host/profile discovery. Finish with real executor and
desktop/phone proof and public guidance. Work orders are sequential and do not
authorize delegation. Stop at this design-package handoff before changing
production or permanent test code.

## Confirmed evidence and conformance

On 2026-10-19 a task resume launched
`@agentclientprotocol/codex-acp@2.2.2`. Its npm execution tree contained
`@openai/codex@0.160.1` but not `@openai/codex-darwin-arm64`, although the
lockfile declared that optional dependency. The cached Codex `--version`
command exited 1 with the same diagnostic. The retained backend bundle had
no warnings; launch logs showed a retained resume token and failure during
initialize, before restoration.

The user repaired the installation and selected automatic repair once instead
of a manual repair button. No user cache or task was modified during diagnosis.
The original reason npm omitted the package remains unknown; Node 26 and an
npm race are not established causes.

Current `managedRuntimeNpmStartupFailure` handles exact ETARGET/policy
diagnostics, while `managedRuntimeStartupRetryForEvidence` requires npm codes
or complete empty-stderr process-exit evidence. Neither confirms or repairs a
missing bundled dependency. `hostutility.probeManagedRuntime` retries only the
ETARGET probe failure code. Profile flights bypass that helper and explicitly
avoid installation.

This is an extension of intended behavior, not a violation of an existing
missing-dependency acceptance criterion. The existing no-shared-tree-mutation
rule is correct and remains mandatory. The primary requirement/design pair
is amended with draft requirement 005; the profile discovery pair links its
narrow exception. The new
[ADR](../../decisions/2026-10-19-isolated-managed-dependency-recovery.md)
qualifies the earlier explicit-maintenance-only consequence.

## Scope

### In scope

- Typed, generation/attempt-bound classification of the observed Codex
  initialize error, verified against actual executor-local package inventory.
- Private project/cache preparation of the selected bridge with optional
  packages included, followed by executable validation.
- Task start and resume on local PC, local Docker, and SSH; agent-wide host
  discovery and both profile capability and model-option probes.
- Existing one-replacement budget, cancellation, sibling safety, retained
  conversation identity, actual runtime observation, and truthful errors.
- Existing desktop/phone progress, retry, and error surfaces, with deterministic
  subprocess/browser/executor proof and public documentation.

### Out of scope

- Proving why the original npm install omitted its optional dependency.
- Arbitrary dependency repair, native Codex app-server, global Codex selection,
  `CODEX_PATH` replacement, custom command prefixes, or passthrough.
- Version fallback, global npm cache cleanup, modifying the shared broken
  tree, new settings, new UI controls, or broad preflight on healthy starts.
- Persistent promotion/cataloguing of a repaired runtime, lockfile restoration,
  or a new orphan-cache janitor.
- Sprites, remote Docker, Kubernetes, post-initialize replay, and a third attempt.

## Technical approach

### Executor-local evidence and prepared-runtime owner

Add shared diagnostic recognition beside `common/npmresolution/matcher.go`,
and bounded inventory/preparation ownership in `agent/managedruntime`.
Derive package and platform expectations from registered Codex metadata and
verified installed manifests, never from the diagnostic's install suggestion.
Use the existing deterministic cache key and runtime-observation inspection
pattern. Absent, unreadable, symlinked, truncated, or inconsistent metadata
must not become an automatic installation authority.

Keep a typed missing-dependency evidence field separate from
`agentctl/types.ManagedStartupEvidence`: the bridge can be alive while its
Codex child has already returned error 1001. Carry exact failed task generation
or probe identity, trusted package, and stable reason. Raw RPC frames and paths
stay out of the API and persistence.

Introduce an opt-in typed directive on the existing authenticated start and
probe requests. Advertise agentctl support and validate directive identity
against the configured trusted command and inventory at the execution boundary.
An old peer receives the ordinary request and never a destructive fallback.
No new browser/MCP package-install endpoint is introduced.

Prepare using npm argument vectors in a private project and private cache,
with explicit optional-dependency inclusion. Recheck the installed bridge
version and execute the expected bundled CLI's bounded version check. The
selected bridge version remains unchanged; normal transitive dependency
resolution is not an independently pinned CLI-version guarantee. Update the
observed provider version honestly.

The private root remains owned by agentctl through child lifetime, not by a
backend request. Stop/cancel must reap installers and children before release.
Detach/adoption must not release an in-use runtime. Only verified owned private
roots can be deleted.

### Task retry integration

Extend `runtime/lifecycle/managed_runtime_startup.go` at its typed
pre-handshake retry gate. Reuse `beginStartupRecovery`, stop confirmation,
shared deadline/backoff, stale-generation fencing, and normal initialization.
Carry the repair directive through `runtime/agentctl.Client.Start`.
Do not reconstruct commands in the backend or call `RepairManagedRuntimeCache`.

Preserve `managed_runtime_startup` routing and generic localized copy. Add
bounded dependency/preparation reasons in failure metadata; accurate attempt
counts distinguish installer failure from a second agent-process failure.
Keep accepted prompt delivery and native conversation restore in the existing
owner pipeline.

### Host and profile integration

Share one bounded probe-retry helper rather than copy repair policy into three
flights. Agent-wide probing keeps the existing publication owner. Profile
baseline and model-option flights preserve complete authorized launch settings,
single-flight keys, generation checks, draft values, and profile-only results.
Do not treat raw environment hashes as safe identity or fall back to default
profile settings.

Retain admission bounds and recheck contexts before preparation/replacement
and publication. Cancel superseded profile repair despite the existing
`context.WithoutCancel` request-lifetime behavior.

### Compatibility matrix

| Shape | Behavior | Evidence / fallback |
| --- | --- | --- |
| Managed Codex ACP, bundled provider, local PC/Docker/SSH | Verify, privately prepare, retry once | Real subprocess and executor tests |
| Same managed package, agent-wide host probe | Recover before final catalogue publication | Hostutility and settings E2E |
| Same package, profile baseline/options | Recover with identical authorized profile context | Profile-generation tests and desktop/phone refresh |
| Bridge alive after provider-child error | Eligible only with typed initialize failure plus verified absence | JSON-RPC 1001 fixture |
| Non-empty `CODEX_PATH`, prefix/custom/native/passthrough | No private managed repair | Negative command/env tests |
| Other providers or ambiguous inventory | Existing error/recovery behavior | Negative matcher/inventory tests |
| Old agentctl or unsupported executor | No new directive or fallback | Compatibility and admission tests |
| Auth/policy/disk/permission/cancellation or post-init error | Existing precedence, no dependency repair | Failure and lifecycle tests |

## Presentation and mobile contract

No rendered UI, copy, layout, navigation, or control change is planned.
Reuse boot progress and `managed-runtime-startup-copy.ts`'s generic startup
failure fallback, existing inline recovery cards, and existing profile refresh
loading/error presentation. Therefore no new ASCII preview is required.

The nearest shipped exemplars are the existing managed-runtime startup retry
desktop/mobile specs and profile-capability refresh specs. The task transcript
remains the scroll owner, and phone Retry runtime/Refresh targets retain their
current touch treatment. Browser tests must prove successful resume and refresh,
and visible final details on exhaustion, rather than only state metadata.
If existing generic copy proves inaccurate, update the design and add the
small affected-region preview before making a UI change.

## Tests

All test names below are planned additions, not claimed existing evidence.
Use local fixtures without registry/network/provider credentials.

| Acceptance | Planned regression evidence |
| --- | --- |
| 005.1, 005.9 | `TestManagedDependencyEvidence` in agentctl process/utility tests: wrapped RPC error, live bridge, wrong provider, malformed/incomplete evidence, external binary |
| 005.2, 005.4 | `TestManagedDependencyPreparation` in managedruntime tests: same exact bridge, scoped registry/environment, private prefix/cache, optional inclusion, validated executable, actual CLI observation |
| 005.3 | `TestManagedDependencySiblingSafety`: live healthy, recovered, and exhausted siblings, shared sentinels and real later reads unchanged |
| 005.6 | `TestManagedDependencyRetryBudget`: ETARGET then dependency error, cleanup refusal, cancellation/deadline during install, no third process |
| 005.7 | `TestManagedDependencyPreparedLifetime` and `TestManagedDependencyTaskResume`: child outlives request/detach, owned cleanup, original resume token, one accepted prompt |
| 005.5, 005.7 | `TestManagedDependencyHostProbe`, `TestManagedDependencyProfileProbe`, `TestManagedDependencyProfileModelOptions`: isolated publication and context-change cancellation |
| 005.8 | `TestManagedDependencyFailureDetails`: sanitized final installer/replacement cause and accurate attempts using existing card |

Retain existing ETARGET/transient/empty-stderr/permanent-error tests as controls.
Every work order starts with its named failing behavioral regression, then the
minimum implementation.

## E2E tests

Extend the existing `e2e/helpers/managed-runtime-recovery.ts` fixtures with a
Codex-like bridge that returns the real initialize failure while remaining
alive, independently absent native dependency, and local fake npm installation.
Assert real private preparation and validated replacement, not just a second
command invocation. Do not hide fixture gaps by emitting ETARGET instead.

- Session startup retry specs, Chromium and mobile Chrome: resume through the
  visible action, show original conversation, accept a new prompt once, and
  retain original session/native identity (005.1-.3, .6-.8).
- Same specs: installer failure and repeated missing dependency show one
  accurate final card/details; cancellation prevents replacement (005.6, .8).
- Settings host-utility and profile discovery specs: visible model availability
  after actual recovery, preservation of draft/selected model, no stale-context
  or agent-wide publication (005.5, .7).
- Docker and SSH managed-runtime specs, containers project: private installation
  occurs on the execution host; backend/shared-tree sentinels remain unchanged
  while a healthy sibling continues (005.2-.4).

## Work orders

- [ ] [Task 01: Verify and privately prepare bundled dependencies](task-01-executor-local-preparation.md)
- [ ] [Task 02: Recover task startup and resume once](task-02-task-recovery.md)
- [ ] [Task 03: Recover host and profile capability probes](task-03-probe-recovery.md)
- [ ] [Task 04: Prove executor and user outcomes](task-04-executor-and-browser-proof.md)

## Verification results

Design-only checks passed on 2026-10-19:

- `python3 scripts/list-docs.py validate`: passed; 389 decisions and 1656 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed; 36 tests.
- `git diff --check -- docs/specs docs/decisions docs/plans/managed-codex-dependency-recovery`: passed.
- Local `validateCoverage` preflight below: passed; all four work orders covered
  with zero reference errors. The projected production path intentionally
  exercises the coverage gate without changing production.

```bash
node - <<'JS'
const fs = require('node:fs');
const path = require('node:path');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/managed-codex-dependency-recovery';
const packagePaths = fs.readdirSync(dir).filter(n => n.endsWith('.md')).map(n => path.posix.join(dir,n));
const contracts = ['docs/specs/agents/requirements/managed-npm-runtime-recovery.md','docs/specs/agents/system-design/managed-npm-runtime-recovery.md'];
const fileContents = Object.fromEntries([...packagePaths,...contracts].map(p => [p,fs.readFileSync(p,'utf8')]));
const changedFiles = [...packagePaths,...contracts,'apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go'].map(filename => ({filename,status:'modified'}));
const result = validateCoverage({changedFiles,fileContents});
console.log(JSON.stringify({ok:result.ok,status:result.status,workOrders:result.workOrders,errors:result.errors},null,2));
if(!result.ok) process.exitCode=1;
JS
```

Product commands are defined in the work orders and have not run: implementation
has not started. Public docs remain unchanged until the product behavior ships.
The design package is committed; implementation work orders remain pending.
No subagents were authorized or launched.

PR review clarifications on 2026-10-22 explicitly require the live-bridge
missing-dependency browser case, distinguish selected bridge identity from
transitive provider resolution, qualify host probe eligibility in both ADRs,
and state the work-order diff-check execution point. These are planning-only
clarifications, not completed implementation or product verification.
After the clarifications, `python3 scripts/list-docs.py validate`,
`python3 scripts/lint-spec-files.py --all`, and
`python3 scripts/lint-spec-files.test.py` passed (36 tests); `git diff --check`
passed before staging. Exact-head CI/review and merge validation remain external
delivery gates, not completed work-order results.

## Risks

- A provider-child failure is not a top-level exit; conflating them bypasses
  process-ownership guards or misses the actual incident.
- npm can change transitive versions inside the same exact bridge's declared
  range. Preserve selection identity and report observations, not a false pin.
- Private cold installation may exceed the caller deadline or fail under
  registry policy. Stop visibly without a second recovery budget.
- Cleaning private files at request return or backend shutdown can break a
  surviving agent. Cleanup must follow agentctl-owned subprocess lifetime.
- Profile flights currently prohibit installs and use detached request
  contexts. A shared helper must preserve cache authority and cancel obsolete work.
- Existing managed-runtime command recognizers expect the canonical prefix.
  Keep logical command identity separate from the final internal private paths.
- A hard agentctl crash can leave an owned private root; this package does not
  add broad orphan deletion or persistent repaired-runtime promotion.
