---
status: draft
system: platform
created: 2026-10-14
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-CI-PERFORMANCE-007
---

# Complete CI completion budget

## Boundary and baseline

Platform owns verification latency. CI automation retains authorization; the
external-runner contract retains eligible-job and provider boundaries. This
continuation changes scheduling and test infrastructure, with no application UI.

The complete October 14 workflow set at `2705829f097d` took 30m03s:
backend run 38524205425, frontend 38524205486, E2E 38524205488.
Windows native execution took 27m59s; PostgreSQL 24m33s; frontend 24m18s;
normal E2E execution reached 18m01s inside a 28m42s workflow. These are diagnostic
observations, not same-source candidate comparisons. Refresh source and selection
for each experiment. PR #4457 already met the distinct 50% E2E-step target.

## Requirement mapping

| Requirement | Design |
| --- | --- |
| REQ-PLATFORM-CI-PERFORMANCE-003 | Complete disjoint test partitions and required joins |
| REQ-PLATFORM-CI-PERFORMANCE-004 | Attempt-specific hosted measurement |
| REQ-PLATFORM-CI-PERFORMANCE-005 | Occupancy and loaded feedback comparison |
| REQ-PLATFORM-CI-PERFORMANCE-007 | Full workflow budget and staged delivery |

## First candidate: independent frontend and Windows work

The [October 26 continuation](ci-performance.md#october-26-cache-and-partition-continuation)
defines the E2E dependency-cache repairs.
It also defines duration allocation within these same two frontend jobs.
Its work orders preserve this delivery target and the current job counts.

Run the existing frontend static checks and build independently of two built-in
Vitest file shards. Keep the same package script (including pretest generation),
production NODE_ENV guard, isolated projects, container, cache discovery and
worker-budget resolver. Register the two standard-tier jobs in the existing
runner planner. Each shard emits a distinct JSON report. Compare the complete
file/test identity multiset and outcomes against an unsharded same-source run;
retain unknown files in their existing full setup. The required frontend gate
joins the static job and every shard, rejecting failed/cancelled/unexpected
skips and absent reports. Keep the public check name unchanged.

Partition the native Windows command sequence into compilation, native platform,
persistence and settings groups. The initial process cohort stayed unchanged and measured 19m08s in hosted
job 115897903616. The two-job candidate still took 16m14s at its slowest. Partition the same native
inventory into four hosted cohorts. The native runner accepts `-cohorts=2|4`
and a bounded `-cohort` index; omitted/zero selection retains all cohorts and
the default count stays two for local compatibility. Enumeration and exact per-cohort
completion validation remain mandatory, and all hosted results join the backend
gate. Reports record the selected cohort so a single-job report cannot claim
full package coverage. Reject unsupported counts and out-of-range cohort numbers before starting work.
Move whole existing steps, retaining their commands and timeouts;
do not derive coverage from source-name guesses. Every prior native step has
exactly one owner. All groups remain on windows-latest and join the existing
backend aggregate. Targeted native compilation uses GOMAXPROCS=2 and a 2 GiB
Go soft memory limit on the existing hosted Windows runner, so cold race builds
do not thrash under the former 512 MiB heap. Test alarms stay intact.
Persistence/settings groups first compile their exact package union with the
same race/fts5 flags and a zero-match selector. The persistence compilation
allowance is twenty minutes; settings keeps its ten-minute allowance. This
bounded warmup permits two compiler packages and runs no selected assertions.
It does not change subsequent test/step deadlines or the full-job deadline.
Full-job measurement includes compilation.
Fail-fast stays false. This isolates compilation and groups
related tagged persistence checks without changing test selections.

## Subsequent bounded candidates

The first backend candidate isolates `internal/task/repository/sqlite` from the
ordinary package-hash jobs and partitions it into three hosted jobs. The remaining
package inventory uses four stable hash buckets after the second original bucket
exceeded the full budget; preserve native enumeration and exact union. The
explicit PostgreSQL 16 catalog retains one ordinary-packages job plus the same
three repository cohorts. Other packages keep their existing commands.

A narrow repository-only Python launcher enumerates native Go tests with the same
race/coverage flags as execution. It accepts only successful JSON package
completion and native top-level Test/Fuzz/Example names. Sort names and distribute
round-robin into three nonempty cohorts; use exact anchored selectors with no
slash so all nested subtests and fuzz seeds remain selected. Unknown new names
enter the inventory automatically. Reject malformed/duplicate/missing enumeration,
unselected execution, missing/duplicate terminal test records, or failed package
completion. The test command's exit is authoritative even when its report exists.

Normal repository cohorts retain race detection, atomic coverage and the
120-minute deadlock bound; PostgreSQL cohorts retain race detection, the
90-minute bound and only the existing nested SQLite million-fixture exclusion.
Each cohort uploads inventory, selected names, raw Go JSON, diagnostics and
coverage where applicable. All cohort results join the unchanged backend gate.
Keep the full explicit PostgreSQL catalog in the workflow and assert that its
sole repository exclusion delegates to all three required repository cohorts.
The launcher is not a generic scheduler and does not alter local test commands.

The workflow-sensitive sidebar resource lane is also part of complete acceptance.
Its memory job has 84 sort/group/direction cases plus alternating, preparation
and maximum-input checks. Partition only its whole isolated child scenarios into
three hosted groups. Group one retains the original three top-level tests and
skips only the five explicitly delegated pool-sort keys; group two selects
state/updatedAt and group three createdAt/title/custom. New sort keys remain in group
one by default. Preserve all nested groups/directions, 100-read plateau loops,
GOMAXPROCS=4 child measurement, native-memory limits and deadlines. Keep the
populated SQLite/PostgreSQL benchmark lane unchanged. Native Go fixture tests
prove complete disjoint leaf selection, including a future sort key.

For E2E retain duration-aware manifests, complete report reconciliation, guarded
one-worker processes, build identity and Docker/Kind coverage. Measure build,
container startup, test fixture work, browser execution and report overhead
separately. The first candidate uses 28 normal jobs, retaining two guarded processes per job
and six container jobs. Main profile 38554759444 predicts a 514.7-second slowest
process for the unchanged 3,808-test normal catalog, versus 1,027.8 at fourteen
jobs; measured queue and occupancy decide adoption. Run backend and web builds
concurrently inside the existing build job and join both before producing the
delivery fixture and identity manifest. A failed child remains a build failure.
The measured cross-platform helper builds move to a separate required hosted
job. Browser prerequisites compile only the native/Linux binaries they consume;
Linux/arm64, Darwin helpers and diagnostic CLI/winjob compilation remain required
in parallel. Reuse existing Make targets without changing local/release builds.
The report gate joins this job and rejects failure, cancellation or an unexpected
skip. Every further change gets a source-specific work-order update before implementation.

The browser-free build and report jobs bootstrap Go from `go.mod` (build only),
Node 24 and pnpm 9.15.9 on their existing hosted Ubuntu runners. They do not
pull a Playwright image. Browser and desktop jobs retain their existing images.
Build resolves and publishes a separate host pnpm store; report only restores
it. Both still install the frozen lockfile and execute every original operation.
Setup-go caching stays disabled in build because the shared Go cache action
owns its paths and validates installed compiler/environment compatibility.
Cold tool/cache setup remains part of complete elapsed measurement.

Container global setup owns one temporary canonical helper manifest per
Playwright invocation. Worker bundles and caches remain isolated and validate
the shared identity and Linux helper digest before using it. Teardown removes
the shared artifact; standalone fixture calls still generate their own manifest.
Kind receives only the final worker harness images in one batch, retaining all
real recipe builds and their layers while exporting their shared base once.
The pinned Kind loader accepts multiple image names and saves one archive.

Independent E2E helper compilation saves source-specific snapshots under its
existing job-role and dependency prefix. The browser build reuses the shared
optional Go cache action: module downloads use dependency/toolchain keys, while
compilation generations roll daily within installed compiler, architecture,
build environment and image-recipe compatibility. Source changes still invoke
every native Go build; Go's content-based invalidation remains authoritative.
Exact same-day hits avoid recompressing an unchanged generation before browser
jobs can start. Include cold population, rollover, compression, upload and restore
in measurements, retain trusted cache scope and keep cache failures optional.
Windows uses separate optional module-download and native-compilation caches;
setup-go caching is disabled so each directory has one cache owner. Compilation
generations roll daily in UTC within suite, architecture, installed Go version
and module identity, retaining their compatible restore prefix. All process
cohorts share one compiled-package key, published only by successful cohort one.
Other suites publish their own build generations; only the compile suite
publishes the common module cache. Exact hits skip saves, and every native
command remains unconditional regardless of cache availability or outcome.

Sidebar resource modes share an optional role/dependency Go module and build
cache. Native Go cache validation still decides which compilation can be reused.
Only a successful memory-1 mode saves the common cache; every mode executes its
original measurement command regardless of cache outcomes. Runtime child-process
isolation, memory budgets and evidence artifacts do not depend on compilation
cache warmth. Cache restore/save failures remain optional setup outcomes.

The four ordinary Go buckets preserve native discovery and stable hashing, with
three measured heavy packages assigned to the otherwise light third bucket:
backendapp, ACP adapter transport and coordinator. This explicit balancing rule
changes placement only; new packages still enter the hash allocation and every
selected package retains the original race/coverage command.

The earlier three-process experiment requested up to three normal processes
through its existing memory/CPU guard (12 GiB available,
four CPUs required for three). Each process still has one worker and disjoint
whole files; container jobs remain single-process. Measure contention and retries
before adoption, with no matrix growth or unsafe resource override.

The following four-process experiment keeps the same 28 normal jobs and six
single-process container jobs. A controlled four-CPU parent with three processes
passes all 122 selected identities (one existing skip and one retry), peaking
at 4.37 GiB with no swap or OOM event. It averages 3.34 busy cores, so a fourth
process is an experiment within the existing VM rather than another runner.
Its local preflight is rejected below ten GiB available; no four-browser local
performance result is claimed. Full hosted observations decide adoption.

Planned normal runs reserve two GiB, budget two GiB per isolated process, and
clamp capacity to one through four. Use the lower host/cgroup remaining memory;
resolve the current unified subgroup before reading its limit. Unknown memory
permits one process. Four processes require four available CPUs; smaller CPU
hosts retain the spare-core rule. Every process still has one Playwright worker,
whole files and a separate backend/database. Managed local runners retain their
existing maximum of three and six/twelve-GiB thresholds. No unsafe override,
provider activation or matrix growth is part of the experiment. Shell timing
preserves native exits; scoped cgroup counters are the CPU authority and missing
counters remain unknown. Resource artifacts never replace native blob evidence.

### One native E2E catalog producer

Build collects the complete configured Playwright inventory once using the
public reporter suite/test API. Each row retains the native test ID, project
and owning file suite rather than a helper declaration location. Both cohort
manifests derive their units and counts from those same rows. The existing
manifest artifact carries the catalog with checkout SHA, run ID and Build attempt.
Build publishes its producer attempt as a job output, so rerunning a consumer
can reuse that successful Build without accepting an unrelated artifact.
CI consumers require matching provenance and an unchanged checkout, validate
the full manifest against the catalog and current spec hashes, and fail closed
on missing, foreign or stale inputs. Local working-tree runs retain fresh native
discovery, including uncommitted tests.

The required report job compares the complete actual native identity multiset
with this catalog. Every selected case must appear exactly once after retry
aggregation, including existing skips; missing, extra, duplicate or unidentified
cases fail the report. Preserve test failures, retry policy and all required
job gates. This removes repeated imports rather than removing coverage. No new
provider, privilege or cache trust is introduced. Keep the original native
discovery path for local work and rollback; do not silently use it to conceal
an invalid CI catalog.

## Evidence, acceptance and rollback

Use one bounded hosted candidate at a time. Collect attempt-specific run/job/step
responses and original reports. A test command's nonzero exit remains failure;
artifact upload or later summary success never erases it. Missing evidence is
unknown or failure, never zero tests or success. Keep all attempted samples.

Frontend adoption retains three comparable samples, at least 30% shorter execution
and at most 10% extra runner-minutes against optimized unsharded verification.
For the complete goal, collect three fully selected successful workflow sets at
or below 900 seconds each, including waiting/setup/reporting. A changed source
requires explicit identity deltas, not an unexplained percentage comparison.
Record queue variability; do not infer p90 or a guaranteed ceiling.

Revert or revise candidates that lose coverage or worsen loaded feedback. No
billing settings, provider activation, global worker overrides, assertion deletion,
retry increases, skipped required platforms or manual approval bypasses are part
of this package. Existing protected workloads remain hosted.

## Delivery

[CI performance plan](../../../plans/ci-performance/plan.md), Tasks 14–16, owns
implementation and measurement. The maintainer explicitly requested continuous
execution through verified completion and merge; the design checkpoint does not
end this authorized loop. No delegation is authorized.

[Budget decision](../../../decisions/2026-10-14-complete-ci-latency-budget.md)
records the revised fan-out boundary.

### Container fixture affinity

Container scheduling treats the `containers` project's `tests/kubernetes/`
files as an atomic group because they share expensive worker-scoped Kind setup,
except the image-heavy kubernetes-worker-presets.spec.ts file, which owns one
existing shard when multiple shards are available. Its worker setup is absent
from native per-test timings. The remaining files balance across the other
shards; one-shard runs retain the complete catalog. The group retains each file's native catalog
identity and estimate; its scheduling weight is their sum. Other files retain
independent LPT placement. This changes
placement only: six container jobs and one worker per job remain fixed, and the
full-worker opt-in scenarios remain in the manifest with their existing gates.
Verify the grouped real suite's cleanup and elapsed time before adoption.
