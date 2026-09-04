---
id: "03-public-html-preview-guidance"
title: "Publish HTML preview guidance"
status: done
wave: 3
depends_on:
  - "02-responsive-html-preview"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 03: Publish HTML preview guidance

## Summary

Document how to preview a self-contained HTML file and when the Browser panel
is still required. Keep the security and asset limitations visible in the
existing developer-tools how-to page.

## In scope

- Add a concise HTML preview subsection beside the Files and editor guidance.
- Explain current-buffer rendering, the eye action, `Show code`, and desktop and
  mobile availability.
- Explain that workspace-relative and remote assets are blocked and direct
  multi-file applications still require a development server.
- Run public-doc validators.

## Out of scope

- A new public docs page or navigation entry.
- Architecture rationale, CSP syntax, or internal component names.
- Screenshots or diagrams. The short same-surface workflow does not need one.

## Acceptance

- A user can follow the public instructions to preview an eligible file and
  return to source. The instructions do not require a save or server.
- The same section states the self-contained limitation and routes multi-file
  sites to the Browser panel workflow.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public
```

## Files likely touched

- `docs/public/developer-tools.md`

## Dependencies

- Task 02 supplies the final user-facing labels and behavior.

## Risks

- Describing HTML preview as a general site server hides the explicit
  relative-resource limitation.

## Parallelism

`parallel-safe` with Task 04 after Task 02.

## Inputs

- Acceptance criteria `.1` through `.5`.
- The scope, HTML document construction, security, and failure sections of the
  system design.
- The existing Files and editor integrations section in
  `docs/public/developer-tools.md`.

## Results

Added the self-contained HTML preview workflow and its sandbox and asset
limitations to the public developer-tools guide. Public documentation
validation passed:

```text
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public
```
