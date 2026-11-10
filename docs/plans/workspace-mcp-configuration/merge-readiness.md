# PR 3293 merge readiness

Work resumed on another machine on 2026-10-20. The published checkpoint was
verified and conflicts were resolved with current main. Integrated local checks
and bounded source review are recorded below. Phone captures and the bounded
review through `7fdc73103` are complete. The latest conflict/CI round below
supersedes that checkpoint. Use the live PR and current task receipt for the
final commit's CI and review status. The user requires confirmation before merging.

The earlier receipts below are historical. Their local files and review session
are unavailable on this machine. They do not establish current readiness.

## Current-base integration and fixture repair

### CI follow-up after `dcecf2a37`

Exact head `dcecf2a37bed99a13ecd4c58bed0d07425ccf730` completed CI with
85 passed, 13 skipped, five failed and zero pending checks. Backend checks,
including PostgreSQL partition 3, passed. E2E run `40038394810` failed in
browser shards 22 and 24, container shard 5, and the aggregate. Walkthrough
run `40038392437` reached its 600-second generation deadline while gathering
source evidence; it produced no verified walkthrough. This result is not green.

The pseudo-locale secrets page reported stored `GitLab token` names as
untranslated copy. Its oracle now reads authenticated secret metadata and
exempts the matching row's name data element and the name interpolation
in a recognized translated secret-action attribute. It retains the surrounding action frame, bare labels,
other rows and identical text outside the row. A regression reproduces the
stored-name failure. Review also found that removing every name occurrence
could hide an untranslated action with the same words. A suffix-only
exception could also hide Delete for a record named elete. Negative controls
cover repeated, bare and suffix-colliding labels, plus matching data outside
the row. The final oracle retains every unrecognized label frame. No production copy, UI behavior or translation assertion is removed.

The archived-session recovery failure was a disabled Resume button after the
error disclosure appeared. Its unchanged focused local case passed; the
original disabled guard remains unidentified. The SSH failure showed failed
frontend recovery after backend restart while direct file reads succeeded.
An initial local SSH command instead stopped before that scenario because a
pre-commit runtime and the current-checkout helper manifest had different
commit identities. That setup failure does not reproduce the CI failure. After rebuilding the
runtime and helpers with matching identities, the unchanged SSH case passed
with one worker and zero retries through both reconnects. This local run used
an owned rootless Docker daemon and its existing host-network/cleanup wrapper,
so it is not an identical CI environment.
The final focused browser run passed all five cases (both collision controls,
secrets page, migrated-attribute control and unchanged recovery disclosure),
with one worker and zero retries. Scoped ESLint and documentation validation
passed. The isolated reviewer accepted the bounded two-file source candidate.
Remaining static, immutable-review and exact-head CI results are recorded in
the delivery receipt. No baseline or root-cause claim is made for either browser failure.

### CI follow-up after `ef96550e6`

Exact head `ef96550e6c89210cf381b02729efc52c4fd811d8` completed CI with
84 passed, 11 skipped, eight failed and zero pending checks. PostgreSQL
partition 3 and its aggregate failed in run `40034866189`. Browser shards
4, 5, 8, 12 and 23 and their aggregate failed in run `40034866252`. The
previous retention concurrency failure passed. This result is not green.

The PostgreSQL cancellation test received `driver: bad connection` from the
cursor read instead of an error discoverable as `context.Canceled`. The
projection retry boundary now returns successful attempts unchanged and joins
failed-attempt errors with a concurrent context error. It preserves the driver
or ambiguous commit error, performs no cancellation-triggered retry, and does
not infer rollback. Callback controls reproduced the lost cancellation before
the fix and cover successful attempts, ordinary errors and lock-contention
retry. The original real-PostgreSQL commit/cancel cases and these controls pass
under the race detector. This does not prove cancellation caused the driver
failure; it preserves both observed error conditions.

The queued-session absence helper used competing wall-clock and assertion
poll deadlines. It now observes the full same five-second window with one
deadline and checks the exact message list on every read, including the final
read. The narrow PR hover and phone task-action fixtures await finite drawer
animations before pointer actions. Phone submenu navigation also requires the
expected accessible title. CI showed a missing tooltip and the wrong submenu;
the original animation timing is not independently reconstructed.

The synthetic mode-mismatch fixture now seeds a session without a provider
process. A live mock provider previously competed with its injected mode state
through subscription replay. Mode text, touch geometry, focus, scrolling and
explicit set-mode request assertions remain. Saved fallback layout measurements
read the badge and its owner in one DOM evaluation after visibility. They keep
all containment and overflow assertions and require positive badge dimensions.
The previous separate bounding-box request returned null after a successful
text assertion; the original remount cause remains unproved.

The native candidate build passed. Four affected phone cases and two desktop
cases passed with one worker, zero retries, two CPUs and 4 GiB. The initial
browser command refused the stale backend binary before executing tests.
Scoped ESLint, formatting and documentation checks passed. The build embeds
the parent commit with a dirty marker; its tested source is the candidate
recorded in the delivery receipt. Immutable review, remaining static checks
and exact-head CI are recorded separately there. No public documentation
change is needed: cancellation error composition is internal, and browser
fixtures retain product behavior. No product UI timeout, touch minimum or
provider delivery behavior is changed by these fixture repairs.

### CI follow-up after `5c9b42656`

The exact reviewed head `5c9b42656335d56dfc05d889bf51cdf0c70ca754` completed
CI with 86 passed, 11 skipped, six failed and zero pending checks. PostgreSQL
partition 3 and its aggregate failed in run `40029587822`; browser shards
6, 25 and 26 and their aggregate failed in run `40029587688`. The routine-save
shard passed. Review threads remained clear and GitHub reported no conflicts.

The PostgreSQL failure was a collector commit deadline in
`TestDeliveryRetentionConcurrencyCanonicalEffectScopeFallback/foreign_incarnation`,
not a package timeout. The fixture deliberately parks the collector while
testing an independent operation, consuming the same 250 ms budget used in
production. A temporary real-PostgreSQL probe that held this barrier for 500 ms
reproduced the deadline. The transaction body is now extracted unchanged behind
the public method, which retains option normalization and the 250 ms limit.
Only the parked lock-proof fixture invokes the same body with normalized options
and a separate five-second context derived from its caller. Cleanup cancels,
releases and joins the collector. Existing lock, ownership and durable-result
assertions remain. Public-entry tests cover default and oversized requested
budgets, force expiry before payload mutation, and require an unchanged cursor
and unpruned inbox row. They passed before extraction too. This does not infer
rollback from an ambiguous commit response. The temporary probe passed after
extraction and was removed; it is not shipped test code.

The desktop scroll fixture matched both `filler message 120` and
`middle filler message 120`. Its two filler locators now match the complete
seeded text. The sidebar fixture reloaded after optimistic view selection
without waiting for preference persistence; the failed snapshot showed Other
selected instead of Repositories. Each selection now awaits a successful
settings PATCH matched by workspace and view before later reloads. Existing
query, row, recovery and reload assertions remain. Both affected desktop cases
and the phone sidebar case passed with one worker and zero retries.

Quick Chat's confirmation tap passed only on retry in CI. Its original cause
is still unknown; three unchanged local repetitions passed. Three unchanged
sidebar repetitions also passed before the persistence-wait correction. These
passes do not establish a baseline defect or exclude a new contribution.
The first real-PostgreSQL race run passed all selected concurrency checks,
public-budget controls and the temporary probe. After the cleanup correction
and probe removal, the final race run passed with 36 test/subtest results,
zero skips and zero failures against isolated PostgreSQL 16.15. Frontend
type checking, scoped ESLint, formatting and documentation checks passed.
Final build and review verification are recorded in the current delivery receipt; exact-head CI
remains a separate gate. No product timeout or UI behavior was changed.

### CI follow-up after `7c9fdc427`

The isolated review accepted the exact published commit
`7c9fdc4270673c18d690fed61d15fda70b478b90`. Its complete CI result was
90 passed, 11 skipped, two failed and zero pending checks. E2E run
`40026651456` failed shard 8/28 and its aggregate because the routine catch-up
policy save case passed only on retry. Backend, frontend, all other browser
shards, containers and walkthrough checks passed. GitHub reported no merge
conflicts or actionable review threads. This is not a green CI result.

The failing snapshot shows the selected draft policy while Save still displays
its disabled Saving state. The test called `waitForLoadState("load")` after
clicking Save; the current document already satisfied that state, so the test
could read persisted data before the PATCH completed. The test now arms an
exact-routine PATCH response observer and a future document load event before
clicking Save. It checks the response status, then retains the rendered-policy
and persisted-value assertions after the reload. No product behavior or timeout
changed. All four tests in that file passed with one worker, zero retries,
two CPUs and 4 GiB against the previously verified unchanged runtime build.
Scoped ESLint and formatting checks passed. The next commit's source review
and exact-head CI remain separate delivery gates.

### CI follow-up after `6a0ee44cc`

The isolated review initially accepted `6a0ee44cc1ed94c3ae962de9a90938146ef40e13`;
the later investigation below found missing resume-identity binding and a
synchronous event-publication deadlock.
Exact-head CI completed with 84 passed, 11 skipped, eight failed and zero
pending checks. Backend, frontend, container tests and walkthrough generation
passed. E2E run `40003935293` failed seven browser shards and its aggregate:
eight cases passed only on retry. The cancellation gate and isolated SSH
reachability checks passed. This is not a green CI result.

The original phone capacity trace now establishes a delivery race in the test.
The exhausted outcome was persisted at 18:50:07.783 UTC, but its conversation
notification reached the phone at 18:50:13.683 UTC, after the five-second UI
assertion had failed. Both browser contexts now wait for the session's exhausted
conversation notification before checking the existing UI outcomes. The event
wait uses the existing 180-second terminal-outcome budget; the UI assertions,
eleven-dispatch count, retained execution identity and ACP trace assertions
remain unchanged. Three focused repeats passed with one worker and zero retries.

Four desktop setup corrections retain their outcome assertions: match the
Office user comment exactly so an agent reply quoting it cannot also match;
use the existing in-app navigation helper before pressing the saved conflicting
shortcut; and wait for executor-picker focus restoration before opening the
next picker in the branch refresh and URL repository reopen cases. The latter
setup races are supported by source inspection and the disappearing picker
trace, but their complete CI event ordering was not captured. All four cases
passed twice under two CPUs and 4 GiB, with one worker and zero retries.

All six desktop cases passed in an unchanged-source focused replay. The desktop
partition containing the pseudo-locale failure also passed all 29 cases under
two CPUs and 4 GiB. These passes do not explain the original locale or canvas
failures. The canvas partition replay passed 32 cases, skipped three and failed
one unrelated Japanese time-width case. Its screenshot contained missing glyphs;
the host font lookup had no Japanese font. The unchanged case passed after a
private fontconfig file exposed the already-installed Noto CJK font. No font,
product or geometry assertion was changed. Current-main E2E passed, so no
baseline-failure claim is made for the original CI cases.

The wrapped-terminal trace shows workspace creation preceding promotion into
passthrough resume. Its ready callback can omit the active resume identity:
passthrough startup returned before binding it, and promotion did not inherit
the current identity from execution creation. The existing ownership guard
then rejects readiness. A real-PTY regression reproduced the empty callback
identity through Launch promotion and StartAgentProcess. Startup now binds the
admitted identity before either dispatch path, after checking cancellation.
The lifecycle regression also requires direct PTY follow-up receipt and completion;
it does not exercise orchestrator admission. A cancelled-start control preserves
the previous identity and generation.
The CI trace does not record the rejected callback identity or prove the final
stdin stall, so that full failure is not claimed as deterministically reproduced.
This restores the existing resume contract; public configuration and recovery
instructions do not change. The guards for stale attempts remain in place.


After rebuilding that candidate, the desktop native-wrapper case still failed;
the ACP restart case passed. A scoped local goroutine capture then confirmed a
second fault: ordinary prompt admission held the session cancellation mutex while
synchronously publishing AgentRunning, whose subscriber tried to lock the same
mutex. PTY input could not begin. A concurrent ready callback also waited for that
mutex while holding a startup callback read lease. A bare delayed RUNNING event
would risk reviving a session after immediate READY, and an added startup write
lock would introduce another deadlock.

The repair keeps RUNNING publication synchronous before PTY writes. A dedicated
callback checks actual admission ownership and passes an exact-session marker
through lifecycle publication. The running handler consumes that marker at entry,
executes under the borrowed guard, and removes it from downstream context. Other
sessions and unmarked events retain normal locking. Admission stays held through
all chunks and successful acceptance; failed or partial writes do not report
successful acceptance. No new lifecycle lock or delayed RUNNING event is added.
A deterministic regression first demonstrated synchronous lock reentry. Focused
coverage exercises ordinary prompt admission, failure before/after a chunk,
immediate READY settlement, context propagation and unmarked/different-session
controls. The focused race suite passed in orchestration, executor and lifecycle.
Additional admission/publication rejection controls passed under race. The native
runtime and fixtures rebuilt successfully. Desktop native-wrapper and ACP restart
cases each passed twice; the phone native-wrapper case passed twice. All six
checks used one worker and zero retries under two CPUs and 4 GiB. The tested
binary embeds parent `6a0ee44cc` with a dirty marker; its recorded source blobs
identify this candidate. The first Go lint run reported two issues and reached its five-minute limit:
an explicitly ignored, already-logged event-publication error and a test switch
style correction. Both are corrected; the retry passed with zero issues using a bounded 5 GiB Go heap.
New exact-head CI remains pending until recorded below.

The diagnostic browser repeat passed once and failed once with the confirmed
lock cycle. An earlier diagnostic attempt failed at initial idle, before the
follow-up; its cause remains unknown and its stack does not establish this
cycle. These captures are separate from the original CI failure, which lacked
a goroutine dump. The first new regression run also exposed an invalid test
fixture assumption: its onAccepted hook needs a saved turn ID, but that fixture
has no turn service. It now checks the existing afterDispatch acceptance boundary.

The final lint-only rebuild passed both desktop wrapper cases, but its phone
replay failed the unchanged wrapper receipt count: three starts instead of two.
The backend log proves two local PTYs were installed for the same execution:
terminal reconnect started one process before the promoted orchestrator resume
started another. This is a third source defect, separate from the identity and
synchronous-publication faults above. Real-PTY regressions reproduced both a
second direct resume replacing a live PID and promotion replacing the terminal PID.

The candidate now rechecks current process ownership under the lifecycle locks
for local fresh and resumed starts. A live or pending PTY is reused; dead-process
replacement and stale exit-callback rejection remain. Promotion preserves a live
process's runtime status and binds the admitted identity to its startup generation.
Readiness reconciliation is restricted to the current unaccepted resume while
SQL still says STARTING, with no active/reserved/durable turn and proof of a live
local process, completed first-idle callback, current READY status and matching
attempt identity. It uses the existing conditional state writer, without a
synthetic completion event or prompt acceptance. Process-lock contention defers
the proof instead of blocking under the session admission guard. A failing
regression first established the missing idle reconciliation. A generation-zero adoption regression also failed before the candidate stopped
advancing a live process's generation and resetting its settings projection.
The first combined run passed lifecycle and process tests, while the new service
fixture exposed use of an unavailable executor wrapper. The helper now uses the
runtime manager's context-aware execution identity lookup, as other readiness
checks do. The expanded focused race run passed in orchestration, executor, lifecycle and
process packages. Scoped lint found one complexity-limit violation in the new
readiness helper; its idle ownership/turn check was extracted without dropping
conditions. The isolated reviewer then identified a replacement-before-binding
window. A deterministic regression confirmed a stale observed execution could
bind its attempt to the successor. Binding now uses the observed execution pointer
and revalidates it under the existing startup writer and store lock, rejecting
rotation without acquiring process locks under the writer. The final focused race
run passed all four packages, including existing binding and MarkReady controls.
The isolated reviewer accepted the revised ownership guard at source level.
The next lint run reported zero issues but reached its five-minute limit, so it
is incomplete rather than passing. The warmed-cache retry passed with zero
issues and the runtime/fixtures built. Subsequent bounded review found that
resumed dispatch must retain the same observed execution too. Both binding and
dispatch rotation controls failed before that correction. Startup now passes its
expected execution into resume, while session-only terminal reconnect resolves
the current execution before entering the same process boundary. A rotated
observation rejects instead of falling through as a process without a live PTY.
An expanded routing run exposed a legacy sessionless-launch regression in the
new guard. The guard now validates execution ID and pointer, preserving the
unchanged legacy test while rejecting removed/replaced executions. That expanded
race run passed. Lint then flagged the renamed resume function's inherited
complexity. Process ownership and process launch are now separate helpers under
the same lock lifetime, and post-launch bootstrap reuses the existing equivalent
stream-connection helper. The final focused lifecycle race run passed, including
startup routing, binding, completion and reset controls. Final lifecycle lint
passed with zero issues and the runtime/fixtures rebuilt successfully. The rebuilt
binary was recorded against exact Go source hashes. Both desktop wrapper cases
passed; the phone run passed once and failed once before restart while waiting
for initial idle. It did not reach the duplicate-start assertion. The trace and
backend log were retained; no baseline or fully reproduced CI-cause claim is made.

That failure's log records initial READY and workflow completion, followed by
SQL RUNNING and rejection of the workflow's next prompt as already running. The
startup callback read STARTING but passed no expected state to its later state
transition, so a fast completion could settle WAITING between those reads and
then be revived. A deterministic repository interleaving reproduced that state
revival. The callback now supplies expected STARTING to the existing guarded
transition and conditional SQL writer. This adds no timeout, new state primitive
or relaxed browser assertion. The focused executor startup/admission race run
passed. Lint requested a selector simplification in the new test fixture; that
style-only correction is applied. The final focused executor race run passed,
scoped lint passed with zero issues, and the runtime/fixtures rebuilt. Three
phone wrapper repetitions and both desktop wrapper cases passed on that final
source, with one worker and zero retries under two CPUs and 4 GiB. The runtime
binary embeds parent `6a0ee44cc` with a dirty marker; its hash and exact source
hashes are recorded separately. Original failed runs remain evidence, and these
local passes do not replace new exact-head CI. Final immutable source review and
publication follow this checkpoint.

### CI follow-up after `86653face`

The existing isolated review accepted `86653face8a2da662e8496534e5700acbf0b7117`
against integrated base `144e9e503f82001bb4d1440b603f7c33b932dd0a`, with no
actionable source findings. Exact-head CI ended with 81 passed, 12 skipped and
nine failed checks. Backend run `39997136290` failed its Windows process shard
while downloading `google.golang.org/grpc@v1.83.2` from `proxy.golang.org` with
HTTP/2 `INTERNAL_ERROR`; no repository assertion failed in that job. E2E run
`39997136328` failed five shards and its aggregate because SSH reachability,
last-prompt scrolling, phone capacity feedback, acknowledged silent cancellation,
and resumed wrapper input passed only on retry. The earlier scheduler and drawer
repairs passed. Walkthrough run `39997133439` reached its 600-second generation
deadline without verification. These are failed checks, not a green result.

The cancellation failure trace shows an eight-second mock acknowledgement during
a roughly ten-second reload. The test now releases that acknowledgement explicitly
after its pending-state/reload assertions, then requires real completion. Its
acknowledged-case join bound is the existing 120-second test budget instead of
12 seconds; the uncertain case retains its 12-second join and 120-second hold.
The mock release wait is bounded and survives ACP request-context cancellation.
Two deterministic unit failures established premature timed acknowledgement and
premature acknowledgement through a cancelled request context. An intermediate
browser candidate failed both phone cancellation replays because of that context
boundary; those failures are retained separately from final verification.

The SSH threshold fixture now disables scheduled probes within its scoped backend
environment. Explicit real-SSH probes alone advance the counter, preserving the
first-failure, second-failure and single-success assertions. Source inspection
establishes the competing automatic probe path; the original CI interleaving has
not been reconstructed. The existing zero-interval/manual-probe tests remain.

Four traced scrolling replays and two phone capacity replays passed with one worker
and zero retries. Their CI causes remain unknown. The reviewer found no proven
wrapper/MCP integration defect and identified that task deletion obscured failure
screenshots. First-failure traces and pre-cleanup wrapper page, API and backend
evidence now preserve that boundary without changing the output assertion. No
product behavior, public configuration or public documentation contract changes.
Final candidate browser checks passed: three desktop and three phone cases cover
wrapped terminal resume and both acknowledged/uncertain cancellation; two real-SSH
replays cover manual hysteresis. All used one worker and zero retries. The private
rootless SSH fixture used the previously verified host-gateway and cleanup wrapper;
its daemon was stopped after confirming no containers remained. An initial SSH
command stopped at the stale delivery-helper check before any test; rebuilding
the helper and plugin fixture cleared that prerequisite without bypass.

Complete mock-agent and reachability packages passed with the race detector.
Typecheck, scoped Go/web lint, TypeScript formatting, catalog/specification and
whitespace checks passed. The cancellation tests were moved to a dedicated file
after the initial Go lint file-length failure. Native fixtures were rebuilt from
the candidate source; their embedded identity remains dirty parent `86653face`,
not the final commit. Immutable review and fresh published-head CI remain required.

### CI follow-up after `d0ca264b5`

The published merge commit `d0ca264b5fb27c2a313ce7eeb6f7f4baea699d47` is
mergeable against integrated main `144e9e503f82001bb4d1440b603f7c33b932dd0a`.
The existing isolated review verified its parents, exact reviewed-tree equality,
nine conflict resolutions and 71 shared automatic intersections, with no
actionable source findings. The review's remerge-stat command emitted a partial
clone object-fetch diagnostic; its verdict rests on direct source/tree comparisons,
not an unqualified remerge reconstruction. Normal hooks passed without bypass
after a shared lint lock failure and a five-minute timeout; the successful retry
kept the original limits and used the supported serial-runner option.

Exact-head CI ended with 84 passed, 10 skipped and eight failed checks. Backend
run `39992086832` failed the workflowsync scheduler test in shard 2 plus its
aggregate. E2E run `39992086867` failed five shards plus its aggregate because six
cases passed only on retry: unloaded last-prompt scrolling, native wrapper resume
input, mobile Stats read recovery, mobile retention action/status recovery,
mobile workflow-approval link geometry, and active-task Go-cache cleanup. The
required backend and E2E gates are therefore red. There were no pending checks,
unresolved or hidden review threads, or actionable issue comments. Both backend
and E2E runs on exact base `144e9e50` were green; they do not prove these failures
are baseline failures or exclude a new interaction.

The scheduler test failed five of 1,000 local race-enabled repetitions. It started
a worker that could consume the job before `close` inspected the queue, so its
exactly-once discard assertion did not consistently exercise the named queued-job
case. The test now constructs the scheduler queue without a worker; existing
worker/cancellation tests remain. The unchanged discard assertion passes with
100 repetitions of the scheduler tests, and the complete workflowsync package
passes three times with the race detector.

The approval-link test measured 43.999969482421875 pixels against the strict
44-pixel minimum while its drawer could still be moving. It now waits for the
existing finite drawer animations before measuring, retaining the same assertion.
The other three desktop cases each passed two unchanged-source local replays,
one worker and zero retries. Those passes do not establish their CI root causes.
All three phone cases also passed twice, including the repaired geometry check,
with one worker and zero retries. Five browser CI causes remain unproven despite
these focused passes. Scoped Go/web lint, formatting and documentation checks
passed. Final delta review and fresh published-head CI remain required. No
production behavior or timeout was changed.

### Main integration on 2026-10-23

The pre-merge published head is `193de56ba2597b335a97b1e5ab01d09340f22647`.
This merge integrates main `144e9e503f82001bb4d1440b603f7c33b932dd0a` from
previous base `c17a4691aa0658070ea2a1cb26d05641bf8c1874`. The pre-merge
head had 92 passing and 11 skipped checks, including all six required checks.
Those results do not establish CI status for this integration.

Nine file conflicts were resolved. Passthrough commands retain main's launch
wrapper and strict profile token validation together with prepared MCP delivery
and acceptance. Instance authentication records both the consumed control
credential and main's narrower authenticated-control capability. Bearer and
adoption credentials do not acquire wrapper authorization. Main's prompt-preview
interaction implementation retains Escape dismissal and delayed-hover guards.
Workflow move preview keeps both competing-preview injection and completed
response-body observation.

The WebSocket test helper now distinguishes request predicates (`requestWhere`)
from main's response predicates (`where`). Existing PR callers were migrated;
three initially failing helper tests pass. An automatic merge also omitted the
repository owner's discard callback from the extracted state hook. A focused
Reset regression failed before restoring its type and callback wiring, then
passed. The incoming credential-rotation LSP fixture assumed `/bin/cat`, which
is absent on this Nix host; it now resolves `cat` through the inherited PATH.
No authentication or stream assertions were weakened. The prompt-hover browser
fixture attempted to pause its clock at an already elapsed instant; its pause
target now includes a one-second margin before pointer actions, with all existing
interaction assertions retained.

Candidate validation passed: 80 focused frontend tests, web typecheck, full web
lint, localization checks, scoped Go lint, documentation catalog/spec lint, and
race-enabled lifecycle, API and orchestrator checks. Native and web builds
contain the integrated source but embed the pre-merge head with a dirty marker.
Eight desktop browser checks passed before the clock repair; the repaired hover
case and desktop continuation cancellation then passed with one worker and zero
retries. An initial nine-case browser invocation failed in fixture setup:
inherited launcher configuration bound the backend to a non-loopback address,
while readiness checked localhost. The rerun removes inherited internal config
paths and explicitly binds the disposable backend to loopback. These setup
failures are not passing product coverage. The first phone run passed three
checks but failed workflow seeding in its first worker. That worker used port
18107; a concurrent backend from another task was confirmed listening there.
The phone rerun passed all four checks with one worker, zero retries and an
explicit, checked-free port offset. This establishes a port collision, not the
exact source of the initial null workflow-step response. Final
immutable source review and published-head CI remain separate gates.

### Cancellation ordering and browser fixtures after `eb8696da`

Head `eb8696da28ff830a0d4ec74092d0b72b76024dc3` remained mergeable against
main `c17a4691aa0658070ea2a1cb26d05641bf8c1874`. Its CI ended with 84 passed,
13 skipped and six failed checks. E2E run `39540523893` failed four shards
and the aggregate. Shard assignments changed, so failure comparisons use test
identities. The phone uncertain-cancellation and temporary-read failures
recurred. Initial-turn 529 retry and desktop conflict auto-fix also failed
before passing on retry. Walkthrough run `39540519774` reached its 600-second
generation deadline; verification did not run.
The complete blob audit found 4,148 attempts: 4,103 passed, 40 skipped and
five failed attempts, with five retries. Passing retries do not satisfy the
fail-on-flaky policy.

The new phone cancellation log showed local cancellation marking an execution
failed before runtime-loss classification. Its durable submission remained
unsettled, but classification skipped failed executions. A regression reproduced
the missing uncertainty event. Runtime loss now reconciles failed executions with
a pending submission while preserving their status and submission identity.
Settled failed executions remain unchanged. The focused runtime replacement
suite passes with the race detector.

The initial 529 snapshot already showed attempt 3 while its assertion waited for
attempt 1. The test now proves cached initial-prompt replay through persisted
retry advancement and checks the visible retry card and cancellation.
The phone task-read test waits for session hydration and the named foreground
coalescing window before sending its next online event.

The auto-fix fixture's delay was inside a system block that the mock provider
discarded. The supposedly active repair could finish and legitimately permit
another round. The mock now recognizes an explicit script block on an auto-fix
prompt. A regression proves the scripted repair stays active until cancellation;
ordinary prompts and non-script blocks do not select this behavior. Both desktop
and phone tests require the provider's acceptance marker before repeating feedback,
then disable automation and stop their fixture session. The complete mock-agent
suite passes with the race detector.

Nine phone checks passed: three repeats each of uncertain cancellation,
temporary-read recovery and conflict auto-fix. Twelve desktop checks passed:
three repeats each of acknowledged cancellation, uncertain cancellation,
initial-turn retry and conflict auto-fix. All runs used one worker and zero
retries. Native and helper builds, scoped frontend lint, formatting and
documentation validation passed. The tested runtime contains the repaired
working tree and embeds parent `eb8696da` as its version identity.
Bounded source review and fresh committed-head CI remain required.
These internal repairs do not change public configuration or intended workflows.

### Recovery serialization after `740cc655`

Head `740cc655086df36a1c691186b26dfd77c6deb982` includes current main
`c17a4691aa0658070ea2a1cb26d05641bf8c1874` and has no merge conflicts.
Its CI ended with 84 passed, 13 skipped and six failed checks. E2E run
`39536080140` failed four shards and the aggregate. The complete blob audit
found 4,148 attempts: 4,103 passed, 40 skipped and five failed attempts, with
five retries. Passing retries do not satisfy the fail-on-flaky policy.
Walkthrough run `39536076371` exited after its model provider reported
`Endpoint is unavailable`; verification did not run.

The desktop uncertain-cancellation failure exposed a backend panic. A workspace
lookup joined the session's cleanup operation and asserted that cleanup's nil
result was an execution pointer. A deterministic regression reproduced this
panic through both session lookup entry points. Cleanup and promotion now retain
their own results while sharing serialization with execution creation. Concurrent
lookups wait for those operations, then run execution admission. Cleanup also
runs after a concurrent workspace read succeeds or fails. Promotion rejects an
execution that cleanup has removed. Focused lifecycle tests pass with the race
detector, including the reproduced boundary and existing recovery and promotion
coverage.

The file-tree restoration snapshot showed rows near the bottom after a helper
searched past the first row before it mounted. The test now requires the top row
to become visible without a scrolling search, preserving the viewport under test.
The branch-ranking test waits for the previous executor picker's focus restoration
before opening the branch picker. Its exact-match ordering assertion is unchanged.
Phone cancellation now retains first-attempt traces; failed silent-turn cases also
attach their disposable provider trace and backend log. The phone uncertainty
timeout and transient task-read trigger did not recur in three local repeats
each. The latter test remains unchanged; its CI cause is still unproven.
Twelve desktop cases passed without retries, covering both cancellation outcomes,
branch ordering and file-tree restoration. After correcting a branch-test override
that forced local retries on, three final branch repeats passed with retries zero.
All runs used one worker. Native and helper builds, scoped frontend lint,
formatting and documentation validation passed. Fresh committed-head CI and
bounded source review remain required.
These internal repairs do not change a public configuration or user workflow.

### E2E repair after `43536158`

Bounded source review accepted `43536158067763bcf47b453384176aee81f865b7`
against main `c17a4691aa0658070ea2a1cb26d05641bf8c1874`. Its CI run
`39530299150` ended with 86 passed, 11 skipped and six failed checks.
Five E2E shards and their aggregate failed. The complete blob audit found
4,149 attempts: 4,103 passed, 40 skipped and six failed attempts, with six
retries. These passing retries do not satisfy the fail-on-flaky policy.
Backend, frontend and walkthrough checks passed.

The phone mode-picker close control measured `43.999969482421875px` in CI.
Its nominal size is now 48px; the minimum 44px assertion remains unchanged.
Coordinator ordering is measured in one browser evaluation after menu motion
finishes. The phone task-list test waits for picker motion before selecting
an option and requires dismissal before checking the saved URL and row order.
The first-attempt trace showed the original sort tap during picker motion.

The upload snapshot showed Changes active, with the uploaded file present.
The Default layout intentionally activates Changes on new Git updates.
The test now observes that file there and returns to Files before checking
the file tree. The cancellation fixture previously allowed acknowledgement
after eight seconds while runtime interruption was still pending. Only its
uncertain variant now holds acknowledgement for the full test duration and
requires one open turn before killing the runtime. Both cancellation variants
wait for the mock provider's private trace to record receipt without chat output.
The original uncertain
phase assertion remains. This removes a fixture race; the complete CI
recovery failure cause was not proven from a first-attempt trace.

Unchanged local reproduction passed nine phone and six desktop cases.
After the phone repair, nine repeats passed with one worker and zero retries.
The web build, typecheck, scoped lint and formatting passed. The phone mode
picker was captured again and visually inspected. Nine desktop fixture repeats
passed with one worker and zero retries, covering both cancellation outcomes
and upload navigation. Three SSH restart repeats passed without retries.
The CI reconnect timeout did not reproduce; its cause remains unknown.
Initial local SSH attempts skipped for
a missing Docker client, then failed during container image setup because
the disposable daemon could not resolve package hosts. An explicit working
DNS resolver repaired the image build. Later setup attempts failed because the
test binary carried an older commit than the generated compact manifest.
Rebuilding the native runtime aligned those identities. A local rootless-Docker
adapter also made the host Git fixture reachable and restored fixture mount
ownership before cleanup. These setup failures are not reproductions of CI's
reconnect timeout. Final provider-receipt checks passed both cancellation outcomes
on desktop and phone. The final phone run also checked both dimensions of the
mode-picker close target and captured the dialog again. All used one worker
and zero retries.
First-attempt traces are retained for desktop stall recovery and SSH passthrough.
Fresh committed-head CI and bounded source review remain required.

The prior published repair passed bounded source review. Its first CI attempt
had 85 passed, 11 skipped and seven failed checks. The backend cancellation case
passed 50 local race-enabled repeats; four browser cases passed three local
repeats each. A single failed-job rerun passed those cases, the backend gate and
the walkthrough, but ended with 90 passed, 11 skipped and two failed checks.
E2E shard 10 and its aggregate failed on two different flaky cases. Passing
retries do not satisfy the fail-on-flaky policy.

The chat fixture seeded transcript filler through an endpoint that creates an
open turn. After the preceding real turn had completed, that synthetic turn
could leave the composer busy. A direct assertion reproduced one open turn
where zero were required. The fixture now waits for the real turn to complete,
seeds filler in its own turn, and completes that turn before the next prompt.
The quorum rejection fixture also returned to Work with automatic agent launch
enabled, racing its next manual move with a STARTING session. A scoped fixture
removes only that Work entry action and restores the original events afterward.
Review's decision clearing, guard assertions and manual transition remain intact.
These changes repair test setup; no public behavior or documentation changes
are needed.

Main then advanced to `c17a4691aa0658070ea2a1cb26d05641bf8c1874`.
The session-launch conflict retains both MCP selections and main's prepared
primary-session reuse. The new base's dependency guards and phone coverage are
preserved. Integration verification passed 120 top-level backend tests (186
test/subtest results) across the orchestrator and MCP handlers with the race
detector, with no failures or skips. All 12 affected desktop cases and eight
phone/status-bar cases passed with one worker and zero retries. Go lint reported
zero issues. Typecheck, scoped frontend lint, formatting, native/helper/web builds,
catalog validation and specification lint passed. Bounded source review of this
delta and exact-commit CI remain pending.

## Main integration after `b21700025`

CI for the published merge `1f1a19a9050da858f71ea31cb36f7fa730f7af61`
finished with 79 passed, 13 skipped and 11 failed checks. One backend shard,
seven flaky browser shards, their aggregate gates and the optional walkthrough
failed. All seven browser cases passed on retry, which does not satisfy the
fail-on-flaky gate. The walkthrough reached its 600-second generation deadline
without running verification.

The backend failure reproduced after waiting for silent startup to release its
attempt. The earlier binding guard required prompt acceptance and therefore
replaced the callback owner of a successful silent restore. Silent startup now
records prompt readiness separately from provider acceptance. Failed, cancelled
and superseded attempts still require a new binding. This preserves the existing
recovery contract and needs no public configuration or workflow change.

Browser remediation retains the original assertions. Geometry checks wait for
finite animations. The phone filter picker still overflowed after that wait:
Floating UI rounds its edge-aligned position to device pixels. An 8px collision
inset and viewport width bound keep the actual surface inside the screen.
Workflow pickers wait for dismissal and focus restoration
before the next interaction. The uncertain-cancellation case polls the persisted
phase because the retry button also appears during reconnection. Three other
failures did not reproduce locally; their specs now retain failure traces.
Their causes remain unknown.

On this machine, the first localized-width reproduction lacked Japanese fonts
and rendered missing-glyph boxes. An isolated Noto CJK font configuration fixed
that environment limitation. The original 44px minimum remains unchanged.
The first focused Go compilation exceeded its 4 GiB scope and was killed by the
kernel. The repeated run uses an explicit 8 GiB scope. Neither initial failed
command is a passing receipt.

The repaired backend regression and ownership guards passed three race-enabled
repeats. The broader recovery selection passed 104 top-level tests and 138
test/subtest results, with no failures or skips. Go lint, frontend lint,
formatting, typecheck, native/helper/web builds and documentation checks passed.
The final desktop run passed seven cases, including coarse-pointer workflow
controls, selected filters and localized geometry. Four affected phone cases
passed twice each. The filter overflow still failed twice before the inset fix,
then passed three repeats after rebuilding the web runtime. Browser runs used
one worker and zero retries. The final filter screenshot was inspected.
The source review and CI results for this repair are superseded by the
current-base record above.

The 45-minute waiter for `b21700025` ended with eight passing checks, twelve
skipped checks and six missing required checks. No registered check failed.
Main had advanced to `21949f6362e6830b9fb9162d2ca6ba41c5900331`, which conflicted
with the branch. Missing checks were not running checks, and this was not a
passing CI result.

The merge retains the startup test's existing three-second recovery bound,
both MCP and conversation-search API exports, and both sides of the workflow
preview coverage: settled task/session projections, exact destination identity,
and an injected competing preview request. Incoming session restart, silent-turn
recovery, composer layout and persistence behavior remain intact.
The merge hook also found a literal in incoming stall handling that duplicated
the branch's existing session-ID constant. Reusing that constant preserves the
value and resolves the integration lint finding.

On the merged candidate, 110 focused frontend tests passed. The two affected
runtime packages passed a focused race selection with 68 passing test/subtest
events. Typecheck, resolved-file lint/format, localization checks, specification
lint and the documentation catalog passed. Sixteen merged locale files parsed
without duplicate keys. Native, helper and web builds passed before browser
verification. An initial broad Make target was stopped and replaced with the
separate native/helper targets; it is not a successful build receipt.

Sixteen desktop browser cases passed with one worker and zero retries, covering
fallback, resume projection, restart with live siblings, silent-turn recovery
and all three workflow-preview cases. The first phone run passed twelve cases,
including MCP geometry at 320/393/767px, and failed one scroll-to-start assertion
in the dense composer case. That failure did not reproduce in three isolated
runs or three repetitions of the preceding interaction sequence. Those passes
do not establish its cause. The original assertion remains, and the spec now
retains first-attempt failure traces. Temporary pointer/scroll diagnostics were
removed from the delivery tree. A final run of the original six-case phone
composer spec passed with one worker and zero retries. The initial failure
remains recorded; it is not reclassified as fixed or baseline-only.

Exact-head CI and the final bounded integration review remain required. Earlier
broad command timeouts and coverage limits below remain unchanged.

## CI remediation after `cbc9ad6c3`

The exact-head waiter completed with 76 passing checks, 13 skipped checks and
14 failed checks. The failures were 13 E2E shards and their aggregate. The retry
summary records 4,055 first-attempt passes, 18 flaky cases and 40 skips. Every
flaky case eventually passed, but fail-on-flaky correctly leaves CI red.
Backend, frontend, documentation coverage and walkthrough checks passed.

Retained failure evidence identified four causes with focused red/green tests:

- A restored workspace could retain resume intent when promoted for a fresh
  fallback successor. Promotion now uses the admitted launch request's intent,
  so the successor receives its initial prompt.
- A failed resume during startup reconciliation left recovery history behind.
  A restored execution then bypassed binding, so its completion callbacks were
  rejected and the session stayed RUNNING. Prompt admission now requires an
  accepted binding for that execution or creates a new binding. Existing stale
  callback and cancellation guards remain enforced.
- The task context menu could restore focus to the row after Rename opened.
  The dialog focus trap then selected the whole draft. Rename now opens after
  the menu releases focus. The earlier external-focus-hook experiment did not
  solve this race and was removed.
- The mock agent's slow response used delays that ignored cancellation. Its
  delays now honor the prompt context, allowing acknowledged cancellation.

Affected E2E setup now waits for persisted Azure preferences, settled drawer
geometry, original-session message persistence across workflow handoff, and
initial session/Git hydration before Files interactions. Folder upload waits
for the intentional Changes update before selecting Files. Scroll preservation
is measured after the reader disables transcript auto-scroll. Profile dragging
retries only row scrolling before pointer-down when a refreshed row detaches.
The workflow handoff assertion now checks original-session persistence. Other
assertions, minimum touch sizes and existing timeout values are retained.

The three affected Go packages passed the focused race run. Frontend typecheck,
scoped lint and formatting passed. The rename flow passed nine browser cases
across all three surfaces, plus four phone rename/caret checks. Desktop fallback,
phone fallback and cancellation,
MCP 320px status, turn-file visibility, workflow routing and history recovery
passed with retries disabled. The real Kubernetes sibling stop/restart case also
passed with the rebuilt static runtime. Prompt-permission and clone failures did not
reproduce in three local repetitions each. Their cause remains unknown, as do
the isolated CI failures in MCP report state, phone workflow selection and
turn-file visibility. Those specs now retain first-attempt failure traces.
All local browser runs used one worker, zero retries and a 4 GiB/two-CPU limit.

The latest base remains `74427cb50c24e281e5b3fa826c7136ffed5be5db`.
Its incoming files do not intersect this remediation. Current-head CI, the
final bounded review and user merge confirmation remain delivery gates.

## CI remediation after `7fdc73103`

The next exact-head waiter completed with 85 passing checks, ten skipped checks
and six failed checks. All six failures came from five flaky E2E cases and their
aggregate. The complete blob audit found 4,073 passing attempts, 40 skipped
cases and seven failed attempts. The retry summary identifies five flaky cases.
Backend and frontend CI passed. Walkthrough generation, publication and linking
also passed on this head, superseding the earlier walkthrough failures below.

Three test timing races have deterministic local reproductions and repairs:

- Workflow auto-advance now holds the mock turn until the In Progress card is
  observed. The prior three-second delay could expire during navigation. A
  delayed-navigation reproduction failed before the change and passed after it.
- Saved-view reload now waits for the selected view and cleared draft to reach
  persisted settings. The optimistic chip alone did not prove that the queued
  write had completed. A delayed-write reproduction failed before the change
  and passed after it.
- Monitor checks now wait for the backend's agent Monitor message before the
  existing card assertions. A delayed-start reproduction failed before that
  boundary and passed after it. Event counts, card content and hidden-envelope
  assertions remain unchanged.

The dynamic fallback and Kubernetes sibling-restart CI timeouts remain
unexplained. Each target passed unchanged in isolated runs and broader local
replays. Those replays were not fully green: the Kubernetes sequence recorded
16 passes, four skips and an in-cluster startup failure; the fallback prefix
recorded 36 passes and a restoration test failure where terminal wrapping split
the expected output. A failed predecessor restarted the worker, limiting the
ordering evidence. These results do not prove that the PR cannot contribute to
the intermittent CI failures. Both target specs now retain backend logs on
failure, and fallback retains its first failing browser trace for the next CI
run. No assertion or timeout was relaxed.

The local in-cluster bundle included a native agentctl that depends on Nix
libraries absent from its container. With the same-source static Linux helper,
both in-cluster startup and sibling restart passed. Twelve ordinary Monitor
and saved-view repetitions passed with retries disabled, alongside six workflow
repetitions and the three delayed-operation regressions. All browser runs used
one worker and two CPUs with a 4 GiB limit.

Main advanced to `7914b4110baaa68a5633eec3204233a0c9875a65`; its merge with the
published head is clean. Eleven focused attachment-storage and SQLite ACP
metadata tests passed in that synthetic merge. The PostgreSQL variant was not
selected. Main then advanced to `74427cb50c24e281e5b3fa826c7136ffed5be5db`.
That merge is also clean, and all 15 incoming workspace-cookie tests passed in
its synthetic merge. Prior focused integration checks for the earlier main
deltas remain separate evidence.

The trusted documentation coverage workflow failed before creating a job because
GitHub's shared concurrency queue was full. Its read-only local evaluator
passed against the exact PR file set. The remote queue has begun draining;
a fresh remote result is still required. This round changes test setup and
failure evidence only, so public behavior documentation and MCP screenshots do
not change. New-head CI and bounded review remain pending. No merge is authorized.

## CI remediation after `7964b698`

The exact-head waiter completed with 78 passing checks, 12 skipped checks and
11 failed checks. The E2E report records 4,065 first-attempt passes, six passes
after retry and 40 skipped tests. The six flaky cases fail the repository gate;
retry success is not passing CI evidence.

The Rename dialog exposed a selection-restoration race. Radix can select the
returning input before the deferred caret restoration runs. A first keydown
could then cancel restoration and replace the entire title. Two deterministic
regressions failed before the fix. Pending restoration now runs before keydown
or text insertion, then clears so it cannot overwrite the user's new caret.
Pointer selection, changed values and unmount cleanup keep their existing guards.

Five browser setup sequences now wait at their relevant boundaries:

- The Bitbucket contract waits for the initial agent turn and Review placement
  before opening a task-card menu that a workflow move can unmount.
- The phone persisted-history case performs its required reload before checking
  the seeded trailing message. The visibility, unloaded-prompt and alignment
  assertions remain; the shared helper's default live-update path is unchanged.
- The Kanban drag waits for the target's existing drop highlight before mouseup.
- Change workflow waits for a successful move response before asserting that
  its dialog has closed.
- MCP application-report routes drain before the next reload or fixture cleanup.
  Route errors are not ignored.

The unchanged cases passed focused local repetitions; those runs did not
reproduce the intermittent CI failures. The caret unit tests independently
reproduced the input-ordering defect. Corrected verification passed 23 focused
unit tests and 38 browser cases, with one worker, retries disabled and a
two-CPU/4 GiB limit. Browser coverage includes desktop and phone rename,
Bitbucket, Kanban, workflow change, phone history, MCP geometry at
320/393/767px, desktop MCP and coarse-pointer tablet MCP. The rebuilt web,
6 GiB typecheck, scoped lint, formatting, specification lint and catalog
validation passed. The initial full-Chromium attempt failed at browser launch
because the isolated environment lacked libraries; it is not test evidence.

The two PostgreSQL CI leaf failures occurred during Go module downloads from
`proxy.golang.org`, before the affected tests ran. Their backend aggregate also
failed. A fresh push must recheck them. Both walkthrough attempts reached the
600-second generation deadline with an empty draft and no verification stage.
That workflow comes from trusted main, so a PR-only workflow edit cannot repair
the current run. The walkthrough remains failed evidence.

Main later advanced to `5f4af538b132297a96c383ec731acd800b06f76d`; its merge with
the published head is clean. The branch still contains the explicitly tested
`34eba773` integration. Current-head CI and bounded review of the new fix delta
remain delivery gates. Exact commits, command receipts and subsequent review
results belong in the current Kandev plan. No merge is authorized.

## Conflict and CI round on 2026-10-20

The user requested conflict resolution and passing CI after main advanced to
`34eba773398ceafa6497b7dfdbbec8e6c5b433da`. The eight conflicts were independent
additions to task locale catalogs. The resolution preserves both MCP strings
and main's task-deletion strings. A three-way key comparison rejected conflicting
values and duplicate keys. Localization validation passed.

E2E run `39433180844` at head `52af0b2` exposed two failed first attempts that
passed on retry. The mobile workspace-source test measured a 43.99998474121094px
button before waiting for drawer entry to finish. It now waits for finite drawer
animations before measuring, preserving the strict 44px assertion. The mobile workflow-override
test sampled progression while the resumed initial agent still displayed
“Starting Mock”. It now waits for that session to become idle before reloading
the page. Task-move completion alone does not establish session readiness.
No product geometry, animation, timeout assertion, or visibility assertion changed.

Both unchanged cases passed three local repetitions, so the intermittent CI
failures were not reproduced locally. Both corrected cases then passed three
repetitions with one worker, retries disabled, and a two-CPU/4 GiB limit.
The MCP Connections geometry checks also passed at 320, 393 and 767px
against the merged runtime, with retries disabled.
The fresh merged web build, native runtime/fixture build, 6 GiB typecheck,
80 focused frontend tests, 160 focused lifecycle tests, specification lint and
public-document validation passed. The lifecycle selection covers MCP,
silent-restore startup/stream retry and workspace retention.

A separate task service/handler Go command exceeded its 4 GiB compilation limit
before producing results. This is failed command evidence. The retry compiled
one package at a time under a 6 GiB limit and a 2 GiB Go memory target, then
passed all 26 selected task-service and handler tests.
Exact command outcomes and the final published head belong in the current
Kandev task plan. These local results do not establish passing GitHub CI.

Public docs need no additional change for the two test-only synchronization
repairs. The merge retains main's public-document updates and the PR's MCP
documentation. Existing screenshots identify their capture commit explicitly.

## Resumed delivery

The current task is `31c5c5a9-c42d-4782-9a72-82a525ffee0b`. Its Kandev plan owns
the current verification ledger. The canonical delivery branch remains
`feature/plan-workspace-mcp-c-dcc`. The old integration alias is historical.

A separate clone isolates delivery from a broken ref in the shared repository.
Current-main conflict resolution preserves MCP selection state and the newer
initial-workspace-layout controls. It also retains durable workspace path
reconciliation alongside MCP repository identity collection.

Fresh frontend checks passed: the initial affected suite (255 tests across
14 files), the extended task-edit/merge regressions (96 tests), typecheck with
an explicit 6 GiB heap, full ESLint, localization, and the web build. The added
coverage checks MCP rejection/retry before launch, explicit empty selections,
clean/started edits, a real editor retry, and combined MCP/layout create/reset
behavior. The final editor test cleanup also passed its focused rerun.

Real PostgreSQL MCP deletion and snapshot tests passed with race detection.
The first Go run failed and is retained as diagnostic evidence. Its failures
included an incorrect streams package path and fixture assumptions about
`/bin/sleep`, `/usr/bin`, and a usable shell PATH with an empty environment.
A private Bubblewrap namespace supplies those paths without changing the host.
Six packages passed in the corrected run. Lifecycle exceeded the ten-minute
package runner limit without an assertion failure. A separate bounded partition
processed its remaining 487 declared tests. The union accounts for all 2,725
declared lifecycle tests: 2,704 passed and 21 skipped, with no missing names or
test assertion failures. Skips include opt-in container/provider checks,
macOS-only checks, release/browser/subprocess helpers, a case-insensitive
filesystem check and unavailable zsh fixtures. They are not passing coverage.
This is partitioned evidence, not a claim that the timed-out command passed.

The pruned-delivery startup fixture separately failed on both the integrated
source and exact main `e3ef99a98f8d340f2447650cb4318b00977eb44e`. Its context
expired during schema setup, before recovery. The fixture now initializes its
database before creating the unchanged three-second recovery context. Both
delivery modes pass. This is a test-budget correction, not a production change
or a claim that baseline evidence excludes every possible PR contribution.
The workspace placement/MCP repository-scope integration regression also passes.

Current Go lint identified repeated literals. The integrated source now reuses
the typed stdio constant, shares Claude's reserved server name between config
and delivery evidence, and reuses the existing non-durable storage reason.
The affected passthrough, Claude strategy and native compatibility tests passed
after these behavior-preserving changes.

The latest main update changes delivery-retention scheduling. Worker tests and
its two new repository regressions passed on SQLite and PostgreSQL. A broader
SQLite retention run reached its five-minute runner limit during schema setup.
It remains failed evidence; the focused checks do not imply that the broad run
completed or that every retention behavior was verified locally.

The current web, native runtime, delivery fixture and plugin package built
successfully. Eighteen affected browser cases passed across desktop, phone,
narrow fine-pointer and coarse-pointer tablet layouts, with one worker and
retries disabled. They include 320/393/767px Connections geometry, incremental
shell reload/cancellation, and combined MCP/layout selection and persistence.

The initial desktop run passed eight cases and failed the new 28px assertion
because it sampled the dialog during its opening animation at 27.407px. The
geometry helper now awaits existing finite animations before measurement.
No size assertion or product animation changed. The desktop and tablet geometry
cases each passed three subsequent runs. All three phone sizes passed again,
as did the combined Advanced case and a separate phone capture flow.
The Advanced capture now scrolls both settings into view before taking the image.

The isolated review session is `213febdc-c9bc-4579-a513-5dd0415997fc`.
Its bounded review of the immutable integration commit found no new blocking
issues and marked the task-edit MCP coverage finding addressed. It reviewed the
seven conflict resolutions, added coverage, deadline placement and literal reuse.
It did not audit every file in the original PR or establish runtime/CI approval.
The final test synchronization and documentation delta requires bounded closure.
No merge, auto-merge, queue admission or follow-up execution is authorized.

## Historical local evidence from the previous machine

| Check | Result |
| --- | --- |
| Full changed and affected frontend tests | 42 files, 478 tests passed |
| Full affected Go runtime | 11 packages passed, including lifecycle and agentctl API |
| Real PostgreSQL | Atomic deletion, rollback and coherent resolution passed |
| Orchestration integration | Backend app, orchestrator, task service/handlers and MCP handlers passed |
| Static checks | Go lint zero issues; frontend lint/typecheck, localization and gofmt passed |
| Documentation | Index/spec lint and coverage of 26 work orders/44 documents passed |
| Artifacts | Web, embedded assets, native runtime, delivery fixture and plugin package built |
| Desktop/tablet browsers | 23 distinct cases passed with retries disabled |
| Phone browsers | 20 distinct cases passed with retries disabled, including 320/393/767px Connections |

The affected settings suites were rerun after the final width repair: seven
desktop and five phone cases passed. The remaining browser flows use unchanged
production code. The MCP dialog keeps its explicit single-column grid. No later Go runtime
change invalidates the full runtime run. All review findings through the fifth
isolated round are addressed. The latest CI candidate additionally repairs
frontend/browser fixtures and a prompt-preview dismissal race; its exact-head
review and CI are recorded separately below.

The unchanged terminal integration fixture passes with a controlled POSIX shell;
the host's customized interactive shell is unsuitable for that fixture. Pending
MCP tests use a busy-until-cancel mock without an unnecessary post-cancel delay.
No production setting was changed to make either fixture pass. Earlier timeout,
failed and historical-preview runs are not counted as successful evidence.

Main :9998 retains its original process and health 200. The old pod-local preview
is not claimed as the final tested artifact. The managed browser runs use fresh,
disposable data and the current rebuilt runtime.

## Integration and remediation record

User scope on 2026-10-19: finish and review the existing implementation, resolve
valid findings and PR comments, pass CI, then request confirmation before merging.
No merge or auto-merge is authorized yet.

## Deferred work

Four queued children were created under the resumed task:

- Management usage/global-profile navigation and neutral delivery presentation
  (points 3/4): `2adba1ca-619e-4bfc-b705-3bbc628df4dd`.
- Session-only inherited exclusions (point 1):
  `a3588dfb-7c25-4f91-9a3d-665ce89c6605`.
- Optional assignment after registration (point 2):
  `58d6bce2-eab4-4ac6-9ac4-50a31011e1e1`.
- Built-in management and tool pruning (point 5):
  `6d1b4323-e821-4599-acea-07273a60dd78`.

All four have automatic starts disabled and no sessions. Each requires explicit
user authorization before work. Each must start from actual current main
containing the final parent merge, preserving user edits.

The previous machine's six-file management design package and Platform patch
were not supplied or recovered. The new child's plan records scope and this
gap. It does not claim to contain the archived package. These features remain
excluded from this PR.

## Integration identity

Published PR branch: `feature/plan-workspace-mcp-c-dcc`, captured remote head
`2ff981eb1ba07927e600e7f5cbc4e99b53961d05`. The local takeover branch had
rebased-equivalent older history; its two authored commits were replayed onto
that exact published head without dropping the remote's task-field/E2E fixes.
Pre-integration archive: `archive/pr3293-before-upstream-integration-20261019`.

Captured live base: `b544f15d81ca45a2bfbc8c61d695d7d818aefecd`. The current-base
merge required 62 conflict resolutions, including runtime journal/adoption and
owner wiring, coordinator/settings routes, translation catalogs and older E2E
stabilization work. Preserve the newer base behavior alongside MCP selection.
Accepted setup recording follows main's extracted passthrough-reset owner.
The main child-first E2E reset supersedes the old retry-based helper; current
Chart/provider/touch/ETag tests supersede the old boundary-specific variants.
Catalog JSON key merge had no overlapping changed-value conflicts.

## Historical independent review

User-requested separate Kandev session: `881edc17-320c-40c3-a8be-6dd4fd28cc86`,
Isolated PR review. It owns a private detached review worktree and has no
implementation or GitHub mutation authority. Parent owns remediation. Initial
snapshot: `d30fed2ba6c2c616f366e0027e1c84699e17079c` plus archived picker/mobile
repair patch (tree `03a75a9d94e42a41992223faa8ad8a5c0c0fa726`). A final integrated
head delta review is required before declaring readiness.

## Verification

Pending completion. Current merged-tree evidence:

- Focused MCP frontend suite: 11 files, 112 tests passed.
- Locale validation passed for all seven shipped languages and pseudo.
- Catalog: 390 decisions/1661 specs; all-spec lint passed.
- PR-documentation coverage: covered, 23 changed work orders/40 referenced
  documents, zero errors.
- Harness validator: 204 files passed; harness/spec-linter unit suites passed
  (19 and 36 cases). API scoped guide: 36 lines, targeted harness hook passed.
- Actual PR delta whitespace against captured main passed. Incoming main's CSV
  line endings/Markdown spacing do not represent authored changes in this PR.
- Backend tests, typecheck, lint, post-integration browser checks, final review
  and exact-head CI remain pending. Earlier preview receipts are historical,
  not proof of this newer merged runtime. Main instance :9998 is untouched.

Final disposition and exact-head results will replace pending statements here.

## Review remediation checkpoints

Round one identified nine valid findings. Real PostgreSQL deletion/snapshot tests, legacy URL import tests, editor-context regressions, argument/disabled-selection tests, required-input validation and exact-operation evidence tests reproduced failures and passed after repair. Round two reviewed immutable tree `8408383d075359fb63662174317ae2156358bdf5` against main `b544f15d81ca45a2bfbc8c61d695d7d818aefecd` and found unsupported Registry argument metadata and project-file provider reload acceptance still incomplete. Both were reproduced and repaired; expanded final validation remains pending.

Additional main integration regressions exposed missing managed-conversation admission on resume and missing visibility for helper-side policy filtering. API tests reproduced both and pass with shared admission across all four session operations. Startup acceptance/publication uses the existing generation lease. No executable placeholders are delivered.

Current validation includes 117 frontend tests across 15 files; frontend typecheck, full lint and localized catalogs; a web build and complete runtime/plugin-fixture build; 48 public documentation pages; 390 decisions/1662 specs; and PR coverage for 26 work orders/44 documents. New source changes invalidate earlier artifact/final-head claims and require the recorded final reruns. Two bounded Go lint attempts timed out while loading packages; neither counts as passing. A third uses trimpath cache identity to match the runtime build.

The installed GitHub CLI 2.45 cannot upload screenshots. A temporary GitHub CLI 2.102.0 was downloaded from the official release and verified against its published SHA-256 digest; native attachment support is confirmed. Publication and final-viewport recapture remain pending.

Round-three delta review closed both round-two findings and identified a P1 lock-order inversion in idle acceptance. A real application regression gated the ACP load, admitted a completion callback and queued a startup writer; it reproduced the deadlock (RED) and passed after the startup lease moved before the prompt lock (GREEN). Ordinary ACP application, project-provider deferral, exact setup acceptance and CLI operation regressions passed in the same focused run. A narrow follow-up audit checks the other generation-lease placements before final review.

The third lint attempt also exceeded context loading. Compiler response files identified internal/orchestrator and internal/task/service as the large jobs, consuming roughly 3–4 GiB each under a 3 GiB soft limit. Their type exports are being warmed sequentially with a bounded 5 GiB limit and 10-minute command limit; lint will retain its normal five-minute budget. No failed or timed-out lint run is counted as passing.


Round four verified both lock-order repairs and found that detached idle updates lacked a host deadline. Controlled unanswered reconnect and stalled restoration tests reproduced retained leases and an expired-context failure write. Both now pass with a two-minute operation deadline and a separate five-second persistence budget, preserving old acceptance and releasing a real queued startup writer. Focused lifecycle, PostgreSQL store, Registry and API checks passed after these changes. Five lint findings from the first successfully loaded analysis were repaired; final lint remains pending.


Round-five isolated review checked tree `29c0f8098f1e5b6b009c22b8d9e85f0d6966367e` against round four, confirmed the host deadline and bounded failure persistence, and found no further concrete issue. No unresolved independent-review finding remains in its inspected paths. Final committed-head identity and integration/CI gates remain separate. Current focused frontend verification passed 107 tests in 13 files. Remote main and the published PR head still match the captured b544/2ff references; the GitHub review-thread helper returned successfully with no actionable entries.


Final local static/runtime checkpoint: changed-package Go lint passed with zero issues after source exports were warmed with an 8 GiB per-process memory target; the normal five-minute lint timeout was retained. Full frontend lint passed after extracting a duplicate style string. Current typecheck, localization, web production build and embedded-asset sync passed. All 36 changed frontend test files passed (416 tests). The full affected Go run passed all 11 packages, including lifecycle (224.197s), API (92.005s), Registry, settings/handlers/store with real PostgreSQL, runtime client/launcher and streams. The earlier cold-cache checkout/startup failures did not recur. Documentation index/spec lint/26-work-order coverage and existing-file gofmt check passed. Native rebuild, integrated browser cases, commit hooks, exact committed-head review and GitHub CI remain pending.


The remaining scoped integration run passed backendapp, orchestrator/executor/message queue, task service and MCP handlers. The unchanged Cursor authentication terminal fixture initially inherited the host customized zsh and failed to exit; rerunning task handlers with `SHELL=/bin/sh` and shell startup variables unset passed (0.269s). This is a controlled test environment, not a production change or a skipped assertion. Main :9998 retained the same process and returned health 200. The native/runtime/plugin-fixture rebuild is underway before managed browser execution.


The rebuilt native runtime, delivery fixture and plugin package passed. Desktop managed E2E initially passed 21/22 cases and exposed a fixture mismatch: `/e2e:cancel-hold` now delays cancellation eight seconds while the default join budget is three seconds, invoking recovery and closing a just-opened dialog. Pending-change tests now keep the turn busy until cancellation but use a zero mock post-cancel delay, wait for observable editable idle, then inspect real MCP acceptance. The repaired desktop case passed with retries disabled (one case, 37.0s runner). Other 21 desktop cases passed against the same source. Phone verification is running. No production behavior was changed for this fixture repair.


Phone E2E passed 19/20 cases, including pending application, 320/393/767px Connections geometry, provenance, settings inputs and recovery. The long unsupported-package case reproduced a real internal overflow: the dialog's implicit grid track expanded to min-content width. An explicit `grid-cols-1` gives that track a zero minimum rather than clipping content. The same long-name regression now shares a helper across desktop and phone settings tests. Current web/native rebuild and scoped lint passed; both complete settings suites are being rerun against this fix. The merge brings 175 staged Go packages through normal hooks, so their test export cache was warmed successfully without running tests or changing the five-minute lint budget.


Published-head CI identified the strict frontend fixture integration gap recorded
in task 11. The failures reproduced (five files/36 failures), then passed with
known-owner MCP read responses, complete mocked form state and an explicit
secrets/MCP/coordinators ordering assertion. Combined affected tests passed
41 files/454 cases; final focused checks passed five files/38 cases, scoped lint
and typecheck. No production/UI code changed. Current-head CI still requires a
fresh passing result after the next normal remediation commit and push.


## Published-head CI remediation

The first complete a57 run ended with 81 passed, 11 skipped and 11 failed
checks. Backend tests passed. Frontend fixture failures are repaired as above.
E2E artifact audit covered all 118 blob reports with no parse errors: 3,998
first-attempt passes, nine flaky verdicts, four failed cases and 40 skipped.
These failures are not counted as passing evidence. PR review evidence was
complete with no unresolved, hidden or actionable threads.

Browser fixture repairs restore current-main worktree-path capture and the
preview screenshot-helper import. The session-deletion test uses the same
captured session ID for its action and assertion. Local path regression was
RED then three cases passed; preview and the full session suite passed under
two CPUs/4 GiB with retries disabled. A changed-E2E identifier audit exposed
only the missing preview helper after path repair.

The prompt-history failure reproduced under the same limits. A component
regression showed a delayed hover timer reopening a preview after Escape.
Explicit dismissal now suppresses delayed reopen until a new trigger
interaction; click/keyboard toggles and later hover remain supported. This
changes prompt-preview interaction only, with no MCP layout or copy change.
The original click/Escape browser assertion is retained. After rebuilding,
that plugin scenario and the existing native hover scenario both passed
under the same pressure limits (two tests, 26.2 seconds). The final affected
frontend run passed 42 files/478 tests, including all 24 prompt component
cases; frontend typecheck and scoped lint passed. Final commit review and
current-head CI remain gates.

Other first-run flaky cases were investigated without speculative product
changes: workflow/Azure focused cases and their ordered pressure replays
passed (37 and 12 cases); dynamic fallback desktop passed under pressure;
the combined phone autoscroll, fallback, plan-comment and workspace-clone
suites passed 19 cases. All used retries zero and one worker. Independent
source audits found no concrete new MCP transaction or dynamic-startup lock
defect; the initial transient causes remain unknown. Fresh complete CI on
the remediation commit is still required before merge confirmation.


## Second-head browser fixture synchronization

On da26, CI found two further fixture synchronization failures outside the
feature source. The workflow move-preview helper matched only a task URL and
read a browser response body after UI waits. A controlled competing response
for another destination reproduced a wrong `reuse_current` result where
`create_new` was expected. The helper now matches the destination step, consumes
JSON immediately, observes transport and UI together, and asserts task/step
identity. No body error is swallowed, retried or replaced by a separate read.
All existing move, model, recipient and final-session assertions remain. The
complete spec passed three cases; its ordered desktop CI partition replay
passed 35 cases under two CPUs/4 GiB, one worker and retries disabled.

The Threads deep-link test asserted a chat before its task's session list had
loaded. An eight-second initial GET stimulus reproduced the same loading-state
failure. Reusing the existing client session-settlement helper fixes the test
without changing its rendered assertion timeout or the product loader. The
complete two-case layout suite passes with that real slow read and all stream
bounds, row geometry, unmount/remount and navigation assertions retained.
The exact original CI request timings and CDP resource failure are not proven
from the saved snapshots; these deterministic counterexamples establish the
fixture weaknesses repaired here.

This delta changes only browser tests and this record. Product source,
42-file/478-test unit evidence and published MCP/prompt captures remain
unchanged. Final immutable delta review, normal hooks and fresh committed-head
CI remain required. The optional automated walkthrough separately timed out
twice at its fixed generation deadline; no workflow or check policy was changed.


The repeated mobile freeze case had two observation gaps: an immediate DOM
scroll assignment captured an unsettled baseline, and the substring marker
could match the outgoing prompt before the assistant appended its reply. Its
fixture now completes a native touch gesture, verifies trusted start/move/end,
waits for stable position and geometry, and checks the exact assistant marker.
It also checks preservation immediately after disabling. The final ten-pixel
bounds and two-second budget, real composer/send path and history-depth
preconditions remain. No transcript implementation or tolerance changed.
The old wheel-only helper failed the trusted-touch setup contract; five focused
native-gesture repeats passed under the same resource limits. That setup RED
is not a reproduction or claimed root cause of the intermittent CI offset.
Full phone-suite validation and final committed-head CI remain gates.

The complete da26 artifact audit reconciles 118 blob reports: 4,012 first-attempt
passes, three retry passes, zero final failed cases and 40 skipped. Those three
flaky verdicts are exactly the workflow, Threads and mobile fixtures described
above; fail-on-flaky correctly kept the required E2E gate red. No parse errors
or additional failed cases were found. All six container shards passed.


Final fixture review found and closed a history-precondition gap: the settled
sample now requires both a nonzero history offset and more than 100 pixels
remaining below the viewport. A local controlled schedule that holds settlement
at the bottom was accepted before this check (RED) and rejected after it
(GREEN). The probe changes only the disposable test clone and is not committed.
It does not reproduce the original CI offset. Three edited browser specs pass
all TypeScript diagnostics; the 60-file changed-E2E unresolved-name/import audit
and scoped lint also pass. Existing product screenshots remain representative.
