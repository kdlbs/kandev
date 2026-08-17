# Merge queue

The `main` ruleset enabled the merge queue and six required checks on
2026-08-16. This document records the workflow preparation, active rules, and
the Stable release exception.

## Why

Before activation, `main`'s ruleset (id `13341245`) had `deletion`,
`non_fast_forward`, `required_linear_history`, and `pull_request` rules. It had
no `required_status_checks` rule.

That is how #2681 (`38c94e7d0`) landed red. Its E2E checks were fully green, but
they had run 23 hours earlier against a merge ref that predated the retry loop
added by #2569 (`15788438b`). Neither change is wrong on its own; together they
put a roughly 4.5-second floor under content search, just over the 5-second
default assertion in the test #2681 added. Both pull requests were green, and
their merge was not.

Required status checks alone would not have caught it -- #2681's checks were
green. A merge queue would: it builds the actual merge result and runs the
checks against that, at merge time, in queue order.

## What changed in the workflows

### The `merge_group` trigger

Six workflows gained `merge_group: types: [checks_requested]`:

| Workflow                 | Gate job       | Check name to require                   |
| ------------------------ | -------------- | --------------------------------------- |
| `e2e-tests.yml`          | `e2e-gate`     | `E2E Tests Passed`                      |
| `backend-tests.yml`      | `test`         | `Run Backend Tests`                     |
| `frontend-tests.yml`     | `frontend-gate`| `Frontend Tests Passed`                 |
| `architecture-lint.yml`  | `lint`         | `Architecture boundaries do not regress`|
| `lint-action-pinning.yml`| `lint`         | `All action refs are SHA-pinned`        |
| `lint-harness-files.yml` | `lint`         | `Harness files pass lint`               |

Deliberately excluded, because they cannot report in a queue and would stall it
forever, or because their redness is not a property of the merge:

- `pr-title.yml`, `claude-code-review.yml`, `opencode-code-review.yml`,
  `preview-env.yml`, `notify-docs.yml` -- all `pull_request` or
  `pull_request_target` scoped. There is no pull request in a merge group, so
  they never run and never report.
- `cargo-audit.yml` -- also runs on a weekly `schedule`, by design: a newly
  published RUSTSEC advisory turns it red with no change to the code. In a queue
  that would block every merge in the repository until somebody bumped a crate.
  It stays a pull-request and scheduled check.
- `ci-base-image.yml`, `universal-rebuild.yml` -- these publish images rather
  than test a merge.
- `release.yml` and the plugin registry workflows -- out of scope.

### The path-filter problem, and the approach chosen

Most of these workflows were heavily path-filtered (`apps/web/**`, `!**/*.md`,
and so on). That is a problem for a queue in two independent ways.

1. A workflow skipped by a trigger-level `paths:` filter reports **no
   conclusion at all**. GitHub's documentation is explicit that "checks
   associated with that workflow will remain in a 'Pending' state". A required
   check that stays pending blocks a pull request from being added to the queue,
   and blocks a merge group from ever merging.
2. `merge_group` does not support `paths:` in the first place. It takes `types:`
   and `branches:` only. So the filters could not simply be mirrored onto the
   queue side even if we wanted them there.

Two approaches were available: mirror each filter onto an inverse trigger with a
"skipped reports success" shim job, or make the gating jobs always run and
short-circuit internally.

**The shim was rejected, and the internal short-circuit implemented.** The
deciding factor is that `paths` with `!` exclusions has no exact inverse. Take
frontend-tests' old filter, `apps/web/**` followed by `!**/*.md`. A pull request
that touches only `apps/web/README.md` is skipped by that filter, and is *also*
skipped by the mirrored `paths-ignore` list, because every changed file appears
in it. Both halves skip, no conclusion is ever reported, and the queue waits
forever -- the exact failure the shim exists to prevent, reintroduced by the
shim. Making the inversion correct would mean hand-deriving a second pattern
list per workflow and keeping the two in sync by eye. Worse, the shim needs a
job whose *name* matches the real check, which for `e2e-tests.yml` would mean
reproducing fourteen shard names and six container shard names.

So: the trigger-level `paths:` filters are gone, and each expensive workflow
grew a cheap `changes` job that applies the same list one level down.

```
changes ──> static_checks ─┐
        └─> test_shards ───┼──> test          <- the required check, if: always()
        └─> ...          ──┘
```

`.github/scripts/changed-paths.py` implements GitHub's filter-pattern semantics
(`*`, `**`, `?`, leading `!`, last match wins) and is unit-tested in
`changed-paths_test.py`, which also asserts the pattern lists still live in the
workflows and still select what they used to. It rejects `+`, `[...]`, and
`{...}` rather than quietly matching them as literals.

Two properties of this shape matter:

- **The gate job always runs.** It is `if: always()` and treats a `skipped`
  dependency as a pass and a `cancelled` one as a failure. This is on purpose:
  it means the required check never depends on how GitHub reports a
  conditionally-skipped job, only on a conclusion the workflow computes itself.
- **The gate covers every job in its workflow.** A queue gates on check names an
  admin picks by hand, so a job outside the gate's `needs` is a job whose
  failure would never block a merge, silently. `backend-tests.yml`'s
  `postgres-boot` and `test-windows` were outside its aggregate before this
  change and are now inside it.

Two workflows -- `lint-action-pinning.yml` and `lint-harness-files.yml` -- have
no `changes` job. They are a checkout plus a handful of short linters, so they
simply always run: cheaper than the job that would decide whether to run them,
and it closes the gap where a subject outside their hand-maintained lists
changed and nothing checked it. Their contract tests were updated to assert the
absence of a filter rather than the presence of specific entries.

### PR context in a merge group

A `merge_group` run has no `github.event.pull_request` and no
`github.event.before`. Every expression that read one has been made explicit
about the event it is on, and given the merge-group equivalent,
`github.event.merge_group.base_sha` -- the commit the queue branch was cut from,
which is the merge group's base:

- `architecture-lint.yml` -- the two mutually exclusive `pull_request` / `push`
  lint steps were collapsed into one, behind a baseline-resolving step. Under
  the old shape a merge-group run would have executed the linter's self-tests
  and then linted nothing at all, reporting green having compared the branch
  against no baseline.
- `backend-tests.yml` -- `Resolve base SHA`.
- `frontend-tests.yml` -- `Check formatting`, the i18n new-code ratchet, and the
  E2E sleep ratchet.
- `e2e-tests.yml` -- the GHCR login guard reads
  `github.event_name != 'pull_request' || <fork check>`, which resolves to the
  first operand on a merge group. That is correct: the queue branch lives in
  this repository and its token is a base-repository token.

Concurrency groups were checked too. None of the six keyed on a PR number
(`preview-env.yml` does, and it is not a gate). All six now set
`cancel-in-progress: ${{ github.event_name != 'merge_group' }}`, because a queue
reads a cancelled check as a failure and ejects the pull request.

## Active ruleset configuration

The `main` ruleset contains a `merge_queue` rule and requires the six gate names
in the table above. It does not require per-shard names.

Do not require `lint` from `pr-title.yml`, `Validate public docs`, review checks,
or deployment checks. These checks do not run in every merge group.

### Stable release exception

The Stable release workflow creates a mechanical release PR with `GITHUB_TOKEN`.
GitHub holds the PR workflows for approval, but maintainers do not require CI
for this generated commit.

The protected `release` environment contains `RELEASE_PR_BYPASS_TOKEN`. This
fine-grained personal access token belongs to an organization administrator and
selects only `kdlbs/kandev` with `contents: write` permission.

The workflow exposes this token only to `gh pr merge --admin`. It also binds
the merge to the expected PR head. `GITHUB_TOKEN` performs all other PR work.

The administrator merge bypasses the queue and required checks for the release
PR only. Ordinary PRs still use the queue and all six required checks.

### Caveat: the ruleset can still be bypassed

The `main` ruleset lists one bypass actor:

```json
{ "actor_type": "OrganizationAdmin", "bypass_mode": "always" }
```

`bypass_mode: always` means an organization admin can merge past whatever is
configured here, queue included, without a review request or a temporary
override. Configuring required checks constrains everybody else; it does not
constrain an org admin. If the point of enabling the queue is that nothing red
reaches `main`, that entry needs to change as well -- to `pull_request` mode, or
removed -- and that is a separate decision about who is trusted to break glass.

## What is verified, and what is not

Verified locally on this branch:

- `actionlint` v1.7.12 is clean on every file touched here. Its one repository
  finding is pre-existing and in `release.yml` (`queue: max` under
  `concurrency`, a key it does not know).
- `zizmor` v1.29.0, run over the six workflows before and after: one finding
  removed (a `template-injection` error in `frontend-tests.yml`, from moving
  `github.base_ref` out of an inline expression into `env:`), none added.
- Every `.github/scripts/*_test.py` contract test passes, as does
  `lint-action-pinning.py` over all 18 workflows.
- `changed-paths_test.py` passes, and fails when the guarantees are broken:
  restoring a `paths:` filter to a gating trigger, deleting a `merge_group`
  trigger, and reordering `!**/*.md` above the includes were each confirmed to
  fail it.

Not verified, and not verifiable until a queue is switched on:

- **No `merge_group` event has ever fired against these workflows.** Everything
  about the merge-group path -- that `github.event.merge_group.base_sha` is
  populated as expected, that the `changes` job resolves a usable base from it,
  that the gate jobs report under the names in the table -- is read from
  GitHub's documented payload, not observed.
- Whether GitHub blocks a pull request from *entering* the queue on a check that
  reports nothing. The design assumes it does, which is why the filters moved;
  if it turns out not to, the change is still correct, just more cautious than
  it needed to be.
- The cost. Every pull request now starts six workflows instead of however many
  its paths selected, and each merge group runs the full E2E suite. The
  `changes` jobs keep the expensive work filtered, but the floor per pull
  request is higher than it was.

The cheapest way to close the first gap is to enable the queue on a low-traffic
day and watch the first merge group's check names against the table above.
