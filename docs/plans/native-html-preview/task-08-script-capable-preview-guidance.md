---
id: "08-script-capable-preview-guidance"
title: "Publish script-capable preview guidance"
status: cancelled
wave: 4
depends_on:
  - "07-responsive-preview-surfaces"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.9
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 08: Publish script-capable preview guidance

## Summary

Update the Files and editor guide after the runtime-backed behavior is shipped.
Explain how inline scripts work inside the restricted preview runtime and make
the resource, browser-API, navigation, and multi-file limitations explicit.

## In scope

- Replace the static-only preview description in
  `docs/public/developer-tools.md`.
- Explain current-buffer rendering, unsaved edits, `Show code`, embedded
  resources, isolated inline scripts, and blocked remote/workspace resources.
- Explain when a development server and Browser panel are required.
- Run public-doc validators and the docs diff check.

## Out of scope

- Architecture rationale, VM package details, or internal message shapes.
- Public claims before the browser and WebView evidence is complete.
- New navigation entries or a separate public page.

## Acceptance

- A user can preview an eligible file and understands that inline scripts run
  in a restricted runtime without Kandev credentials or network access.
- The guide does not describe the preview as a general site server or promise
  unsupported browser APIs and external resources.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public
```

## Files likely touched

- `docs/public/developer-tools.md`

## Dependencies

Task 07 supplies final user-facing labels and runtime behavior.

## Risks

Publishing a security claim before browser and WebView tests pass can create a
false guarantee for users.

## Parallelism

`parallel-safe` with Task 09 after Task 07.

## Inputs

- The final requirements and public behavior from the system design.
- The existing Files and editor integrations guide.
- `/docs-maintainer` validation rules.

## Results

Updated `docs/public/developer-tools.md` with the restricted runtime contract,
current-buffer and unsaved-edit behavior, embedded-resource policy, blocked
browser capabilities, navigation behavior, source recovery, and Browser-panel
guidance for multi-file applications.

Verification completed:

```text
node --test scripts/validate-public-docs.test.mjs: 61 passed
node scripts/validate-public-docs.mjs: Validated 43 published docs pages.
git diff --check -- docs/public: passed
```
