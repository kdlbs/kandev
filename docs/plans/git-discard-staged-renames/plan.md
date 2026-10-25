---
created: 2026-10-07
status: implemented
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
legacy_specs: []
---

# Implementation Plan: Discard a staged rename as one change

## Overview

Selecting a staged rename destination in Changes and choosing Discard must restore its
committed source and remove its destination together. Today destination-only classification
can report success while leaving the source deleted and its deletion staged. One sequential
work order proves the real failure, implements bounded pair selection, and verifies actual
Git/files/index outcomes through the operator and registered repository transport.

Platform owns the shared selected-change mutation contract. Workspaces and Tasks retain
repository/environment identity; UI retains the existing Changes presentation. Reuse
[the active owner](../../specs/platform/requirements/workspace-git-status.md), specifically
`.44` and the minimal missing recognized-rename contract `.47`. The existing path-details
supplement links the main design, whose baseline size is 32,685 of 32,768 bytes.
No duplicate incident requirement, independent UI requirement or new ADR is needed.

## Admission and assumptions

The design turn produced exactly four unstaged, uncommitted artifacts: the owner requirement,
existing path-details design, this manifest and one work order. ROOT subsequently reviewed
their immutable hash receipt and released implementation with an exclusive local-heavy lease
in the same primary session. No production/permanent tests, fixtures, Go checks, installation,
commit or PR ran during design. Same primary/profile/executor; no delegates, new tasks,
sessions, tabs or model switch.

Confirmed: destination selection means undo one recognized staged rename; preserve unrelated
index/worktree content and repository/path/environment safeguards. Conservative local
choice: reject occupied sources, unsupported pairs and ambiguous endpoint overlap before
request mutation. Verified source facts below establish the cause. No material design
question remains; a later transport or filename integration gap checkpoints ROOT before
expanding ownership.

## Evidence and source identity

Baseline: `a5f6f7722ee0ebf6ae966f96a7d9c8a17ce6f8fc`, initial child worktree clean.
Accepted ROOT receipt/classification/log: `/tmp/kandev-root-renamed-file-discard-proof-receipt.json`,
`/tmp/kandev-root-renamed-file-discard-proof-classification.json`,
`/tmp/kandev-root-renamed-file-discard-proof-test.log`.
Actual `GitOperator.Discard` on real staged `git mv old.txt new.txt` receives only `new.txt`.
Its filtered status is A; result is successful, destination disappears, source stays absent,
and D old.txt remains staged. Two causal assertions fail while ordinary modified and staged
added controls and unselected index/worktree/HEAD checks pass.

Original native 12299/start chunk 554210/terminal chunk 17cfcb actually joined exit 1,
UTC 03:03:32.129309Z to 03:03:40.961776Z, actual package 0.261s. PID/PGID 382279,
wrapper 382277 and outer group 382256 were physically gone; scratch overlay removed, ROOT
clean. This is accepted evidence, not a child rerun. Protected mode-0400 proof candidate
`/tmp/kandev-root-renamed-file-discard-next-candidate_test.go` has SHA256
`33e098e2a38c29936bc7ddd60f245182aea85c19997e73da538c730b28c8c892`;
never replay, copy, import, mutate or remove it. Permanent tests must be independently authored.

Fresh child comparisons match all four source hashes:

| Source | SHA256 |
| --- | --- |
| `apps/backend/internal/agentctl/server/process/git.go` | `8f3ed48fd867fdabadbc0d1accb9e50e377caca3ec25f93c05861e4dfad38e16` |
| `apps/web/components/task/task-changes-panel.tsx` | `3c49b30f685774aaddb65bd2d0e9914ebc84465075f1d40a992e8c2e3fe04eeb` |
| `apps/web/components/task/changes-panel-hooks.ts` | `0e07ed387b38cfa745827b28791168f089f407d05acea539e0e3eb0323641896` |
| `apps/web/hooks/domains/session/use-session-git.ts` | `6ffd907705b30c29620cbe1061c9b68143356cfe2be2d7bea5078de2875077e4` |

The current tracker uses unfiltered tracked text porcelain and retains `FileInfo.OldPath`,
with staged rename origin in mixed facets. Discard instead queries each literal destination
with filtered text porcelain and cannot recover the origin. Its runner merges stdout/stderr;
its flag validator does not yet allow `-z` or `--untracked-files=no`. Local installed Git
status/restore manuals confirm NUL destination/source ordering and removal of indexed paths
absent from the restore source. Read-only `ls-tree -z HEAD --` checks on this checkout confirm
that `:(literal)README.md` and a nested literal path return exact NUL tree records. No Git
fixture, mutation or Discard proof replay was run during design.

The UI trace confirms destination-only `handleDiscard` and `getDiscardOperations` through
`useSessionGit` repository routing. This is source reachability, not executed browser,
HTTP, WebSocket or real UI selection evidence. Earlier literal-selection delivery is recorded
in [its completed package](../git-discard-literal-selections/plan.md); its historical outcomes
remain accurate and need no rewrite. Other workspace-status linked packages remain unchanged.

## Scope

### In scope

- One Discard-local fresh tracked NUL-status parser and selected staged-rename preflight.
- Pair restore through existing literal selected commands and result/refresh contracts.
- Exact required internal status flags, real tests, registered selected-repository proof.
- Minimal implementation-time public Git reference clarification and accurate delivery results.

### Out of scope

Whole-repository reset/clean, restoring every deletion, guessed copies, rename configuration
changes, status tracker/parser migration, Stage/Unstage changes, rollback or all-writer
concurrency policy, schema/API changes, framework/cache/configuration abstractions, metrics,
frontend production/layout/copy, browser builds and unrelated audits. No access to child61
workspace-file/terminal work or child62 chat context worktrees. Preserve foreign processes,
caches, refs, managed dependencies/worktree and ROOT proof.

## Technical approach

Follow [Selected staged renames](../../specs/platform/system-design/workspace-git-path-details.md#selected-staged-renames).
Keep the existing operator lock. Read full tracked status before selection because filtering
can erase rename identity. NUL framing makes special native filenames unambiguous without
changing the tracker parser. A committed-source/absent-HEAD-destination tree check, source
`Lstat` absence and unique nonoverlapping pairing authorize only the selected rename origin.
Restore both endpoints with one existing HEAD/index/worktree restore selection; exclude the
rename destination from added-file removal. Refuse unsafe pairing before any request mutation.

| Boundary | Identity and evidence | Behavior / unsupported fallback |
| --- | --- | --- |
| Operator | Real repo, fresh NUL porcelain, selected literal destination | Unique staged R pair restores both endpoints; failed evidence or unsafe pair fails without mutation. |
| Ordinary files/copies | Existing literal paths, no authorized R origin | Tracked restore or added/untracked removal; never infer an origin from A/D or similarity. |
| Registered HTTP | `GitDiscardRequest.Repo` through `gitOpForRepo` / `Manager.GitOperatorFor` | Named selected repository only; invalid scope retains existing failure. |
| Desktop/phone | Destination-only source trace, same operation result | Existing success/error/refresh; no new UI transport fields or presentation. |

The small exact flag admission belongs in `common/securityutil/git.go`; do not weaken
generic validation or add prefix-wide acceptance. Use bounded new Discard helper/test files
instead of enlarging `git_test.go`. No API production modification is currently causal.

## Tests

| AC | Planned permanent function / file | Independent observable oracle |
| --- | --- | --- |
| `.47`, `.44` | `TestGitOperatorDiscardStagedRenames`, process `git_discard_staged_renames_test.go` | Source restored in index/worktree, destination absent, no residual D; pure/edited/mixed/multiple/native filenames plus ordinary/copy and unrelated staged/worktree controls. |
| `.47`, `.44` | `TestGitOperatorDiscardStagedRenameRefusals`, same file | Recreated source, directory/symlink occupation, unsupported/ambiguous pair, failed/framing-invalid external status evidence and mixed selections fail truthfully without request mutation. |
| `.47`, `.44`, `.9` | `TestHandleGitDiscardStagedRenames`, API `git_discard_staged_renames_test.go` | Actual registered status response contains rename destination/origin (mixed staged facet for edits), destination-only Discard restores pair in selected repo, subsequent status removes it, other repo and neighbors unchanged. |
| `.47`, `.44` | `TestHandleGitDiscardStagedRenameRefusals`, same API file | Real registered result rejects occupied source, invalid repository and empty request, all index/worktree evidence unchanged. |
| `.47` execution dependency | `TestIsKnownSafeGitFlagDiscardStatus`, securityutil `git_test.go` | Exact required flags accepted, unsupported variants rejected; real operator/HTTP tests remain the substantive proof. |

All read assertions preserve config/HEAD/refs/content/environments. Mutation assertions allow
only the selected changes and paired origins to change; compare unrelated index mode/blob
content rather than raw index-file stat bookkeeping. Test details and exact commands live in
the single work order. Existing literal/ambient-pathspec suites run only because the new status
read and expanded restore selections affect those exact boundaries.

## E2E evidence and mobile parity

The registered real HTTP status-to-destination-discard-to-status flow is the end-to-end
boundary for this backend correction. No mocked Git or invented metadata establishes rename
visibility. The mobile pure-data exception applies: shared result semantics change, while
layout, navigation, touch, scrolling, copy and breakpoint behavior do not. Operator and
registered transport tests cover it. No Playwright/E2E build or ASCII UI preview is required.
If fresh source reveals a causal UI wiring issue, checkpoint ROOT before scope expansion.

## Public documentation

Audited `docs/public/git-operations.md`, README and screenshot catalog. The reference page's
Everyday Operations Discard row and `worktree.discard` row need a small implementation-time
clarification: selecting a recognized staged rename destination undoes its source/destination
pair; occupied or unsupported pairing fails, preserving recreated content. No public file
changes during design; no screenshot or navigation change. Run both public-doc validators
when that minimal edit is made.

## Work orders

- [x] [Task 01: Undo a selected staged rename safely](task-01-discard-staged-rename.md)

One sequential work order. No planned delegates or new sessions. ROOT released local implementation and publication under the exclusive lease. Hosted checks
and review remain pending; merge requires a separate ROOT serial MERGE INTERRUPT. Published head freezes except actual findings. Publication alone does
not complete the persistent task.

## Verification results

Historical design checks passed:

- Catalog validation: 357 decisions and 1,414 specifications.
- Specification-linter tests: 36 passing; full specification lint passed.
- Owning requirement/design remain discoverable in the Platform catalog and below limits.
- Repository `validateCoverage` preflight: `covered`, `ok:true`, `errors:[]`; one changed
  work order links the owner, `.44`/`.47`, design and plan for planned runtime paths.
  This is structural design linkage, not executed runtime proof.
- `git diff --check` passed; exactly the four intended files are unstaged/uncommitted,
  with an empty staged diff and no production/permanent-test changes.
- Lightweight native handles 16915 and 46749 actually joined exit 0; no heavy command or
  outstanding owned handle. All read-only Git subprocesses returned terminal exit 0.

Implementation was later released and completed in this primary session. Independently
authored real operator RED tests failed for the missing committed source and residual staged
deletion; registered fresh-status → Discard → fresh-status RED also exposed that deletion.
Occupation/empty-entry and scoped external-evidence refusal tests failed causally on baseline.
The external fault executable intercepts only the full tracked NUL status command, leaving
ordinary status and mutation prerequisites enabled; its reachability is asserted independently.
No ROOT proof replay or import was used.

The bounded helper reads full tracked NUL status, validates eligible unique staged rename
pairs against literal HEAD tree evidence and source absence, then restores both endpoints
with the existing literal command. Ordinary classification moved unchanged to the helper
file to keep Discard within the existing function budget. Only two exact flags were admitted.
No API/frontend production, tracker, schema, configuration or framework changes were needed.

The work-order's exact final race selectors passed: operator package 7.087s (including edited,
multiple, source-only, native filename and refusal cases plus existing literal/environment
controls), registered API package 7.809s (actual status-visible pure/mixed rename, selected-repo
mutation, refusals and ordinary/literal controls), and securityutil package 1.013s. Source and
neighbor index mode/blob/file bytes, unrelated staged deletion, other repository, HEAD/refs/
config and environments stayed within the selected-change contract. Scoped lint returned
0 issues with concurrency 2, serial admission and the exact baseline revision.

The public guide's two Discard rows now describe pair restoration and truthful refusal.
Public-doc validator tests passed 62/62; validation passed for 47 published pages. Catalog,
full specification lint and whitespace checks passed. These are backend/registered HTTP
proofs; no browser execution or screenshot claim is made. External writers remain outside
atomicity guarantees, and Git A/D or copy evidence never authorizes inferred restoration.

Exact original commands, UTC, native handles, logs, PID/PGID and actual join/physical closure
are retained in the own task plan and `/tmp/kandev-child63-d6c521cc/` receipts. The one conditional pinned pnpm 9.15.9 frozen install passed using existing Node 24.21.0;
normal active commit hooks remain a delivery prerequisite. Hosted CI,
full current-head CodeRabbit review, merge readiness and separate ROOT merge authorization
remain pending; publication does not complete the persistent task.

## Risks

- Git rename detection follows current configuration and similarity; independent A/D or
  copies are not automatically paired. No promise to recover a rename Git does not recognize.
- Occupied sources intentionally refuse the whole preflighted request, even with ordinary
  selected neighbors. The user must preserve/reconcile recreated content before retrying.
- Existing text status parsing is not mutation authority for exact special-name pairs.
  A status-visible filename failure requiring tracker changes checkpoints ROOT.
- External writes after preflight remain outside atomicity guarantees. Source absence is
  rechecked at mutation; execution failures are reported without a rollback promise.
- Full tracked status adds one bounded admitted read per Discard; untracked tree enumeration
  is excluded. No resource envelope or deadline increase is authorized.

## Grounded review correction

The published initial implementation still treated accepted relative spellings such as
`./new.txt` differently in rename lookup and ordinary endpoint exclusion. Greptile finding
4202800563 prompted a ROOT-authorized correction in the same order. Additional independent
operator and registered HTTP RED tests reproduced missing source/residual staged deletion,
duplicate endpoint failure and occupied-source mutation of ordinary neighbors. Raw-NUL
rejection controls passed before correction. Initial local results above remain historical
for changed inputs; corrected-head verification and delivery are pending.

The bounded correction uses a cleaned spelling only to find a candidate recognized endpoint,
then issues literal NUL status for the original argument to prove Git accepts that spelling
and identifies exactly that path. Only verified endpoint aliases share selection/dedup/
exclusion. Ordinary selections retain their raw arguments; source-only selection stays
literal, and rejected raw arguments cannot become valid by cleaning. No global validator,
tracker, API, frontend, ordinary policy or public-guide expansion is required.

The same work order records exact affected selectors, one scoped changed lint and the single
mandatory full CHANGED backend lint against the exact API PR base. Original observer/cutoffs,
normal hooks and separate ROOT merge authorization remain in force.

The first full CHANGED lint timed out after six minutes (exit 124, empty log, cause unknown),
without a lint verdict. Its native original was joined and physical wrapper/group closure
verified before ROOT authorized one identical bounded recovery against the same exact API
base. No cache-population cause is claimed. Failed and recovery originals are both retained;
recovery failure requires another ROOT checkpoint rather than a third invocation.

The identical ROOT-authorized recovery passed with 0 issues in 252.938 seconds. Both lint
originals were actually joined and their wrapper/PID/process groups verified gone. Affected
operator race coverage passed (7.669s), the added source-only/native-separator inputs passed
their narrow rerun (2.200s), and registered HTTP race coverage passed (9.798s). Scoped changed
lint passed with 0 issues. These checks preserve ordinary controls, rejected raw paths,
occupied-source refusal and selected-repository isolation. The raw-spelling correction is
ready for normal active hooks and corrected-head publication; hosted review/merge gates
remain separate.

CodeRabbit's completed initial FULL review also identified missing C endpoint counts in the
existing shared-endpoint refusal. ROOT confirmed that finding against `.47` and released a
second bounded actual-code correction in the same order. Independent real operator refusal
RED at the controlled status-command boundary precedes the smallest count correction, with
ordinary copy and alias controls. No native-generated copy shape is claimed by that fault
evidence. Keep rename alias discovery limited to R endpoints. Exact new/affected checks,
one new full CHANGED invocation and normal hooks/publication precede final-head review;
the same original observer and separate merge gate remain unchanged.

Copy-endpoint GREEN passed all four new operator cases (1.586s), and scoped changed process
lint passed with 0 issues. The new mandatory full CHANGED run timed out at six minutes
(exit 124, empty log, cause unknown). Its original was joined and physically closed. This
supplies no full lint verdict; the one order is blocked on ROOT's timeout/resource decision.
Five scoped paths remain uncommitted at f08285d. No automatic retry, head change, final-head
FULL request or physical-return claim follows the failed gate.

ROOT subsequently reviewed the failed original and physical closure and explicitly released
one identical bounded recovery for this copy correction. The order returns to in_progress.
All prior failure evidence remains; no cause/cache-population claim or bound increase is
made. Another failure requires a ROOT checkpoint, with no third invocation or publication.

The copy correction's one identical recovery passed with 0 issues in 191.081s, then its
native original and process group were joined/gone. The original timeout remains failed
with unknown cause. New operator GREEN (1.586s) and scoped changed lint also passed. The
five-path correction is ready for normal active hooks/publication and grouped disposition;
final-head substantive FULL review, hosted CI and separate ROOT merge release remain gates.
