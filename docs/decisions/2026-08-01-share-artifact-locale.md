# ADR-2026-08-01-share-artifact-locale: Shared-Task Artifacts Render in the Creator's Locale

**Status:** accepted
**Date:** 2026-08-01
**Area:** backend

## Context

Public task sharing publishes three files to a GitHub Gist: `share.html` (the styled
conversation, the link users actually send), `README.md`, and `snapshot.json`. The
first two are rendered by Go and, until now, held hardcoded English copy — "Untitled
task", "You", "Assistant", "Tool output", the metadata table labels, and so on.

Localizing them raises a question the rest of the product never has to answer. Every
other localized surface resolves its locale per request: the SPA reads the
`kandev_locale` cookie, and `i18n.FromRequest` does the same for the SPA-unavailable
error pages. A gist is a **static file on GitHub**. When a reader opens the link,
they make no request to Kandev — there is no cookie, no `Accept-Language`, no
request at all. The locale cannot be resolved at read time, so it must be chosen at
write time, and whatever is chosen is frozen into the published bytes.

## Decision

Shared-task artifacts render in the **share creator's locale**, resolved with
`i18n.FromRequest` in the create handler and threaded explicitly down the call
chain:

```text
httpCreate → Service.CreateShare(ctx, sessionID, locale)
           → Backend.Upload(ctx, workspaceID, snap, locale)
           → BuildShareHTML(snap, locale) / BuildGistREADME(snap, url, locale) / gistDescription(snap, locale)
```

The locale is an ordinary argument at every hop. It is deliberately not a
`context.Context` value and not package state: the builders are pure functions of
(snapshot, locale), which is what makes the per-locale render tests possible.

`i18n.Normalize` is applied where the value is emitted as `<html lang>`, so the
document's declared language and its message lookups can never disagree.

To support this the `i18n` package gained `Tf(locale, key, vars)` — `{{name}}`
interpolation plus i18next-compatible plural selection (`key_one` / `key_other`
chosen from a `count` var). Call sites never `fmt.Sprintf` a translated string and
never build a plural by appending "s"; both would put word order and grammar in Go
instead of in the catalog.

The locale is **not persisted** on the `task_shares` row. Nothing re-renders an
artifact after upload today, so there is nothing to be consistent with.

## Consequences

- A share created by a user working in French produces a French artifact. The
  language matches the author's, which is the only signal we have and is consistent
  with every other surface in the product.
- The published language is fixed at creation. A reader who prefers another language
  gets the creator's; re-sharing the session under a different locale is the only way
  to change it. This is inherent to publishing a static file and is accepted.
- `Backend.Upload` gained a parameter. Any future share backend must accept and honor
  a locale rather than assuming English.
- The pseudo-locale is now a genuine completeness oracle for these files: rendering a
  snapshot under `pseudo` and finding an English message proves a literal was left
  hardcoded, which is exactly what the new builder tests assert.
- **If a re-render path is ever added** (for example regenerating `README.md`
  post-upload with the real rendered URL, which the code comments contemplate), the
  creator's locale must be persisted on the share row first — otherwise the
  regenerated file silently reverts to whatever locale that later caller happens to
  hold.

## Alternatives Considered

1. **Always English.** Rejected. It is defensible — a public link goes to arbitrary
   readers — but it makes the artifact the one place in the product that ignores the
   user's language choice, and it would leave the copy permanently un-exercised by
   the pseudo-locale, so regressions would be invisible. English remains the
   fallback for any unsupported locale anyway, so the failure mode is already
   covered.
2. **Creator's locale with a `?locale=` override on the create request.** Rejected
   for now as unjustified surface: it adds a public API parameter and its validation
   and tests to serve a case nobody has asked for. The threading this ADR
   establishes is exactly what such an override would need, so adding it later is
   cheap.
3. **Carry the locale in `context.Context`.** Rejected. It would make the builders
   depend on ambient state, and a caller that forgot to populate the context would
   silently emit English with no compile-time signal.

## Notes

The rule for what counts as copy is unchanged and worth restating, because this code
mixes the two freely:

- `messageRoleAttrs` returns a **CSS class**, a **label**, and an **emoji**. Only the
  label is copy. Its fallback branch echoes the raw role from the message store —
  wire data, never translated.
- `roleUser` / `roleAssistant` / `roleSystem` are wire values, not display strings.
- In "Built with [kandev](…)" only "Built with" is copy; brand names, URLs, filenames
  (`share.html`, `snapshot.json`) and emoji are not. Interpolated values keep
  filenames and URLs out of the catalog so the pseudo-locale cannot transliterate
  them into dead pointers.
- The ~37 `http.Error` strings in the backend stay English: they are diagnostics the
  SPA maps onto its own translated toasts, and translating them would duplicate the
  copy in two places.
