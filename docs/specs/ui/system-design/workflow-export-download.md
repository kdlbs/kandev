---
status: current
system: ui
requirements:
  - REQ-UI-WORKFLOW-EXPORT-DOWNLOAD-001
---

# Workflow Export Download System Design

## Purpose and boundaries

UI owns the local download of an already presented string, independently of
its source. Tasks owns workflow content and its export/import contracts. The
[requirements](../requirements/workflow-export-download.md) define the outcome;
[clipboard feedback](clipboard-feedback.md) remains independent.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `.1`, `.2` | [Data flow](#data-flow) |
| `.3` | [Failure](#failure) |
| `.4`, `.5` | [Presentation](#presentation) |

## Data flow

Both single export and Export All supply `content` to the existing
`WorkflowExportDialog`. At activation, pass the current prop directly to
`triggerFileDownload` in `apps/web/lib/utils/file-download.ts` as
`{ fileName: "workflows.yaml", content, isBinary: false }`. The helper owns
Blob creation and browser/native lifetime. Use `content.trim()` only to decide
disabled state. Do not parse, normalize, add a BOM, derive the filename, cache a
stale value, request another export, or modify the shared helper. The fixed
filename is program data; add a reasoned i18n exemption if the scanner needs it.

## Failure

Catch synchronous helper failure and call the existing toast with
`task:failedToDownloadFile` and default variant. The error variant reports
telemetry, so it is inappropriate for this local action. Preserve dialog and
content for retry. Reuse `task:download` at render time; no new locale keys.

## Presentation

Keep the existing dialog, textarea, focus, navigation, scroll owners and
Copy/Close behavior. Add a visible semantic Download button alongside them;
a small separate button component is acceptable if useful, without a shared
helper or hook refactor. Follow `settingsActionClassName` geometry and the
existing Workflows settings actions as the nearest shipped control exemplar;
the mobile language's existing dialog primitive supplies the surface. The
short preview and temporary action justify keeping the current dialog instead
of a new drawer or navigation. On phones the footer may stack or wrap while
keeping all controls visible and contained. Cap the dialog to the dynamic viewport and let its preview row shrink and
scroll internally, keeping header and footer within reach on short phones.
Preserve modal focus and dismissal, safe-area handling, and document width. Ordinary
fine-pointer desktop actions match existing settings density; phone width and
coarse-pointer conditions independently provide at least 44 px hit areas,
including a narrow fine-pointer phone. Compare actual rendered bounds, not
classes alone. Localized text must fit without clipping.

## Persistence, security, and observability

The action downloads only the in-memory displayed text upon user activation.
It adds no persistence, backend call, permissions, logs, or metrics. Existing
workflow export and import trust and data boundaries remain unchanged.

## Related records

- [Clipboard feedback](clipboard-feedback.md).
- [Workflow import profile selection](../../tasks/requirements/workflow-import-profile-selection.md).
- [Delivery plan](../../../plans/workflow-export-download/plan.md).
