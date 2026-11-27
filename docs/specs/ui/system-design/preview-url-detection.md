---
status: current
system: ui
requirements:
  - REQ-UI-PREVIEW-URL-DETECTION-001
---

# Automatic preview URL detection system design

## Purpose and boundaries

The shared preview detector converts process-output text into a candidate URL
for existing preview presentations. This is a reusable UI selection contract,
independent of executor provisioning, proxy admission, and task feedback lifetime.
The [requirements](../requirements/preview-url-detection.md) own its observable
behavior. Manual port opening and feedback capture do not own this selection.

The correction is confined to `apps/web/lib/preview-url-detector.ts` and its
existing `preview-url-detector.test.ts`. No caller, proxy handler, API, dependency,
or runtime-lifecycle change is needed.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-PREVIEW-URL-DETECTION-001` | [Candidate validation](#candidate-validation), [Selection and consumer flow](#selection-and-consumer-flow), [Verification and mobile parity](#verification-and-mobile-parity) |

## Components and responsibilities

- `detectPreviewUrl` selects a candidate within one line.
- `tryParseFullUrl` retains native `URL` parsing and normalization.
- `tryParseHostPort` validates bare candidates before returning one.
- `detectPreviewUrlFromOutput` retains the last non-null per-line result.
- `rewritePreviewUrlForProxy` retains its existing session-scoped rewrite and
  backend-config handling. Its exported contract and implementation need no change.
- `hooks/use-preview-panel.ts` composes the output detector and rewrite.
- `components/task/browser-panel.tsx` consumes the same output detector and
  uses the rewrite for the selected local URL; explicit user URLs retain priority.

## Candidate validation

Full URL parsing already rejects numeric overflow. The defective bare fallback
has no numeric range check, and its `\d{2,5}` regex can return a prefix of a
longer token. Consequently a rejected full URL can be reaccepted by fallback.

Constrain bare matching to the complete contiguous decimal token, retaining
the existing two-to-five-digit width. A digit boundary assertion must prevent
regex backtracking from accepting a shorter prefix. Validate the complete
captured number as an integer in 0..65535 before constructing `PreviewUrlInfo`.
Do not use partial conversion such as `parseInt` on an arbitrary suffix.

Keep zero and zero-padded supported forms as existing detector behavior. Full
URLs continue through native URL parsing, including its default-port and
leading-zero normalization. Do not impose the bare width on full URLs or
introduce the backend's minimum port here. Range validity does not guarantee
rewrite eligibility or backend admission.

No generic URL parser, ANSI stripping pass, hostname rewrite, or broader
nonnumeric grammar restriction is part of this repair. Existing supported
schemes, host matching, path/query/hash behavior, and ANSI fallback remain intact.

## Selection and consumer flow

1. Examine full matches in their existing forward order. Return the first
   successful full parse.
2. If none succeeds, examine bare matches from last to first. Skip candidates
   outside the complete-token/range contract and return the first valid result
   found in that reverse traversal. Keep the existing line-context HTTPS inference.
3. Return null when neither form yields a valid selection.
4. Keep output traversal unchanged: only a non-null result replaces the previous
   selection. Invalid later lines therefore leave an earlier valid URL intact.
5. Existing consumers compose the selected URL with
   `rewritePreviewUrlForProxy`. Test this exported composition directly with
   real functions and only the existing backend-config mock.

`PreviewUrlInfo` and every exported signature remain unchanged. In particular,
valid full URLs keep native serialization and bare selections keep their current
serialization. Do not change ordering to last-full, first-bare, or last-match
across both forms.

## Failure behavior

Numeric overflow or an overlong bare token produces no candidate for that token.
Selection continues among other candidates. Invalid-only output returns null;
invalid output after a valid line retains that earlier URL. No new error message,
retry, metric, state, or persistence is introduced.

## Verification and mobile parity

Add permanent regression cases in the existing detector test file. Cover full
and bare overflow, overlong tokens, all three supported hosts, upper-bound
success, lower-bound compatibility, mixed same-line preference, and valid output
followed by invalid announcements. Compose the real output detector and real
rewrite with a test session; assert the complete retained proxy URL and the
65535 proxy URL, not only port metadata.

Existing tests supply scheme, query/path/hash, ANSI, framework-output, and
rewrite compatibility controls. Any new compatibility case belongs in the same
test file; no new test framework or caller test file is required.

The mobile-parity skill's pure state/data normalization exception applies:
the shared function has no viewport or interaction input, and no rendered surface
changes. Targeted unit tests plus the exported consumer pipeline cover the repair
on both viewports. No ASCII UI preview, rendered browser check, or new mobile
Playwright test is required for this narrow correction.

## Delivery record

- [Reject invalid preview ports plan](../../../plans/reject-invalid-preview-ports/plan.md).

This local validation correction needs no ADR: the existing parser and consumer
boundaries remain, and this design preserves the complete decision rationale.
