---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-001
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-002
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-003
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-004
---

# Configured Push Target System Design

## Purpose and boundaries

The workspace system owns workspace Git state and the contract that publishes a
task branch to a remote. This design extends the existing workspace push and
push-preflight contracts with an explicit push target and an expected branch.

Adjacent contracts this design uses but does not own:

- Contribution routing (remote contribution, contribution destination) and its
  force-push refusal. This design refuses rather than overriding it.
- Empty-remote first publication, which stays attached to `origin`.
- The task runtime Git credential route.
- The agent runtime transport that carries a push request from the orchestrator
  into the workspace.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-001` | [Data and contracts](#data-and-contracts), [Push-target resolution](#push-target-resolution) |
| `REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-002` | [Control flow](#control-flow) |
| `REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-003` | [Preflight](#preflight) |
| `REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-004` | [Refusal ordering](#refusal-ordering), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- **Workspace Git operator (agentctl, per checkout).** Owns the operation lock,
  push-target resolution, the expected-branch guard, and Git command
  construction. All new decisions live here so that no caller can reach the
  destination selection by a different route.
- **Workspace Git HTTP surface (agentctl).** Binds the two new optional request
  fields for both the push and push-preflight endpoints and passes them through
  unchanged. It performs no destination logic of its own.
- **Agent runtime Git client (backend).** Carries the two new optional fields on
  the push and push-preflight calls. Existing callers that pass neither keep
  their current signatures' behavior.
- **Workspace Git action handler (backend).** Accepts the two new optional
  fields on the existing push action and forwards them. The web client keeps
  sending neither.
- **Git argument validator (agentctl).** The existing defense-in-depth allowlist
  that inspects every argument before a Git process starts. This design keeps a
  remote URL off the command line entirely so the allowlist does not have to be
  widened.

## Data and contracts

Two optional inputs are added to the push and push-preflight request bodies, and
to the equivalent backend action payload and client calls. Both default to
absent, and absent reproduces today's behavior exactly.

- `remote`: the explicit push target, as a configured remote name or a remote
  URL.
- `expected_branch`: the branch the caller believes it is publishing.

Both inputs are trimmed of leading and trailing whitespace before any other
check, and a value that is empty after that trim is absent. For the expected
branch this runs before the strict branch allowlist, so a whitespace-only value
is an omitted input rather than a `push_branch_invalid` refusal. The trim is
normalization, not a refusal: request-body parsing necessarily precedes it, and
it precedes every check in the refusal ordering below that reads either value.

The result gains five optional output fields. Each is omitted when its rule
below does not apply, so a consumer that reads none of them sees the shape it
sees today.

- `pushed_remote`: the resolved remote name. Populated on a successful push that
  carried an explicit push target, and on a successful preflight to which
  contribution routing did not apply.
- `pushed_branch`: the destination branch name. Same rule as `pushed_remote`.
- `expected_branch`: the expected branch the request supplied. Populated on a
  `push_branch_mismatch` or `push_branch_mismatch_after_baseline` refusal.
- `current_branch`: the branch the checkout was actually on. Populated on the
  same two refusals, and reported as empty when `HEAD` is detached rather than as
  the literal `HEAD`.
- `baseline_published`: true when empty-remote first publication published the
  baseline during this request. Populated only on
  `push_branch_mismatch_after_baseline`, which is precisely the second-verification
  refusal that followed such a publication. A successful push never sets it, even when first
  publication ran, because doing so would change the result shape on the path
  that names no push target.

The asymmetry between push and preflight on the first two fields is deliberate.
The push path omits them without an explicit target so that today's only
frontend consumer observes an unchanged result; a preflight on that same path
has no consumer that reads them at all, so it reports what it validated, which
is the whole point of asking it. A preflight under contribution routing reports
neither, because `## Out of scope` promises that path's behavior is unchanged
and these fields are `omitempty`: populating them would add keys to a response
shape this capability undertook not to touch.

Stable error codes, carried on the existing result error-code field:

| Code | Meaning |
| --- | --- |
| `push_remote_not_found` | Name form; no configured remote carries that name. |
| `push_remote_url_unmatched` | URL form; no configured remote has a single-entry effective push URL set equal to the supplied value. |
| `push_remote_fanout` | URL form; a remote carries that URL but also others, so publishing through it would reach destinations the caller did not name. |
| `push_remote_config_unreadable` | The checkout's remote configuration could not be read while resolving an explicit push target. |
| `push_remote_contribution_conflict` | Explicit push target supplied while contribution routing is configured. |
| `push_remote_upstream_unsupported` | Explicit push target supplied together with set-upstream. |
| `push_branch_invalid` | Expected branch fails the ref allowlist. |
| `push_branch_mismatch` | Expected branch is not the current branch, including detached `HEAD`. Side-effect-free at the first verification; at the second it may follow empty-remote preparation that read the remote and retired the local marker without publishing. |
| `push_branch_mismatch_after_baseline` | Same mismatch at the second verification, after first publication published the baseline in this request. |
| `push_branch_detached` | Explicit push target with no expected branch, and `HEAD` is detached. |
| `push_no_remote_configured` | Preflight with no explicit target, no contribution routing, and no `origin`. |

### Push-target resolution

Resolution is a pure function of the request value and the checkout's remote
configuration, and never mutates configuration.

1. Apply the shared trim above. An empty result means absent.
2. If the value satisfies the existing branch-name allowlist, treat it as a
   remote name and require that the checkout's configured remote list contains
   it.
3. Otherwise treat it as a URL. Read each configured remote's effective push URL
   set: its push URLs when it has one or more, and otherwise the single entry
   holding its fetch URL. Git allows a remote to carry several push URLs and
   publishes to every one of them, so a remote matches only when its set holds
   exactly one entry and that entry equals the supplied value under exact string
   comparison, both sides trimmed. A remote that carries the supplied URL
   alongside others is refused with `push_remote_fanout` rather than matched,
   because resolving to it would publish to destinations the caller never named;
   a caller that wants that fan-out names the remote instead, which is the
   difference between asking for a destination and asking for a remote. A remote
   with an empty set never matches. Several single-entry remotes can still match,
   because several can legitimately carry the same URL; select the matching name
   that sorts first by byte order, which is well defined because all such matches
   address the same destination.
4. The name/URL discriminator is deliberately the permissive ref-syntax
   allowlist rather than the strict branch allowlist an expected branch must
   pass. A permissive reading cannot reach a destination on its own, because a
   value read as a name must still match an already-configured remote before
   anything is published; the expected branch has no such backstop, which is why
   it is held to the stricter rule. The strict rule is also what keeps `HEAD`
   out: the abbreviated-name read returns that literal for a detached checkout,
   so an expected branch of `HEAD` would otherwise compare equal to it and pass a
   guard whose whole purpose is to fail there.
5. Resolution yields a remote **name**. Only that name reaches a Git command
   line. A caller-supplied URL is never passed to Git, which is what keeps the
   existing argument allowlist unchanged and keeps credentials out of process
   arguments.

## Control flow

A push request runs entirely inside the existing per-checkout operation lock.

1. Acquire the lock. A busy checkout returns the existing
   operation-in-progress signal.
2. Evaluate every refusal in the fixed order below, including the first
   expected-branch verification. This whole step runs before any remote is
   contacted and before any local ref changes, so a refused request is a no-op.
3. When the resolved remote is `origin` and no contribution routing applies, run
   empty-remote first publication as it runs today. Skip it for any other
   resolved remote.
4. Complete every remaining Git read needed to build the push command. On the
   path that names no explicit push target this includes the upstream-tracking
   read that decides set-upstream behavior; the explicit-target path needs no
   such read, because that flag combination is already refused at step 2. Then
   re-read the current branch, reading `HEAD` as a symbolic ref so that a
   detached checkout is distinguishable from a branch literally named `HEAD`.
   That branch read is the last Git read before the push.
5. When an expected branch was supplied, compare it again to the current branch
   and refuse on mismatch. A detached `HEAD` is a mismatch because it has no
   current branch. The refusal carries `push_branch_mismatch_after_baseline`
   when step 3 published a baseline in this request, and `push_branch_mismatch`
   otherwise.
6. Issue the push. No Kandev-issued Git command runs between the branch read in
   step 4 and the push in step 6.

Verifying twice is what lets both guarantees hold at once. The step 2
verification keeps every refusal side-effect-free even though first publication
sits between the refusal set and the push. The step 5 verification gives the
ordering guarantee the caller actually needs: nothing Kandev issues can move
`HEAD` between the check and the push. The residual window is bounded by the
push itself, because an agent shell can still move `HEAD` outside the lock;
closing that needs an exact-OID lease, which the requirement places out of
scope. In the rare case where step 5 refuses after step 3 already published a
baseline, the result reports the completed baseline publication and leaves the
task branch unpublished, which is the existing partial first-publication
contract rather than a new failure mode. A step 5 refusal after a step 3 that
published nothing is a plain `push_branch_mismatch`. It is still not a strict
no-op: when a first-publication marker is present but the baseline is already on
the remote, step 3 reads `origin`'s advertised refs and retires that local
marker. Both are idempotent and neither publishes the task branch, which is why
this case needs no distinct code — but it is why the no-op guarantee is stated
for the step 2 refusal list only.

Refspec selection:

- No explicit push target: unchanged on every existing path. An expected branch
  acts only as a precondition and does not rewrite the refspec.
- Explicit push target with an expected branch: `HEAD:refs/heads/<expected>`.
- Explicit push target without an expected branch: `HEAD:refs/heads/<current>`,
  with a detached `HEAD` refused first.

The explicit-target path never appends the set-upstream flag and never reads the
upstream ref, because the flag combination is refused before the push.

### Refusal ordering

Fixed, so that a request with several problems always reports the same code:

1. Malformed request body.
2. Existing contribution binding validation failure.
3. `push_remote_contribution_conflict`.
4. `push_remote_upstream_unsupported`.
5. `push_branch_invalid`.
6. `push_remote_config_unreadable` / `push_remote_not_found` /
   `push_remote_url_unmatched` / `push_remote_fanout`. These four are mutually
   exclusive by construction, so their relative order is never observable.
7. `push_no_remote_configured`. Preflight only: a push with no explicit target
   and no `origin` fails at the push itself rather than at resolution. Ordered
   ahead of the branch guards so a checkout with nowhere to publish reports that,
   rather than reporting a stale branch the caller cannot act on yet.
8. `push_branch_detached`.
9. `push_branch_mismatch` raised at the first verification.

The second-verification refusal is deliberately absent from this list. It is
produced at step 5, which runs after first publication, and it is the only
refusal in this capability that can follow a completed baseline publication or a
retired first-publication marker. It carries
`push_branch_mismatch_after_baseline` when a baseline was published in this
request and `push_branch_mismatch` otherwise. Everything in the list above is
evaluated at step 2 and is therefore a no-op. Once the second-verification
refusal is set aside, the list is exhaustive for this capability: every other
refusal it introduces appears in it.

Cheap request-shape checks precede configuration reads, and configuration reads
precede the checkout read that resolves `HEAD`, so a rejected request performs
the least work. The whole list is evaluated at control-flow step 2, before
empty-remote first publication, which is why a refusal drawn from it never leaves
a partially-published remote behind.

### Preflight

Preflight reuses the same resolution, the same guard, and the same refusal
ordering, then issues a dry-run push for the refspec the equivalent push would
use. Two behaviors change relative to today:

- The path that previously reported success without contacting a remote now
  dry-runs against `origin` for the current branch.
- Every successful preflight to which contribution routing did not apply, with
  or without an explicit push target, reports the remote name and branch it
  validated. The push path's rule of omitting those fields without an explicit
  target exists to protect an existing consumer; a preflight on that path has
  none, so withholding what it checked would only make the answer less useful. A
  preflight under contribution routing reports neither and keeps the result shape
  it has today.
- A checkout with no `origin` and no contribution routing now reports
  `push_no_remote_configured` instead of success.

Preflight dry-runs the destination-branch refspec only. On the `origin` path an
actual push may first publish the empty-remote baseline, a different refspec that
preflight does not exercise, so preflight can still report success where that
baseline publication would fail. The requirement's `## Out of scope` names this
limit; REQ-003 removes preflight's blanket success for the destination branch and
does not extend it to the baseline.

Preflight verifies the expected branch once rather than twice, and the
requirement scopes the two-point rule to the push operation for that reason. The
second verification exists on the push path only because first publication sits
between the refusal set and the push; preflight runs no first publication and
mutates nothing, so there is no window for the second read to close and
`push_branch_mismatch_after_baseline` is unreachable from preflight. A dry-run is
a push command, so this is a deliberate exception stated in both documents rather
than an oversight.

The preflight paths this capability introduces or changes apply the same
credential redaction to Git's output that empty-remote first publication already
applies. The existing contribution preflight paths are left exactly as they are,
which is what `## Out of scope` promises.

These changes are reachable only through the preflight endpoint and the startup
remote-contribution preflight, and the startup caller runs only when a
contribution binding exists, so none of them alters an existing live path.

## Failure and recovery

- Every refusal in the ordered list is a no-op: no remote contacted, no local
  ref, upstream setting, or working tree changed. A caller can retry after
  correcting its inputs. The single exception is
  `push_branch_mismatch_after_baseline`, which by construction follows a
  completed baseline publication on `origin`; its result sets
  `baseline_published`, so a caller can see that a retry does not begin from the
  state the first attempt began from.
- A remote that rejects a non-force push is reported as a failure with the
  remote's output. Kandev does not retry it and does not escalate to a force
  push.
- Repeating an identical successful push while `HEAD` and the destination branch
  are unchanged succeeds and changes nothing, which is what makes the operation
  safe for a retrying caller.
- Concurrency is handled by the existing single per-checkout lock. Requests are
  rejected rather than queued, so a caller never waits on a lock it cannot see.
- Empty-remote failure codes keep their current meaning and keep applying only
  to the `origin` path.

## Persistence

None. Both inputs are per-request and are not stored. No schema, migration, or
retention change. Restart behavior is unchanged because nothing new survives a
request.

## Security

- The remote URL form is resolved against already-configured remotes and is
  never placed on a Git command line, so the existing command-injection
  allowlist stays as narrow as it is today.
- The remote name form is validated against the existing ref allowlist and must
  name an already-configured remote, so a request cannot introduce a new
  destination or a flag-shaped argument.
- Kandev-composed output never carries a remote URL: the result fields above,
  the error messages Kandev writes, and its log records identify a push target by
  its resolved name only.
- Git's own output is a separate thing, and the honest description of today is
  not that it is already URL-free. The workspace Git operator redacts the output
  of every command whose subcommand is `push`, the preflight dry-run included,
  through the same credential redaction empty-remote first publication uses. So
  the credential redaction this capability requires is already in force on both
  paths it adds, by construction rather than by a new call, and no path's output
  handling changes. That redaction removes embedded credentials and known token
  values, not URLs, so Git's `To <url>` line ships today and continues to. This
  design does not change that, which is what keeps the path naming no explicit
  push target byte-identical.
- Credentials continue to come from the task runtime Git credential route. Read
  access alone does not authorize publication.

## Observability

The existing push completion log gains the resolved remote name, the destination
branch, and whether an expected branch was supplied and matched. Each refusal is
logged once at its error code. No new metric is introduced: the failure modes are
per-request and are already visible to the caller in the result.

## Prior art

- **Kandev knowledge vault (`wiki-query`).** Searched for: nothing; the leg did
  not run. The skill is not installed on this runner (`~/.claude/skills` has no
  `wiki-*` entry), there is no `~/.obsidian-wiki` directory or config symlink,
  and `OBSIDIAN_VAULT_PATH` is unset, so neither a QMD collection nor a `grep`
  fallback resolved. Treat this as missing input, not an empty result.
- **`saas-kb` MCP (`search_fsm_docs`, `category: "ai_sdlc"`).** Searched for:
  nothing; the leg did not run. The server is not configured for this session,
  in which `kandev` is the only MCP server.
- **In-repository prior reasoning.** Read directly:
  [Empty Remote Repositories](empty-remote-repositories.md), which owns first
  publication on the `origin` path, and
  [ADR 2026-08-12 task-bound fork destinations](../../../decisions/2026-08-12-task-bound-fork-destinations.md),
  which owns contribution-destination routing. This capability departs from
  neither: it refuses rather than overrides when contribution routing applies,
  and leaves first publication attached to `origin`.

## Related decisions

- [Task-bound fork destinations](../../../decisions/2026-08-12-task-bound-fork-destinations.md)
