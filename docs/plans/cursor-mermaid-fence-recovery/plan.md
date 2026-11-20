---
created: 2026-11-17
status: done
requirements:
  - REQ-UI-MERMAID-RENDERING-002
system_design:
  - ../../specs/ui/system-design/mermaid-markdown-boundaries.md
legacy_specs: []
---

# Implementation Plan: Cursor Mermaid fence recovery

## Overview

Recover the diagram and following prose in [issue #5362](https://github.com/kdlbs/kandev/issues/5362).
Keep the correction in three existing frontend files and one sequential work order.
Implementation is complete.

[Requirement 002](../../specs/ui/requirements/mermaid-rendering.md#req-ui-mermaid-rendering-002-markdown-diagram-boundaries) owns the observable outcome.
The [system design](../../specs/ui/system-design/mermaid-markdown-boundaries.md) defines the small rendering correction.

## Scope

### In scope

- Detect an apparent closing backtick run glued to prose inside an explicitly tagged Mermaid code block.
- Split only after the full source fails syntax validation and the diagram prefix succeeds.
- Preserve valid diagrams, stored text, existing Markdown context, and the current render lifecycle.
- Prove recovery on desktop, phone, and reload.

### Out of scope

- Provider reply reconstruction, separators between chunks, SQL projection, and stored-message rewrites.
- Standalone parser-plugin modules, fence-position metadata, recovery components, timers, caches, or dependencies.
- Untagged diagram recovery, task-plan rich-text changes, and arbitrary Mermaid syntax repair.
- New controls, styling, copy, settings, or public APIs.

## Confirmed root cause

The issue reports consecutive Cursor replies without message IDs or an intervening boundary event.
The first ends with closing backticks. The second starts with prose.
ID-less chunks concatenate into one string, with backticks immediately followed by prose on the same line.

That line does not close the Markdown fence.
The following paragraph becomes diagram code, which Mermaid rejects.
The existing normalizer does not repair this form.

### Smallest reproduction

````markdown
```mermaid
flowchart TB
  A --> B
```Following paragraph.
````

The glued input produces one invalid Mermaid code node.
A newline after the final backticks produces a valid diagram and a separate paragraph.

This investigation reproduced the parser failure and traced current source.
It did not inspect the private incident databases or reproduce a live Cursor session.

### Source evidence

Local baseline: `17a18bc2de5713b80058d730cfc87e8e30e5aa5c`.
The four relevant renderer/normalizer files and two requirement files matched remote main `75972b7e19fdaf80bedc3b76e20dfa25be7e19aa` during investigation.

- `convertACPContentNotification` preserves optional provider message IDs.
- `canonicalAgentMessageID` groups ID-less text by known prompt/turn/submission identity.
- `canonicalDeliverySegmentMessageID` preserves the supported segmentation boundary.
- `canonicalDeliveryBatchContent` appends chunks without extra whitespace.
- `normalizeMarkdown` leaves this leading-fence-plus-prose shape unchanged.
- `MarkdownCode` passes the resulting code node to `MermaidBlock`.

[Platform segmentation](../../specs/platform/requirements/durable-agent-text-segmentation.md) requires existing ID-less concatenation.
The rendering requirement lacked this recovery case, so the package adds requirement 002.
The existing [semicolon package](../mermaid-sequence-semicolon-rendering/plan.md) remains unchanged.

## Technical approach

1. In `mermaid-utils.ts`, add a small helper that selects the original source or a verified diagram prefix plus following Markdown.
2. In `mermaid-block.tsx`, run that helper inside the existing debounced render operation.
3. In `markdown-components.tsx`, add the `pre` override and trailing-Markdown callback so recovered prose appears outside the diagram.

After the full source fails, the helper checks suspicious backtick lines in source order and selects the first prefix that validates.
Valid complete source always wins. If no candidate prefix validates or a probe unexpectedly rejects, the original source retains its existing path.
Use the installed Mermaid parser and existing sanitizer/configuration.

The initial draft proposed outer-fence metadata, a parser plugin module, and a separate recovery component.
Those pieces were removed to keep boundary selection inside the existing render lifecycle.
The PR review follow-up adds a small transform to the shared remark pipeline in `markdown-components.tsx` so recovered prose retains reference definitions from the owning Markdown document.
This transform does not select boundaries or inspect outer-fence metadata.

### Provider compatibility

| Input shape | Result |
| --- | --- |
| Cursor ACP with the reported ID-less stream | Keep concatenation and recover eligible rendered content |
| Providers with message IDs or supported boundaries | Keep existing identity and separate-message behavior |
| Another provider with the same malformed Mermaid code | Apply the same verified rendering recovery |
| Valid Mermaid, ordinary text, other languages, or a literal wrapper | Preserve existing content and behavior |

No provider-specific branch is needed.

## ASCII UI preview

### UI-01: Chat diagram followed by prose

Desktop and phone share this content composition:

```text
[Diagram viewport]
  A -> B

Following paragraph.
```

Before recovery, the paragraph is inside the failed diagram's code node.
After recovery, it is a Markdown sibling outside the diagram and `pre`.
This structure satisfies AC-002.1, AC-002.5, and AC-002.7.
Spacing and sample copy are illustrative.
Keep the transcript scroll owner and the existing responsive diagram viewport.

## Tests

| Acceptance | Evidence |
| --- | --- |
| `AC-UI-MERMAID-RENDERING-002.1` | Real-parser tests in `mermaid-utils.test.ts` and the named component regression |
| `AC-UI-MERMAID-RENDERING-002.2` | Valid multiline labels containing backticks remain intact |
| `AC-UI-MERMAID-RENDERING-002.3` | Other languages, wrappers, no-candidate content, and invalid prefixes retain their path |
| `AC-UI-MERMAID-RENDERING-002.4` | Browser reads verify exact fixture content before and after reload |
| `AC-UI-MERMAID-RENDERING-002.5` | Existing desktop/mobile specs gain the recovery and reload case |
| `AC-UI-MERMAID-RENDERING-002.6` | Component checks reuse the existing cancellation and toast path |
| `AC-UI-MERMAID-RENDERING-002.7` | Component checks prove sibling placement, document references, and current task/file-link context |

The named regression `recovers prose glued to a closing Mermaid fence` in `markdown-mermaid-recovery.test.tsx` failed against the original renderer and passes with the implementation.
Keep real-parser preservation evidence rather than relying only on a mocked Mermaid API.

## E2E tests

Extend `e2e/tests/chat/mermaid-fit.spec.ts` and `mobile-mermaid-fit.spec.ts`.
Reuse their helper and isolated fixtures. Allow the seed helper to accept message content while retaining its wide-diagram default.
Its only callers are these two specs.

Each project checks a real SVG, ordinary prose, reload behavior, and unchanged stored content.
Use `listSessionMessages` for the saved-content assertion.
Retain the existing ordinary-diagram fit cases.
Desktop uses `chromium`. Phone uses the configured `mobile-chrome` device.
The managed desktop run builds current assets. The phone run can reuse them without intervening source changes.

## Work orders

- [x] [Task 01: Recover glued Mermaid boundaries in chat](task-01-recover-mermaid-boundaries.md) (`done`).

One sequential task owns the correction and targeted checks. Delegation is not authorized.

## Verification results

### Design evidence

- Initial parser reproduction and source trace: confirmed.
- Simplified source-selection probe: 14/14 expected decisions with installed parsers.
- The probe covers LF/CRLF/CR, indentation, longer marker runs, normal diagrams, wrappers, valid labels, invalid prefixes, and partial markers.
- Invalid full source with a real outer closer or a longer opener also recovers after prefix validation.
- All probes ran through stdin. No temporary source or permanent test was added.
- Catalog validation, full specification lint, local links, whitespace checks, and the coverage preflight passed for this revision.
- Issue #5362 remains assigned to `carlosflorencio`.

The first draft's 13-case metadata-based probe is superseded by this 14-case source-selection probe.
Do not reuse its stricter outer-fence eligibility expectations.

### Implementation evidence

- Initial implementation verification: the component and desktop/phone recovery cases passed after confirmed RED runs. Focused Vitest passed 5 files and 94 tests; Oxlint, typecheck, `i18n:ratchet`, Prettier, documentation checks, managed desktop build, and both browser projects passed.
- Static-review follow-up: the file-preview routing regression and five false-candidate fence cases failed before correction. The focused suite then passed 3 files and 92 tests, covering real Mermaid parsing, file-preview routing, and chat recovery.
- Follow-up Oxlint passed for the changed frontend files, Prettier passed for all nine changed TypeScript/TSX files, and `git diff --check` passed.
- The broad build, i18n, and desktop/phone browser projects were not rerun for the initial static-review corrections; their earlier results apply to that revision. Frontend typecheck was rerun after the PR review follow-up and passed below.
- PR review follow-up: the reference-definition and latest-code-view regressions failed before correction. The final focused Mermaid suite passed 3 files and 94 tests; frontend typecheck, focused Oxlint, Prettier, and `git diff --check` passed.
- Additional PR review follow-up: the multiline-label candidate and Mermaid initialization-order regressions failed before correction. The focused suite then passed 3 files and 95 tests, including validation of a later true closing candidate after an apparent label candidate.

## Risks

- A string-only split can corrupt valid labels. Keep both syntax checks.
- Async results can arrive after input changes. Reuse the existing publication guard and bind recovered prose to its source/task.
- Moving explicit Mermaid blocks outside `pre` affects composition. The ordinary fit cases and full-renderer test protect it.

## Documentation impact

Internal docs only. This correction adds no public authoring syntax, command, setting, or terminology.
