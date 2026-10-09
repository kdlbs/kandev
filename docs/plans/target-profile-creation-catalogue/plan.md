---
created: 2026-10-10
status: implemented
requirements:
  - REQ-AGENTS-TARGET-CREATION-CATALOGUE-001
system_design:
  - ../../specs/agents/system-design/target-profile-creation-catalogue.md
legacy_specs: []
---

# Implementation Plan: Preserve current target profiles during creation

## Overview

One sequential work order repairs accepted additional-profile publication under
an existing configured owner. ROOT reviewed the completed design and explicitly released implementation
in the same primary session. The bounded implementation and local checks are complete.

## Inputs and diagnosis

- [Requirement](../../specs/agents/requirements/target-profile-creation-catalogue.md),
  `REQ-AGENTS-TARGET-CREATION-CATALOGUE-001` and criteria .1-.6.
- [Design](../../specs/agents/system-design/target-profile-creation-catalogue.md).
- [Earlier creation contract](../../specs/agents/requirements/creation-catalogue.md)
  and [completed package](../agent-creation-catalogue-preservation/plan.md) explicitly
  exclude concurrent target changes and remain unchanged.
- Supplied ROOT receipt: main `94b762a6704d9da7f09fbc822228a5f11cbdf6bc`, native89683,
  actual join `92e311`, exit 1, one causal case and two passing controls. This is
  not child execution. No protected fixture access is authorized.

The helper assembles accepted profiles alongside captured old target profiles;
the page reads the live outer list but replaces the target with that assembled
object. Both nested membership and actual selectable choices lose a same-owner
sibling that arrived while POST was held.

## Scope

- Existing-owner create-mode accepted publication, including partial MCP results.
- Current siblings' membership/metadata and real selected/selectable choices.
- Accepted identities, normalized fields, draft remapping, validation, permission,
  coordinator failure and current navigation compatibility.

Excluded: ordinary saves (sibling105/PR4390), deletion, duplication, dynamic writers,
global store/WS/backend/API ordering or tombstone frameworks, orphan inventory,
new-agent semantics, historical plan edits and UI redesign.

## Technical approach

`agent-save-helpers.ts` supplies actual accepted submitted-profile results for full
and partial existing-owner creation, including remapped partial retries.
The extracted `useAgentCreationStoreSync` hook, wired by `page.tsx`, publishes those identities against
the current target, preserving current siblings and owner metadata. Captured old
target profiles remain a draft baseline, not publication authority. Existing
normalizers, setters, projection, WS handlers, coordinator and picker remain real.

| Path / shape | Intended behavior | Evidence / fallback |
| --- | --- | --- |
| Existing `claude-code` owner; profile POST; optional owner PATCH | Preserve same-owner current profiles and accepted normalized identities | T1-T6, actual adapters and picker |
| Accepted POST then MCP failure | Retain accepted ID/pending MCP and current siblings; shared error, no success route | T5 and helper suites |
| Rejected POST | No accepted publication; retain live choice/newer draft | T3 |
| Different owner / new-agent creation | Existing behavior unchanged | Existing `agent-create-catalogue.test.tsx` |
| Current accepted-ID copy / missing current owner | Bounded dedup/newer-copy/no stale owner reinsertion | T6 changed-boundary checks; no broader inventory promise |
| Other provider/dynamic writers | No new provider support or policy | Existing actions/normalizer preserved; outside this proof |

## Tests

The independently authored `app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx`
uses the shipped creation integration as a pattern, never ROOT's protected candidate. Named scenario owners:

| Scenario | Criterion | Required observation |
| --- | --- | --- |
| T1: retains a selected same-owner sibling after creation ACK | .1, .2, .6 | Before ACK selectable/selected; after ACK nested row, option, label and selectable choice retained |
| T2: accepts additional-profile creation without a live event | .3, .6 | Once-only identity, normalized fields, route and dirty clearing |
| T3: rejected creation retains live sibling and newer draft | .4, .6 | Failed save and unchanged creation route |
| T4: creation retains current sibling metadata | .2 | Event-updated existing sibling and newly received sibling coexist; latest metadata retained |
| T5: partial MCP failure retains live siblings and accepted identity | .5 | Pending MCP, original error, draft remap/retry without duplicate create, no success navigation |
| T6: publication guards | .2, .3 | Accepted-ID duplicate/newer copy; missing owner guard at changed boundary |

T1/T2/T3 executed before production edits: exactly one causal failure and two
passing controls, with no setup/unhandled/cleanup failure. More than one post-ACK
assertion in T1 is one causal case. T4-T6 cover distinct changed-boundary risks.
Existing helper tests are compatibility evidence, not proof of page publication.

## Rendered evidence and mobile

Real `AgentSetupPage`, `SettingsSaveProvider.saveAll`, `StateProvider`, API adapter,
normalizer, registered `agent.profile.created`, navigation, `AgentProfilePicker`
and `Combobox` provide the rendered user-flow proof. Isolate only external
`fetchJson` transport. Settle deferred work and unmount/reset navigation cleanly.
These are browser component integrations, not hosted backend Playwright E2E.

No layout, touch, scroll, copy or responsive change is planned. Shared state/data
coverage satisfies mobile-parity's narrow exception. No new ASCII UI preview,
whole-app build, untouched mobile E2E or broad passing-suite replay is required.

## Work orders

- [x] [Task 01: Preserve current target during creation](task-01-preserve-current-target.md)

Exactly one order, wave 1, no dependencies, sequential in the existing primary.
No delegation, recursive tasks, extra sessions or model switching.

## Verification results

ROOT accepted the completed design before explicit implementation release. Design
validators joined `fea8c5` (catalog), `aa2549` (specification lint), `a2dcd4`
(36 linter self-tests), and `aa7cac`/`d0abd7` (coverage, one order, six criteria,
thirteen links, whitespace). Design-only coverage and ROOT's supplied proof are
separate from the child execution below.

Implementation used ROOT's exclusive106 local lease. Every retained native handle
joined; current known owned execution groups are absent. Receipts, logs and the
independent Node24 coverage checker live in
`/tmp/kandev-child106-execution-20261010/`. No protected source was accessed.

| Check | Actual terminal join | Result |
| --- | --- | --- |
| Conditional `pnpm install --frozen-lockfile` | `e65e86` | Exit 0, once because dependencies were absent |
| Independent T1 causal RED / T2-T3 controls | `6bbcde` | Exit 1; one causal failure and two controls pass; no setup/unhandled/cleanup failure |
| Initial five-suite GREEN | `c69920` | 56 pass, T5 fails: remapped partial retry duplicated the accepted draft row |
| Corrected creation/helper suites | `631fd4` | Exit 0; 47 tests pass across three affected suites |
| Existing creation compatibility | `c69920` | Six cases pass in the initial five-suite run |
| Final real-page and publication guards | `598124`, `28b38b` | Exit 0; five page cases and five final hook guards pass |
| Scoped ESLint | `007cea`, `dc7bdd`, `86c6a9` | Initial new-test describe-size warning corrected; all five paths pass |
| Scoped Prettier checks | `c6b3ea`, `679669` | Exit 0; all five paths pass after final test formatting |
| `pnpm run typecheck` | `356063`, `2cb5a8` | Exit 0, including final guard addition |
| `pnpm run i18n:ratchet` / `i18n:check` | `b72a7d`, `5fc3e5` | Exit 0; staged new files are also checked by the normal hook |
| Actual nine-path documentation preflight | `64ebab`, `410c79` | Catalog/spec lint pass; coverage covered, no errors; one order, six criteria, thirteen valid links, whitespace pass |

There are 65 distinct passing cases across scoped runs, across the initial execution and bounded review correction; the final
correction matrix and precision checks below are recorded separately. The partial-retry failure drove replacement of the remapped
create-mode baseline entry; ordinary-save assembly remains unchanged. One short
format check overlapped the original retained typecheck, was joined, and reported
only test formatting; subsequent heavy commands were serialized. Final normal
hooks, commit/publication and current hosted evidence are recorded in the live task
plan rather than requiring metadata-only follow-up commits.

## Bounded review correction

CodeRabbit review `5477493135` and Greptile comment `4236329788` identified invalid
wire timestamps deciding the accepted-ID winner. Greptile `4236329783` identified
partial MCP retry using the older POST draft despite preserving the newer live
copy in the store. Both findings belong to criteria .2/.5 and this same order.
ROOT granted one correction lease and one batched corrective push.

| Check | Actual terminal join | Result |
| --- | --- | --- |
| Malformed timestamp RED | `77bfd9` | Four malformed-current/accepted failures, five existing controls pass |
| Newer-copy partial retry RED | `b93827` | Two failures: unnecessary older PATCH; genuine name edit with old model. Five original page controls pass |
| Five-suite corrected matrix | `e732fb` | Exit 0; 64 cases pass |
| Submillisecond ordering RED | `f4aa0c` | One valid precision failure, nine hook controls pass |
| Final corrected hook/page matrix | `3fc3f4` | Exit 0; 17 cases pass, including all ten hook guards and seven page cases |
| Final scoped lint / formatting | `32cefa`, `7abc07` | Exit 0; four affected code/test paths pass after fixture-only lint corrections |
| Final `pnpm run typecheck` | `df4879` | Exit 0; current code and tests |

The hook compares strict parser epoch-nanosecond results, omitting invalid recency
rather than comparing Date-normalized inputs. Partial creation remaps against the
actual published profile; only real changed draft fields override it, so genuine
in-flight edits coexist with the current live model. Original ordinary-save draft
merging remains the default. Current known owned check groups are absent; original
hosted observer and its deadline remain retained through the correction. Final
documentation checks, normal hooks, corrective publication and hosted disposition
receipts are retained in the live task plan.

## Delivery constraints and risks

All validators, later installation/tests/lint/typecheck/build/hooks need ROOT's
global local-heavy lease. The later implementation release grants one exclusive
lease through affected checks, normal hooks, ordinary commit, push and PR creation;
return it after actual local joins/current known owned cleanup, then continue hosted
in the same turn without another ROOT publication checkpoint.
Use installed mise Node24; no install for docs. Later,
install workspace dependencies once only if absent. Retain original execution
handles, actual terminal joins and known owned cleanup. Preserve shared caches,
volumes, dependencies, foreign references/processes and sibling105/PR4390.

Later authorized delivery uses the exact work-order checks and active ordinary
commit hooks, push and PR template via body file. Continue hosted handling in the
same turn after returning the local lease. Retain one 90-minute `pr-await` observer
within its original clock; feedback while CI runs, batch valid findings before one
corrective push. ROOT alone grants separate serial expected-head merge authority.
Implementation authority was explicitly granted; merge authority remains a separate ROOT grant.

The main risks are treating captured profiles as accepted changes, changing partial
draft/error behavior, or proving only a mocked callback. Recheck moving base and
PR4390 without folding its save repair into this work. Public docs need no change:
workflow, terminology, UI copy, commands and API contracts stay the same.
