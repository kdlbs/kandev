---
id: "08-editor-layout"
title: "Organize the automation editor"
status: done
wave: 4
depends_on: ["03-per-pr-continuation"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-002
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006
  - REQ-OFFICE-AUTOMATION-CONTINUITY-001
  - REQ-OFFICE-AUTOMATION-RETRIES-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.1
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.2
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.3
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.4
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.5
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.6
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.7
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.4
  - AC-OFFICE-AUTOMATION-CONTINUITY-001.6
  - AC-OFFICE-AUTOMATION-RETRIES-001.8
system_design:
  - ../../specs/office/system-design/automations-settings-04.md
  - ../../specs/office/system-design/automations-pr-follow-up.md
  - ../../specs/office/system-design/automation-retries.md
---

# Task 08: Organize the automation editor

## Summary

Selecting **One task per pull request** revealed a follow-up field far above
the choice, often off screen. Move every choice above the fields it affects,
replace stacked radio cards with compact single choices, show placeholders
once, group agent tools, and correct copy that was untrue in some setups.

The second round, after manual testing, regroups the page by decision order:
**Where runs go** (destination and workflow), **Repeat runs** (task for each
run and concurrency), **Instructions**, **Agent** (profiles, repositories, and
a collapsed **Tools** row), and **If a run fails**. It also keeps **No
schedule** available after a schedule is chosen and gives the selected option
a fill and outline that read in both themes.

## In scope

- `SegmentedChoice`, `TargetModeSection`, `WorkflowField`,
  `AgentToolsDisclosure`, and the section composition in
  `automation-editor-groups.tsx`.
- Continuation, retry, prompt, schedule (including clearing), and
  trigger-summary changes.
- Renamed labels in every catalog, the specs, and the public guide.
- Desktop and phone E2E updates.

## Out of scope

- Whether settings pages use bordered cards or full-width bands. That is a
  shared design decision for every settings page.

## Acceptance

1. Component tests cover the choice semantics, section options and
   descriptions, the placeholder disclosure, schedule wording, and summaries.
2. Desktop and phone E2E scenarios cover section order, the workflow above the
   step picker, the selected-option fill in both themes, schedule clearing,
   stacked touch-sized options, the shared-task lock, retry details, the Tools
   disclosure, and the follow-up position.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/automations)
(cd apps/web && pnpm run i18n:check && pnpm run lint)
(cd apps/web && pnpm e2e:run --host --project chromium tests/automations-settings.spec.ts tests/automations-pr-follow-up.spec.ts tests/automations-mcp-selection.spec.ts tests/automations-plugin-tools.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/mobile-automations-scroll.spec.ts tests/settings/mobile-automations-settings.spec.ts tests/mobile-automations-pr-follow-up.spec.ts tests/plugins/mobile-managed-automation.spec.ts tests/settings/mobile-automations-mcp-selection.spec.ts tests/settings/mobile-automation-plugin-tools.spec.ts)
```

## Parallelism

`sequential`

## Results

Done on 2026-11-25.

- Desktop automation E2E (8 specs, chromium): 61 passed.
- Phone automation E2E (6 specs, mobile-chrome): 14 passed.
- `pnpm run i18n:check`, `pnpm run lint:i18n`, and `pnpm run i18n:ratchet`: clean.

Second round, done on 2026-11-25:

- Desktop automation E2E (8 specs, chromium): 63 passed.
- Phone automation E2E (7 specs, mobile-chrome): 15 passed.
- Full web Vitest: 32097 passed; the 3 failures are the known host-environment
  cases in `e2e/helpers/seed-repository-origin.test.ts`.
- The selected-option style keys on `aria-checked`: the tooltip trigger
  replaces the item's `data-state`, which is why the first round's selected
  style never showed.
- The PR follow-up save helpers waited for the relabelled save button to
  disappear and could reload mid-save. They now wait for the saved status.

Third round, after a cross-provider review, done on 2026-11-25:

- Switching to a managed conversation returns **Task for each run** to a new
  task; the save payload never sends a managed conversation's hidden workflow
  or per-PR options. Reselecting the current workflow keeps the chosen steps.
- Segmented choices are a Radix radio group, so the arrow keys move the
  selection. The Tools row opens on each new problem but still closes, says
  when MCP choices cannot load, and marks the Agent card changed for tool
  edits. GitHub PR event rows stay 44 px on tablets and coarse pointers, with
  help in a separate `SettingsInfo` button. The task title is labelled.
- Backend: review-request generations are reserved before dispatch, an
  `opened`-only trigger reads observations, and `keep` waits for an idle task.
  A new automation saves the **Enabled** state it shows.
- Desktop automation E2E (9 specs, chromium): 66 passed.
- Phone automation E2E (7 specs, mobile-chrome): 15 passed.

Deviation: the mockup's lowercase trigger summary uses the localized event
labels as written ("PR: Opened, New commits, Review requested"), because
lowercasing is not locale-safe. Tooltips are disabled on coarse pointers,
where a tap selects and the description line already explains the choice.
