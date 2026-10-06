---
status: current
system: ui
requirements:
  - REQ-UI-CLIPBOARD-FEEDBACK-001
---

# Clipboard Feedback System Design

## Purpose and boundaries

UI owns reusable feedback timing shared by unrelated copy consumers.
`useCopyToClipboard` in `apps/web/hooks/use-copy-to-clipboard.ts` owns each
instance's `copied` state and scheduled expiration. `copyToClipboard` in
`apps/web/lib/utils/copy-to-clipboard.ts` owns the modern API and DOM fallback.
The utility and consumers need no production changes.

The path-specific
[copy action design](copy-file-path-actions.md) owns path content and refusal;
[Auth's browser fallback requirement](../../auth/requirements/secure-context-browser-fallbacks.md)
continues to own transport capability behavior. Neither is the reusable
feedback lifecycle owner.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-UI-CLIPBOARD-FEEDBACK-001.1`, `001.2`, `001.3`, `001.5` | [Timer ownership and completion](#timer-ownership-and-completion) |
| `AC-UI-CLIPBOARD-FEEDBACK-001.4` | [Failure behavior](#failure-behavior) |
| `AC-UI-CLIPBOARD-FEEDBACK-001.6` | [Cleanup boundary](#cleanup-boundary) |
| `AC-UI-CLIPBOARD-FEEDBACK-001.7` | [Consumer boundary](#consumer-boundary) |

## Timer ownership and completion

Keep the public `{ copied, copy }` shape, `copy(text)` promise behavior,
default duration of 2000 ms, and callback dependency on `duration`.
Store the scheduled timeout handle in one instance-local ref, typed as
`ReturnType<typeof setTimeout> | null`. Check against `null` explicitly.

Only after the real utility resolves `true`, clear this instance's existing
timeout, set copied state to true, and schedule its replacement using the
duration captured by the completing callback. Expiration clears the ref and
copied state. Renewal must happen even when copied state is already true;
an effect driven only by that boolean cannot establish a new deadline.

Rerendering with a different duration neither cancels nor reschedules the
existing timer. A retained callback uses its captured duration when it later
completes successfully. Do not substitute the latest render's duration through
a ref or introduce a request counter. Completion order remains authoritative.
Zero duration continues to use normal asynchronous timeout semantics.

## Failure behavior

Preserve the existing failure log and retry behavior. A false utility result
does not touch the scheduled timer or copied state. Native rejection may still
be a success when the actual DOM fallback succeeds; the hook uses the utility's
final boolean outcome.

## Cleanup boundary

An unmount cleanup effect with stable dependencies clears the timer currently
owned by the ref and releases its handle. It must read the current handle,
including a replacement scheduled after earlier successes. Other hook instances
have independent refs and timers.

This design does not add a mounted flag, generation, abort mechanism, or
in-flight admission policy. A write already awaiting acknowledgement can still
complete after cleanup and schedule work through the existing async callback.
That residual is outside the scheduled-timer repair; no universal mount-lifetime
guarantee is asserted.

## Consumer boundary

`WorkflowExportDialog` in
`apps/web/components/settings/workflow-export-dialog.tsx` supplies a compact
rendered integration boundary: its real Radix dialog renders the export content
and a button using `workflows:copy` and `workflows:copied`. Test its real hook,
utility, translations and dialog primitives without mocking them. Verify exact
native write values and real visible button/content outcomes before and after
both deadlines. A fallback integration can inspect the actual selected textarea
inside the dialog while substituting only `document.execCommand`.

CodeMirror, Monaco, Shiki, chat, file browser and settings already consume this
hook; no caller migration is required. This is shared state timing within
existing controls. Under the mobile parity state/data exception, targeted hook
and rendered component evidence suffice; no layout, touch, scrolling,
navigation, viewport behavior, or Playwright change is needed.

## Validation boundary

The work order owns direct timing controls and rendered integration tests using
only native clipboard substitutions and fake time. Existing native, fallback,
textarea-removal and focus-restoration controls remain in the affected suites.
No source-text assertion, mocked hook success, or implementation predicate
counts as rendered evidence.

There is no new persistence, permission, backend API, metric, or log contract.
Local timer management needs no ADR.

## Implementation plan

- [Clipboard success feedback](../../../plans/clipboard-success-feedback/plan.md)
