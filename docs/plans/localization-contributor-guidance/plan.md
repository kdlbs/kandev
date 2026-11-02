---
created: 2026-10-28
status: implemented
requirements:
  - REQ-PLATFORM-I18N-001
system_design:
  - ../../specs/platform/system-design/i18n.md
legacy_specs: []
---

# Implementation Plan: Localization Contributor Guidance

## Overview

Correct the contributor instructions to describe the existing localization
checks and make their commands runnable from the repository root. One sequential
work order covers both documents so their current guidance agrees.

## Scope

### In scope

- Current contributor instructions in `docs/public/web-development.md` and
  `docs/i18n.md`, aligned with the platform i18n contract and current scripts.
- Root-based command examples, required catalog updates, and the distinction
  between gates and advisory reports.

### Out of scope

- Changing locale policy, source, catalogs, scripts, CI, hooks, or UI.
- Rewriting historical migration and audit narratives or unrelated guide sections.
- Browser tests, screenshots, and new permanent tests for documentation wording.

## Technical approach

The public web development page is a contributor how-to; the internal i18n
guide provides detailed instructions and history. Use the web package scripts
as command authority and the key and non-JSX check implementations as behavior
authority. Keep advisory reporting separate from the mandatory gate, including
the reports' failure on unreadable inputs. Follow the existing platform i18n
requirement and design, including the generated Traditional Chinese workflow.

## Tests

AC-PLATFORM-I18N-001.9 and .10 are verified by inspecting both revised guidance
sections against the source and executing the documented root commands, plus
the public documentation and specification validators specified in Task 01.
No production logic changes or permanent tests are needed. No rendered product
UI changes, so a browser E2E or capture is not applicable.

## Work orders

- [x] [Task 01: Correct localization guidance](task-01-correct-localization-guidance.md)

## Verification results

Task 01 completed. The frozen install, documented non-mutating localization
commands, public documentation validators, catalog validation, specification
lint, and diff checks passed. See the work order for actual results and logs.

## Risks

- Historical explanations in the internal guide can be mistaken for current
  instructions; preserve history while making current directions unambiguous.
- Advisory reports can fail for unreadable inputs; their non-gating findings do
  not make the separate catalog gate advisory.
