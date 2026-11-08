---
status: current
system: ui
requirements:
  - REQ-UI-EXTERNAL-VCS-FILE-LINKS-001
---

# External VCS File Links System Design

## Ownership and boundaries

UI owns the existing reusable file-navigation contract across task editors,
Changes, and Review. Provider associations remain integration-owned and task
repository/worktree identity remains workspace-owned. This design consumes their
existing projections without changing persistence, hydration APIs, or provider
provenance. It pairs the [owning requirement](../requirements/external-vcs-file-links.md).

## Components and data flow

`useExternalVcsFileLinkHydration` enables the existing provider loaders once per
task. `useExternalVcsFileLink` reads `StateProvider` stores, identifies the exact
linked repository and worktree branch, resolves its task-repository base, selects
the published revision, then calls `resolveExternalVcsFileURL`.

`ExternalVcsFileLink` and `ExternalVcsFileMenuItem` consume the same hook and render
semantic new-tab anchors. Existing localized provider names, safe relationship
attributes, and compact/touch presentation remain shared. No new fetching,
state mutation, metrics, or logging is needed for this local derivation.

## Repository and revision selection

Keep the current repository resolution order: explicit linked repository ID,
named session worktree/repository, then unambiguous session/task identity.
Repeated repository rows require the active worktree branch to select exactly
one task-repository link. Preserve exact provider/repository matching and the
legacy GitHub owner/name and GitLab host/project guards.

Published candidates retain **source branch identity separately from target
revision**. The source branch continues to select the active worktree's review;
conversion to a commit must not remove that match or select a sibling's SHA.
Select the source branch using the existing unique-branch rules first: an active
branch match wins; otherwise repeated repository rows forbid sibling fallback,
and a single repository row may use one unique published branch.

An explicit `UseExternalVcsFileLinkInput.publishedBranch` remains authoritative.
For inferred GitHub candidates, open or unmerged closed rows target `head_branch`;
merged rows target the trimmed, nonempty `TaskPR.head_sha`. A merged row without
that field contributes no usable published target, while retaining its branch
identity for ambiguity checks. For a selected branch, identical targets can be
deduplicated; multiple distinct targets or a mixture of usable and missing
targets fail closed to the ordinary base/path fallback. Never pick by array
order. GitLab and Azure DevOps continue using their existing branch targets.

Pass only the selected target into the existing low-level `publishedBranch`
slot, which already accepts a Git revision. Keep this internal compatibility
name; no public input expansion or backend schema is required. The URL helper
remains the owner of credential-free repository URLs, encoding, safe refs/paths,
and status-dependent path selection.

## File status and fallback

| File context | With merged GitHub head SHA | Without a usable published target |
| --- | --- | --- |
| Existing/modified | Current path at saved head SHA | Current path at task/input base |
| Added/untracked | Current path at saved head SHA | Omit action |
| Renamed | New path at saved head SHA | Known previous path at base, otherwise omit |
| Deleted | Existing deleted-file base rule | Existing deleted-file base rule |

Base fallback is best effort under the pre-existing contract; it does not prove
that a moving base still contains an old deleted or renamed path. This repair
does not redesign deleted-file history, publish local files, or promise that
unsaved edits exist remotely. Missing base/path evidence yields no action.

The head snapshot is intentional: merging can squash/rebase commits and the base
can move afterward, while the file action belongs to the task's reviewed source
content. Do not substitute merge commits, PR pseudo-refs, a moving base, or a
fork URL in place of available head evidence. Existing open-PR behavior when
fork provenance is unavailable is unchanged.

## Desktop, phone, and security

The same target flows into desktop `sm` and phone `touch` anchors. The closest
shipped phone surface is `MobileFileViewerPanel` with its directly reachable
`size="touch"` action. Shared status/revision normalization changes no layout,
navigation mechanism, scrolling, safe-area owner, or hitbox. No new copy or
translations are introduced. The mobile-parity state/data-normalization
exception is satisfied by rendered real-hook/component tests for both sizes;
new Playwright coverage is unnecessary for this bounded correction.

All existing provider/remote/path validation and credential rejection remain in
the URL helper. No network request is made to check branch existence during
render. Unsupported/ambiguous identities remain unavailable or use the existing
safe base fallback, never a different repository's target.

## Verification and traceability

| Requirement | Design sections and evidence |
| --- | --- |
| REQ-UI-EXTERNAL-VCS-FILE-LINKS-001, AC .1-.3 | Components and desktop/phone anchors; existing semantic component checks |
| AC .4, .5, .9 | Repository/revision selection; real StateProvider hook and rendered merged/open/base regressions |
| AC .6-.8 | URL helper and status table; existing utility and GitLab-host suites plus SHA/status regressions |

Independent RED/GREEN must run after implementation authorization, with no ROOT
private qualifier fixtures. Use the [repair work order](../../../plans/merged-task-file-links/task-01-merged-revisions.md)
for exact commands. The [original delivery record](../../../plans/external-vcs-file-links/plan.md)
remains historical; this correction has one new manifest and one work order.
