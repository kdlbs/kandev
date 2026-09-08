---
status: draft
system: integrations
created: 2026-08-11
owners:
  - tbd
---
# GitLab MR Status Chip Requirements

## Overview

A task whose work lives on GitHub shows its pull request's CI, review and automation state as a chip in the chat status bar, so the user sees it without leaving the conversation. A task whose work lives on GitLab shows nothing there. GitLab users have to look up at the topbar button, or open the MR detail panel, to answer "did my pipeline pass". The GitLab merge-request equivalent of that chip does not exist.

## Requirements

### REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001: GitLab MR Status Chip

**Intent:** A task whose work lives on GitHub shows its pull request's CI, review and automation state as a chip in the chat status bar, so the user sees it without leaving the conversation. A task whose work lives on GitLab shows nothing there. GitLab users have to look up at the topbar button, or open the MR detail panel, to answer "did my pipeline pass". The GitLab merge-request equivalent of that chip does not exist.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001.1:** A task with at least one **open** linked GitLab merge request SHALL render an `MRStatusChip` in the chat status bar and in the passthrough status row, beside the existing GitHub and Azure DevOps chips.
- **AC-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001.2:** The chip SHALL show, at a glance: a GitLab merge glyph, a status glyph whose colour matches the MR's existing status colour everywhere else in the product, an MR count when more than one open MR is linked, and auto-fix / auto-merge badges when task MR automation is enabled.
- **AC-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001.3:** Hovering the chip on a fine-pointer device SHALL open the existing `MRCIPopover` body (pass rate, pipeline stage groups, approval row, unresolved discussions, automation controls, merge action, footer). Tapping it on a coarse-pointer device SHALL open that same body inside a bottom-sheet drawer.
- **AC-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001.4:** The chip SHALL derive its status from the one shared GitLab MR status derivation, not a new private copy. `getMRStatusColor` in `apps/web/components/gitlab/mr-task-icon.tsx` remains the single source of MR status colour, and its output for every input SHALL be byte-identical before and after this feature.
- **AC-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001.5:** The chip SHALL NOT own any recurring fetch, polling, cache-warming or background-sync responsibility. It issues exactly **four** kinds of request, all of them existing behaviour it inherits rather than invents: 1. the lazy read of the task's MR automation options, on mount; 2. the GitLab connection-status read that `useGitLabAvailable()` performs on mount, which the chip needs to source `canLink`. This one has a cross-surface side effect and is stated in full under Sync and freshness; 3. the MR feedback read that `MRCIPopover` already owns, **only while a disclosure is open** (this is what the popover's `enabled` prop gates; see API surface); 4. the link and unlink the user explicitly triggers. Nothing else: no polling, no interval, no warmer, and specifically no `useWorkspaceMRs`. See Sync and freshness.
- **AC-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001.6:** No file under `apps/web/components/github/` changes. `PRStatusChip`'s rendered output SHALL be identical before and after.
- **AC-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001.7:** All chip copy SHALL go through `t()` into `apps/web/src/locales/en/gitlab.json`, including accessible labels.
- **AC-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001.8:** Disclosure is chosen by pointer precision only, via `useTouchDrawer()` (`!isFinePointer`): coarse pointer renders the `Drawer` variant, fine pointer renders the hover `Popover` variant.

## System design

The migrated technical source is split into [part 1](../system-design/gitlab-mr-status-chip-01.md), [part 2](../system-design/gitlab-mr-status-chip-02.md), [part 3](../system-design/gitlab-mr-status-chip-03.md), [part 4](../system-design/gitlab-mr-status-chip-04.md), [part 5](../system-design/gitlab-mr-status-chip-05.md).
