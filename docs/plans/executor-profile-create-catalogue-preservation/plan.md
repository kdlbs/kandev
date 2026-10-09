---
created: 2026-10-09
status: implemented
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
system_design:
  - ../../specs/executors/system-design/profile-editor.md
legacy_specs: []
---

# Implementation Plan: Preserve choices during executor profile creation

## Overview

Publish an accepted normal built-in creation over the current owning catalogue
so a live profile remains available in task and subtask choices. Use ONE bounded
sequential work order: establish strict controls and independent causal
regressions, correct acknowledgement publication, then run affected checks and
deliver only under ROOT's separate release gates.

The executor system owns profiles, creation destinations and catalogue
publication. Extend its existing [requirement](../../specs/executors/requirements/profile-editor.md)
and [design](../../specs/executors/system-design/profile-editor.md#normal-creation-catalogue-publication).
Do not create a separate task-picker or UI specification.

## Admission and evidence

Child93 is task `8e90daae-db4a-450c-bf0a-a6f0933b2eae`, original primary session
`722d888e-8e71-4d5f-86e2-aa1b00e5dd8b`; ROOT parent is
`14825981-b175-411d-999a-31ddc2aa5fc3`. Task title:
**Preserve executor choices during profile creation**.

ROOT admitted this design at 03:23 UTC on 2026-10-09 after independently
verifying PR4360's normal merge
`c69077b1890487ca02699ee913b98070258c094d`, archive ABSENT and proof release.
The old brief's NOT-task wording is historical. Exactly TWO active slots remain:
child93 design and child92 exclusive local-heavy implementation/publication.
No child93 implementation release, lease or merge grant exists in this turn.

Read only the external summaries `qualified-proof.json` and
`future-design-brief.md` under
`/tmp/kandev-root-executor-create-catalogue-discovery-20261009/`.
Qualified original native45407/terminald6a572 actually joined with exit1,
ONE causal failure, TWO passing controls, BOTH catalogue and actual-options
soft assertions reached, and zero unhandled errors. Accepted target creation
succeeded. Earlier50342/3c2793 had an external Monaco `loader.config` stub error
and is not the qualified proof. Source removal, clean ROOT, original joins and
fresh groups gone are recorded ROOT receipts, not child reproductions.

The protected ROOT `candidate.test.tsx` is mode0400, SHA256
`e35e67db2af4cd9dd1a3cea1b2114523cf4867aa249ccf433f1aaf640fa24e98`.
Never read, copy, import, replay, edit, chmod or remove it. Independently author
permanent real regressions only after release. No proof duplication.

Design inspection found local HEAD and `git ls-remote origin refs/heads/main`
both at `c69077b1890487ca02699ee913b98070258c094d`; initial worktree was clean.
All three qualified Git source blobs match:

| Source | Git blob |
| --- | --- |
| `apps/web/app/settings/executors/new/[type]/page.tsx` | `a1bf18e1a5a087bcf1fbb7409397fc30d4374e53` |
| `apps/web/app/settings/executors/new/[type]/executor-types.ts` | `bee74777c64fc6432e7d2453dc6d0ac71baf9bc4` |
| `apps/web/lib/ws/handlers/executor-profiles.ts` | `4d8fb6402625510902796f8d9f574e86ab82098d` |

Recheck source/main and the owner pair after ROOT's later reviewed-package
interrupt, before permanent edits. Reconcile moving-base changes without
rewriting child90's separate normal edit correction.

## Scope

### In scope

- AC-EXECUTORS-PROFILE-EDITOR-001.16 through .18 in the existing owner pair.
- Normal local/worktree/local_docker/sprites creation acknowledgement only.
- Current different-owner entries, current owner metadata and sibling profiles,
  unrelated removals, accepted target and actual eligible picker options.
- WebSocket-before-HTTP target membership through the selected current-store
  upsert, independently tested; strict success and rejection/draft controls.
- Current source/caller audit, targeted validation and gated delivery receipts.

### Out of scope

- Backend deletion, ordinary existing-profile edits, script persistence,
  payload redesign, server arbitration and missing-owner resurrection.
- SSH, Remote Docker, Kubernetes or plugin writer redesign; generic cache,
  global revision, schema, backend, framework and feature-flag migration.
- Notification-after-acknowledgement or duplicate-event writer redesign,
  repairing previously duplicated rows, and newer-target update arbitration.
- UI composition, copy, navigation structure, mobile layout, broad E2E/build
  suites, generic QA/review/simplify and optional polish.
- Native delegates, new persistent tasks, sessions/tabs or model switches.

## Technical approach

`useCreateProfileSave` captures `executors.items`, awaits the real
`createExecutorProfile` adapter, then maps that old array into whole-array
`setExecutors`. A registered created event can publish an independent choice
while transport is pending; acknowledgement then removes that choice locally.

After acknowledgement, read the owning store through `useAppStoreApi`, find
the current owner by `executorId`, and immediately reuse
`upsertExecutorProfile(current, currentOwner, acceptedProfile)` from
`profile-edit-page-chrome.tsx`. If the current owner is absent, leave the
catalogue unchanged rather than invoking the helper's synthesis fallback.
Remove only the creation hook's stale selector/dependency. Preserve payload,
form prerequisites, contributor identity, dirty/revision handling, rejection
propagation, saving/error behavior and real canonical navigation.

The service publishes `ExecutorProfileCreated` before the HTTP handler writes
its DTO. A naive append to the current array can duplicate the accepted target
when its WS notification arrived first. The existing upsert replaces by target
ID; test counts in both catalogue and actual options. Leave the live handler
unchanged. If current source invalidates this bounded approach, checkpoint
ROOT before an alternative or expanded writer change.

### Caller compatibility matrix

| Caller | Owner and transport | Intended behavior and evidence | Fallback/exclusion |
| --- | --- | --- | --- |
| Local | `local`, `exec-local`, normal profile POST | Same common hook; source audit plus common payload/control regressions | No new owner synthesis |
| Worktree | `worktree`, `exec-worktree`, normal profile POST | Faithful real-page causal integration, controls and actual choices | Existing missing-owner behavior |
| Docker | `local_docker`, `exec-local-docker`, same POST | Common hook; audit retained build/image/network/user-namespace gates | No daemon or runtime claim |
| Sprites | `sprites`, `exec-sprites`, same POST | Common hook; audit retained secret/env/credential/network configuration | No provider provisioning claim |
| SSH | Separate executor+profile POSTs | Current-store publication in `SSHCreatePage` | Read-only audit; no redesign |
| Remote Docker | Separate executor+profile POSTs | Current-store publication in `RemoteDockerCreatePage` | Read-only audit; no redesign |
| Kubernetes | Separate create component/resource hook | `useKubernetesExecutorResource` reads current store | Read-only audit; no redesign |
| Unknown/plugin | Unknown route fallback or separate provider surface | Keep dispatch and unavailable/capability behavior | No new supported shape |

## Tests

New independently authored file:
`apps/web/app/settings/executors/new/[type]/create-profile-catalogue-publication.test.tsx`.
A companion `.test-helpers.tsx` may hold bounded fixture setup/cleanup if needed.
Use the real page, providers, coordinator, API adapter, registered WS handlers,
router and actual options hook. Only external fetch transport and Monaco
renderer/loader capabilities may be stubbed. Do not reuse the protected proof.

| AC suffix | Planned test name and evidence |
| --- | --- |
| .18, .16 | `unchanged catalogue creation submits payload and opens accepted profile`: strict baseline, actual POST payload, one target and real canonical route |
| .18 | `unchanged catalogue rejection retains draft and route`: strict baseline, failed coordinator result/contributor, current rows, Name value and existing error |
| .16 | `accepted creation retains another owners live profile and choice`: held POST, actual created handler for another existing owner, pre-ACK presence, BOTH post-ACK soft assertions and accepted target success |
| .16 | `accepted creation retains mixed different owner changes`: actual store/WS additions, updates and removals; current values/options retained and removed rows stay absent |
| .16 | `accepted creation retains current owner metadata and siblings`: independently causal same-owner addition/update/removal and metadata changes during held POST, BOTH assertions |
| .17, .16 | `notification before response leaves one accepted target and choice`: actual created handler for target plus independent profile; ACK uses accepted target fields, exactly one target/option, retained current metadata/siblings |
| .18 | `rejected creation keeps live catalogue and unsaved draft`: later independent event then rejection; no acknowledgement write/navigation, failed contributor and recoverable draft |
| .18 | `creation does not restore an owner removed during transport`: preserve original missing-owner map behavior without synthesizing owner or unrelated rows |

Run the TWO unchanged controls first against untouched production. Independently
establish meaningful causal RED with the current-store and actual-options
assertions both reached, original terminal result and zero unhandled errors.
Same-owner claims need their own causal failure before correction. A previously
passing event-before-ACK control remains valuable: it prevents the selected
current-store fix from introducing a duplicate. Do not claim it was RED if it
was not. No baseline assertion weakening or stub-error acceptance.

## End-to-end and mobile evidence

The component integration crosses real form editing, shared save coordination,
API transport adaptation, real live handler publication, acknowledgement,
current catalogue, actual option projection and real navigation. It supplies
the end-to-end outcome for this state/data repair. The mobile-parity exception
applies because composition/copy/touch/scroll/navigation structure and
viewport-dependent interaction stay unchanged. No new Playwright spec, browser
build or ASCII UI preview is planned. Reassess if scope changes.

## Documentation and decisions

Public-document review uses `/docs-maintainer`. The existing public
`docs/public/executors.md` creation/selection guide, root README and screenshot
catalogue still describe the same user procedure, settings and route. This
repair restores catalogue choices without a new operator instruction or
visible control; internal owner specs/plans change, public docs need no edit.
Store ownership and existing upsert semantics are already established, so no
new ADR or AGENTS.md boundary change is needed.

## Work orders

- [x] [Task 01: Preserve the current creation catalogue](task-01-preserve-create-catalogue.md)
  (`done`, wave 1, sequential, no delegates; delivery gates remain separate).

## Verification results

### Design checkpoint history

Design document checks passed: `python3 scripts/list-docs.py validate` validated
365 decisions and 1478 specifications (exit0);
`python3 scripts/lint-spec-files.py --all` passed (exit0); `git diff --check`
passed (exit0). All four expected documentation paths are unstaged/uncommitted;
no production or permanent test path changed. Requirement/design sizes are
8441/22110 bytes, below their limits.

The lightweight actual-design/planned-trigger documentation coverage preflight
could not start: shell `node` was unavailable, original command exited127.
No Node runtime substitution, retry, install or alternate coverage run was
attempted. ROOT must resolve this environment checkpoint before later execution;
the Node-based preflight is NOT RUN, and no `exempt`/`covered` result is claimed.
The package's explicit requirement IDs and paths were checked against the
owning pair while authoring; this does not replace the deferred coverage gate.

Product RED/GREEN, install, ESLint, typecheck, i18n, hooks and publication are
NOT RUN and require later ROOT release/lease. The work order owns exact commands
and actual receipts; this manifest does not authorize them. All short design
commands returned terminal outputs; no yielded native handle remains. No
implementation lease, observer, PR or merge was started.

### Implementation validation

ROOT reviewed all four artifacts and explicitly released this original primary
with the exclusive CHILD93 lease at 2026-10-09T03:36:04Z. Source/main and the
three qualified blobs matched the admitted snapshot. The deferred actual
design-only preflight passed `exempt`, `errors: []`, preserving the original
PATH exit127 history. Existing Node24.21.0 mise PATH activation was authorized;
the one conditional pnpm9.15.9 frozen install reused 935 packages/downloaded0.

Strict unchanged creation/rejection controls passed first (2 tests). Independent
RED had 5 failing cases and 3 passing controls, with BOTH catalogue/options
soft assertions reached, separate same-owner causal evidence and no unhandled
errors. GREEN passed 46 tests across the four scoped suites. Subsequent test-only
lint grouping/constants and typed notification-envelope corrections each
reran only the eight changed regressions and affected lint inputs; final new
suite passed 8/8. Initial fixture punctuation/matcher errors, initial lint
warnings, and initial helper typecheck errors are retained as failed receipts,
not qualifying evidence. The event-before-response target-count assertions
passed on the old source while independent-choice retention failed; no old-source
duplicate claim is made.

Final affected ESLint, typecheck, `i18n:check`, and `i18n:ratchet` passed. Catalogue
validation passed (365 decisions, 1478 specifications); specification-linter
tests passed (36); full specification lint passed. Actual seven changed-path
documentation coverage passed `covered`, `errors: []`, one changed work order.
The existing requirement/design remain `active`/`current`; the implementation
matches the reviewed bounded creation section. Exact commands remain in the
work order, with original cwd/argv/bounds/handles/joins/groups receipts under
`/tmp/kandev-child93-20261009/`.

No public-doc edit, new mobile/browser test or screenshot is needed for the
state/data exception described above. No production change outside the creation
hook, no WS/backend/editor writer rework, no proof access/replay, no delegate or
new task/session/model. Normal hook commit and ready publication follow these
checks under the lease. Heavy RETURN, ROOT qualification, hosted observation and
merge/archive receipts are recorded in the live task plan; local `implemented`
does not authorize them.

## ROOT execution and delivery contract

The later reviewed-package interrupt and one exclusive GLOBAL local-heavy lease
are both mandatory. Preserve two-task count. Local install/tests/lint/typecheck/
hooks run serially with Node4GiB, maxWorkers1 and no file parallelism. Record each
original native handle, cwd, argv, start/timeout bound, PID/PGID ownership,
terminal result and actual join. Verify fresh exact owned groups are gone
before RETURN; ROOT qualifies it. Resource/timeout/transport/unknown/out-of-scope
checkpoints go to ROOT before retries, alternatives, cache wipes or foreign
kills. Keep managed worktree/deps/shared caches/foreign processes untouched.
Normal hooks stay active; no bypass. Only meaningful scoped RED/GREEN and
affected remediation checks, no broad passing replay or duplicate proof.

Create a clean pushed ready PR using the live template and verify its body.
Freeze the exact head except real corrective findings. AFTER ready PR linkage,
require canonical caller-bound associated-repository complete readback with
`errors: []` and all five task switches FALSE: `auto_fix_enabled`,
`auto_merge_enabled`, `prompt_on_closed`, `prompt_on_merged`,
`prompt_on_review_requested`. Do not prematurely mutate unattached providers;
already-correct association needs no relink.

After ROOT qualifies local-heavy RETURN, attach ONE original all-terminal90m
observer (GNU timeout91m, kill10, cadence60). Retain its exact native handle
across crashes; never duplicate the GitHub timer/observer or replace it before
actual END/join/groups-gone and ROOT authorization. Use ROOT's released observer
contract with repository helpers, without a second timer or native poller.
At ROOT release perform ONE first bounded semantic inspection WHILECI.
Authenticated configured CodeRabbit App347564 substantive FULL CURRENT ALLFILE
`source=covered`, `kind=reviewed` automatic report can suffice with ZERO requests.
At most ONE necessary real-gap request after inspecting automatic coverage;
ACK/progress is not evidence. Ground findings, defer polish and avoid head churn.

Need fresh complete exact-head SIX required contexts plus actual Backend,
Frontend and E2E parent SUCCESS, `errors: []` and zero hidden/actionable/human
findings. No blind, whole or passing hosted reruns. Only ROOT-budgeted exact
audited failed job and normal dependencies after parent-terminal/current-OPEN/
frozen-head checks; budgets are workflow+jobname keyed without ID resets.

MERGE AUTHORITY NONE until a separate ROOT serial static-compatibility grant.
Use normal expected-head squash, no admin bypass/rebase/synthetic merged tests.
Verify actual merge SHA/tree/parent/all owned blobs/remote main inclusion and
all owned handles actually joined END. ROOT independently fast-forwards,
archives ABSENT, releases proof and refills. Callback delivery/acknowledgement
is optional and never a gate; ROOT reads plan/conversation. Critical blocking
decisions use the parent-question barrier and immediately end that turn, with
no operator approval question or follow-on tool call.

Never touch paused oversized task `a6032d95-cc1e-4db8-adca-5b643ae4138d`,
`serialize-workspace_e3isbq2j`, or unproved volume
`2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.
Preserve the task-plan system marker, IDs, user edits, title ownership,
question barriers, completion gates, autopilot rules and final-action rules.

## Risks

- Reading before acknowledgement or inserting an await after the current read
  recreates catalogue loss.
- Appending the accepted target to a current catalogue duplicates a target
  whose real notification arrived first; upsert and count assertions prevent it.
- Passing a captured owner invokes stale metadata or unintended missing-owner
  synthesis; resolve only the current owner.
- Fixtures that bypass providers/coordinator/API/WS/options can pass while the
  production path still fails. Keep controls strict and cleanup actual.
- Source/main or child92 may move before release; recheck exact owning contract
  and source rather than assuming this snapshot remains current.
