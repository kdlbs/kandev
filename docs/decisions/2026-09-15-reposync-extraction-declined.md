# ADR-2026-09-15-reposync-extraction-declined: Keep workflow sync and Office config sync as separate packages

**Status:** accepted
**Date:** 2026-09-15
**Area:** backend

## Context

`internal/workflowsync` syncs workflow definitions from GitHub or GitLab into a
workspace. `internal/office/configsync`, merged in PR #3304 on 2026-09-03, syncs
Office agents, projects, skills, and routines from the same two providers. The
two packages were expected to share an extracted `internal/reposync` library.

That extraction was proposed before the second domain existed. Round 4 of the
Office config sync spec review found that four of its eleven findings existed
only because the design required extracting the shared library from the shipped
workflow sync package while promising workflow sync stayed byte-identical. Each
was a collision between Office's contract and a shipped workflow sync quirk,
forced by physical code sharing rather than by either domain's requirements.

The decision then was to build `internal/office/configsync` standalone, reusing
workflow sync's pattern rather than its package, and to revisit the extraction
once the second domain was real rather than predicted. The system design set the
condition for this revisit explicitly, in wording it no longer carries and this
ADR now preserves: "'not worth extracting' is only a defensible verdict once both
halves exist to be compared." Both halves now exist. This ADR is that revisit.

### The four round-4 predictions, all now shipped

Verified against `origin/main` at `afeebea2d`.

| Concern | `internal/workflowsync` | `internal/office/configsync` |
| --- | --- | --- |
| Unchanged verdict | `service.go:238` includes `&& len(warnings) == 0`, so warnings flip the verdict | `reconcile_run.go:214` omits the warnings term |
| Provider default | `models.go:89` defaults an empty provider to GitHub | `models.go:145` rejects an empty provider |
| Run deadline | none anywhere in the package | `service.go:21` defines `runDeadline = 10 * time.Minute`, applied at `service.go:208` |
| Warning render cap (frontend) | `workflow-sync-status-banner.tsx:75` renders all warnings | `office-config-sync-status-card.tsx:77` applies `slice(0, MAX_VISIBLE_WARNINGS)` |

### Three further shipped divergences, from the design's own list

The round-4 findings and the system design's divergence enumeration are two
different lists. The design named five points on which the callers diverge, and
three of them are absent from the round-4 table above. All three are shipped:

| Concern | `internal/workflowsync` | `internal/office/configsync` |
| --- | --- | --- |
| Empty path | `models.go:163-164` rewrites an empty path to `DefaultPath`, so the repository root cannot be configured | `models.go:243-245` treats an empty path as the repository root, which is why `Path` is `*string` here |
| Walk shape | `service.go:249-252` fetches one non-recursive directory listing | `walk.go:163-164` runs a bounded multi-round directory walk |
| Warning retention (backend) | no cap | `reconcile_run.go:18` caps retention at `maxRecordedWarnings = 100` and appends a truncation entry |

Two accounting notes, so the tally is auditable rather than merely asserted. The
backend retention cap and the frontend render cap are distinct concerns that
happen to share the word "warning"; the design's fifth point is the former, and
round 4's fourth finding is the latter. And the run deadline was *not* among the
design's five: it entered from Office's own `AC-OFFICE-CONFIG-SYNC-004.4a` after
that list was written, so it is a divergence discovered by building, not a
predicted one.

That is seven shipped divergences, not four. Each is deliberate. Office rejects
an empty provider because it is new and has no pre-provider clients, while
workflow sync's GitHub default is what every pre-existing row implicitly was.
`configsync/models.go:306-312` documents the verdict divergence in the source
itself.

### The shared surface, itemized

The genuinely common, collision-free surface is roughly 126 lines. The count is
given per item so a future reader can check it rather than take it on trust, which
only works if the counting rule is stated: **every figure below is physical lines,
including doc comments and internal blank lines, over the span named in its own
row.** Figures are `workflowsync` / `configsync`, non-test.

| Item | Lines | Span counted | Status |
| --- | --- | --- | --- |
| `Poller` | 81 / 83 | All of `poller.go`, which holds the 60s tick constant, the struct and its four methods, and nothing else | Identical in logic; the only non-comment difference is the logger component string |
| Client provider interfaces + compile-time assertions | 28 / 29 | Both interface declarations plus the `var` assertion block, first doc comment through the closing `)` | Identical but for one comment line (`configsync` documents "Satisfied by github.Service." on both interfaces, `workflowsync` on only one) |
| Provider-neutral directory entry struct | 8 / 6 | The struct with its doc comment, three lines here and one there | Same three fields; unexported `dirEntry` here, exported `DirEntry` there. The declarations themselves are 5 lines on both sides |
| Per-workspace lock helper | 4 / 4 | The function; neither side has a doc comment | Byte-identical |
| Provider constants | 5 / 5 | The `const` block with its doc comment | Byte-identical, comment included |
| **Total** | **126 / 127** | | |

Three further candidates were examined. Two are **not** part of that surface;
the third is, and is listed here only because it is easy to assume otherwise:

- **Content hashing.** `workflowsync/service.go:325` (`contentHash`, 8 lines) and
  `configsync/reconcile_run.go:441` (`computeRunHash`, 22 lines) share a purpose
  (an informational digest, never used to skip reconciliation) but not an
  implementation. They take different inputs (a flat file slice versus a walk
  result spanning four entity kinds), frame the hash differently (`path` plus
  content length versus `kind` plus `path`), and order differently (walk order
  versus an explicit sort of skill directories). What is common is the
  `sha256` plus `hex` idiom, which is the standard library.
- **The HTTP controller.** Both expose the same four routes (get, set, delete,
  force-sync), and there the resemblance stops. Workflow sync's `Controller`
  (159 lines) self-mounts on the root engine at `/api/v1/workflow-sync`, takes
  the workspace from a `workspace_id` query parameter, and re-authorizes in every
  handler by translating a service denial into a 404. Office's `Handler` (111
  lines) is registered onto a router group the caller owns, takes the workspace
  from the `:wsId` path parameter, and deliberately does not re-authorize because
  `officeWorkspaceScopeMiddleware` has already scope-checked it. The workspace
  source, the mount model, and the authorization model all differ. This is a seam,
  not shared code, and it is counted in the Decision's tally below.
- **Provider constants** are in the table above: genuinely shareable, and five
  lines under the convention stated there.

Two asymmetries appeared only once the second package was real. The stores are
not the same table under two names: config sync adds an ownership manifest table
and its own indexes, while workflow sync carries two idempotent `ALTER TABLE`
migration helpers for databases that predate its poll toggle and its GitLab
columns. Office has no legacy rows and will never need them, so a
table-parameterized shared store would carry workflow sync's migration history
into a package Office depends on. The packages are also lopsided: 1,105 versus
4,050 non-test lines, because config sync's bulk is reconciliation across four
entity kinds with no workflow sync counterpart.

## Decision

Do not extract `internal/reposync`. `internal/workflowsync` and
`internal/office/configsync` remain separate packages that share a pattern rather
than code.

A shared layer would have to parameterize the verdict formula, the provider
default policy, the run deadline, the empty-path meaning, the walk shape, the
warning caps (backend retention and frontend render, separately), the store
schema and its migration history, and the HTTP controller's workspace source and
authorization model, in order to save roughly 126 lines that do not collide.
Every one of those seams exists to let the two domains disagree, which means the
abstraction would encode the disagreement rather than remove it.

The duplicated `Poller` is real but is not a repository-sync concern. A
`Start`/`Stop` lifecycle over a context cancel and a `WaitGroup` appears in at
least five packages: `internal/integrations/healthpoll`, `internal/gitlab`,
`internal/linear`, `internal/workflowsync`, and `internal/office/configsync`.
The shape is common to all five; the specific choice to hold the mutex across
`WaitGroup.Wait()` is shared only by the two sync pollers, while `healthpoll`,
`gitlab` and `linear` release it before waiting and re-acquire it only to clear
`started`. Extracting the lifecycle into a `reposync` package used by two of
those five would put it in the wrong home and leave the other three copies in
place. It is recorded as separate, repo-wide follow-up work rather than folded
into this decision.

## Consequences

A change to sync behavior that should apply to both domains must be made twice,
and nothing in the build enforces that. This is accepted: no such change has been
required so far, and the seven divergences above are evidence that most changes
apply to exactly one domain.

The two packages may drift further apart. **This supersedes three statements in
`docs/specs/office/system-design/config-sync.md`**. That design has been updated
so none of them still reads as pending, which makes this ADR their only record;
they are quoted in full here for that reason:

- *Purpose and boundaries*: "Extracting the common mechanics is deferred so it
  can be designed against two working implementations rather than one working and
  one imagined." The deferral is resolved. Nothing is pending.
- *`internal/office/configsync` (new)*: the vocabulary and lifecycle are
  identical "by *convention*, enforced by this design and by tests, not by a
  shared type". The convention is retired, and it was never actually enforced by
  tests: no shipped test compares the two packages' column names, field shapes,
  or lifecycle.
- *Prior art and alternatives*: the duplication is "bounded by building Office to
  the *same* vocabulary (identical column names, the same `SyncResult` field
  shape, the same poll-and-record lifecycle) so a later extraction is a merge of
  two working implementations rather than a redesign. A follow-up card carries
  the extraction". This ADR is the disposition of that follow-up card.

All three were reasonable when the extraction was still expected. Now that it is
declined, holding the vocabulary aligned buys nothing and would constrain both
domains for a merge that is not going to happen. Each package owns its contract,
and neither honors a behavior-preservation invariant for the other.

Both client provider interfaces stay duplicated, but each keeps its
compile-time assertion that the real `github.Service` and `gitlab.Service`
satisfy it, so drift in either integration's workspace-routed methods still
breaks the build in both packages rather than surfacing at wiring time.

Poller lifecycle duplication remains. That work is tracked as Kandev card `fc386556-8101-49a6-ac22-97b526c66e50` ("Unify backend
Poller Start/Stop lifecycle across 5 packages"), scoped repo-wide and
independent of this decision: whatever home it picks, it does not reopen the
question of a `reposync` library for these two.

**Revisit this decision if any of the following holds.** Absent one of them, two
similar packages is the intended steady state and no revisit is owed. Nothing in
the build or in CI watches for these; they are checked by whoever is already
editing one of the two packages. The first two are state, readable from the code
at any moment. The third is a count over time, so it needs somewhere to
accumulate or it resets with each reader's memory: the Occurrences list below is
that place.

- A third repository-sync domain is proposed. Three callers change the
  arithmetic: the shared surface is paid for once and amortized across three,
  and a third domain is also evidence that the pattern is a genuine abstraction
  rather than a coincidence of two.
- The divergence count materially shrinks, for example because a requirement
  change aligns the verdict formula and the provider policy. The seam count, not
  the line count, is what made this a no. The seven divergences are tabulated in
  Context across two tables; re-reading them against the code is the whole check.
- A behavior change is required in both domains at once, twice. The first time
  is the cost this ADR accepts; a repeat indicates the domains are coupled in a
  way this analysis did not find. Append each occurrence below, with the date,
  the change, and both call sites.

### Occurrences

None recorded. This list is only as good as the discipline of appending to it,
and it is the sole record: there is no counter anywhere else, so an occurrence
that goes unwritten here is an occurrence that did not happen as far as the next
reader is concerned.

Appending an occurrence is a post-acceptance edit to this document, so it also
takes the repository's amendment marker: change the Status line to
`accepted (amended YYYY-MM-DD)`, the form 36 of the 270 files under
`docs/decisions/` already use. The marker records that this document changed,
not that the decision reversed. Without the bump the append leaves the Status
line unchanged, and that line is the only place it would surface: a human skim
reads it directly, and `list-docs.py decisions` echoes it in the Status column.
The `--status accepted` filter is not that signal, since it matches on the
leading word alone and lists an amended document either way.

## Alternatives considered

**Extract `internal/reposync` with a `Domain` seam.** The round-4 findings imply
a minimum of a verdict hook and a provider-policy hook, and the shipped code adds
a deadline policy, an empty-path policy, a walk-shape strategy, two warning-cap
policies, a store/migration seam, and a controller mount/authorization seam.
Rejected: that is nine parameterization points, and they would buy roughly 126
shared lines of which only the `Poller`'s 81 are logic rather than declarations,
and the `Poller` belongs in a five-package home this library would not serve. The
original design's `Domain` seam had no verdict or provider hook at all, so the
prediction was already wrong about its own shape.

**Extract only the collision-free surface.** Rejected on its own terms. The
interfaces, the directory entry struct, the lock helper and the provider
constants are declarations rather than logic, so sharing them buys little, and
the one piece with real logic, the `Poller`, has a five-package scope that a
`reposync` package cannot serve.

**Migrate workflow sync onto config sync's semantics first, then share.** This
would change shipped behavior for existing workspaces: warnings would stop
flipping the unchanged verdict, an omitted provider would start failing, a
configured path of `""` would change meaning from "the default directory" to
"the repository root", and every sync run would acquire a ten-minute deadline it
does not have today. Rejected as a behavior change with no requirement asking for
it. Notably, no shipped test covers "warnings present, zero changes"
(`TestSyncWorkspace_AlwaysReconciles` has no warnings, and
`TestSyncWorkspace_BrokenFileBecomesWarningAndNilExport` never asserts
`Unchanged`), so this migration could regress the verdict in either direction
without failing CI.

## Related specification

`docs/specs/office/requirements/config-sync.md` and its siblings define the
Office contract whose acceptance criteria drove most of the divergences above.
`docs/specs/office/system-design/config-sync.md` records the original deferral
and the condition for this revisit; see Consequences for the three statements of
its that this ADR supersedes. That design's Related decisions section links back
to this ADR.
