---
id: "04-zero-height-and-menu-title"
title: "Allow full navigation collapse and name its menu"
status: done
wave: 4
depends_on:
  - "03-navigation-split"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
  - REQ-UI-SIDEBAR-CUSTOMIZATION-007
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.5
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 04: Allow full navigation collapse and name its menu

## Scope

Lower deliberate navigation resizing and backend validation to zero pixels.
Retain zero through codecs, reload, expansion, and subsequent collapse. The
chevron and divider remain reachable; clipped entries cannot receive focus.
Add a translated Sidebar settings label and separator before context-menu choices.
Phone retains its existing Customize drawer, one scroller, and 44px controls;
phone edits must preserve saved zero desktop geometry.

## ASCII UI previews

```text
Desktop fully collapsed      Desktop context menu
            v                Sidebar settings
----------------------       --------------------
TASKS                        [x] Home
Task A                       [x] Integrations
Task B                       ...
                             Sidebar layout settings
Phone: Customize -> inset drawer (existing composition)
       Visibility and move controls; desktop height 0 stays saved.
```

## Owned files

Backend sidebar layout validation and geometry tests; frontend navigation geometry,
split component and tests; customization menu; settings locale catalogs; direct
customization desktop/phone browser tests; owning specs, tutorial, and this plan.

## Verification

- Backend navigation geometry boundary test: accept 0 and 63; reject -1/1601.
- Frontend height bounds and pointer/keyboard collapse regression tests.
- Desktop direct-customization browser cases: menu heading, zero-height pointer
  resize, reload, expand/collapse, keyboard Home, and no negative saved heights.
- Phone direct-customization browser case with saved desktop height zero.
- Focused ESLint, typecheck, i18n completeness, docs/spec validation, diff check.

## Results

Complete. Navigation can be saved at zero pixels and restored after expansion,
collapse, and reload. The desktop context menu has a localized Sidebar settings
heading in all seven supported languages and generated pseudo copy.

- Red: backend rejected zero; frontend pointer/keyboard bounds returned 64;
  desktop browser assertions failed on the missing heading and 64px saved height.
- Green: full user service package passed; 10 frontend tests in three files passed.
- Fresh managed build passed. All six desktop direct-customization browser cases
  passed with retries disabled; the phone drawer case passed while preserving
  saved zero desktop geometry.
- Existing browser checks now use a measured 90px drag target and wait for
  viewport resizing to settle before recording divider geometry.
- Typecheck, focused ESLint, i18n completeness, specification/catalog validation,
  62 public-doc validator tests, 47 public pages, and diff checks passed.

Public tutorial updated in docs/public/use-kandev.md. Delivery to the existing
PR follows the user's continuing authorization.
