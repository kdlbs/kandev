---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001
created: 2026-08-11
owners:
  - tbd
---
# GitLab MR Status Chip System Design Part 5

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Constraints

- New files SHALL be at most 600 lines and new components at most 200 lines.
  The GitHub original is 634 lines in one file and is not a shape to copy;
  the trigger, the glyph, the disclosure variants and the status derivation are
  separate units.
- New copy SHALL use a `mrChip*` key prefix in
  `apps/web/src/locales/en/gitlab.json`, following the same surface-scoped
  prefix convention as the existing `mrPopover*` and `mrAutomation*` keys.
  Copy that already exists under `mrPopover*` SHALL be reused rather than
  duplicated. `data-testid` values and `data-status` values are identifiers and
  SHALL NOT be translated.
- Every new file SHALL be appended to `i18nGuardFiles` in
  `apps/web/eslint.i18n.options.mjs` in the same change.
- E2E coverage is required. The page object SHALL mirror the shape of
  `apps/web/e2e/pages/session-page.ts` `tapPRStatusChip`, exposing
  `mrStatusChip()`, `mrStatusChipDrawer()`, `mrStatusChipDrawerClose()`,
  `mrStatusChipPopoverInner()` and `tapMRStatusChip()`.

  Mirroring the shape means mirroring its **scoping**, not just its arity.
  `prStatusChip()` is not a bare `getByTestId`; it resolves
  `activeChat()` -> `chat-status-bar` -> `pr-status-chip`, which is what keeps it
  from tripping Playwright's strict-locator check when more than one status row
  is mounted. `mrStatusChip()` SHALL do the same for `chat-status-bar`. Because
  this feature also has a scenario against the passthrough row, the page object
  SHALL additionally expose **`mrStatusChipInPassthrough()`**, scoped to
  `passthrough-status-row`. Without it the mandated zero-argument set can reach
  only one of the two surfaces this spec requires coverage of. No accessor may
  resolve `mr-status-chip` globally or paper over duplicates with `.first()`.
- Unit coverage SHALL include the `getMRStatusColor` and
  `aggregateMRStatusColor` parity tables described above, updating
  `mr-task-icon.test.ts` and `mr-status-colour-parity.test.tsx`. The
  `aggregateMRStatusColor` table SHALL include at least one all-terminal pair in
  both input orders, so the preserved array-order tie is pinned rather than
  assumed.
- **File boundary.** The rule below governs **production React source only**. It
  does not and cannot restrict the supporting files this same section mandates:
  the locale catalog `apps/web/src/locales/en/gitlab.json`, the guard list
  `apps/web/eslint.i18n.options.mjs`, the page object
  `apps/web/e2e/pages/session-page.ts`, new specs under `apps/web/e2e/`, and the
  unit tests named above are all required edits and all sit outside
  `components/gitlab/`. An earlier draft stated the rule without that carve-out
  and so contradicted its own i18n and E2E requirements.

  Within production React source: the one permitted change outside
  `components/gitlab/` and the two mount points is extracting `useUnlinkTaskMR`
  into `apps/web/hooks/domains/gitlab/use-task-mr.ts` and pointing
  `MRTopbarButton` at it. `MRTopbarButton`'s rendered output and observable
  behaviour SHALL be unchanged, and `components/github/` SHALL NOT be touched at
  all. `components/azure-devops/` SHALL NOT be touched either — which is why the
  Azure leg of the DOM ordering has no AC.

## Related

- [gitlab-integration](../requirements/gitlab-integration.md): the umbrella GitLab
  feature this chip sits inside.
- `apps/web/CLAUDE.md`, "GitHub PR status UI": the GitHub-side invariant this
  feature's single-derivation requirement mirrors for GitLab.
