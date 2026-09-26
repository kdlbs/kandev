# TYPO-013: Normalize settings description punctuation

- Priority: P2
- Status: Resolved
- Scope: Settings page descriptions and inline helper copy
- Related: TYPO-005, TYPO-012

## Finding

Settings descriptions and helper rows do not consistently use complete-sentence
punctuation. One inline helper also uses a Unicode em dash, which conflicts
with the repository's user-facing copy rule. This is a small copy concern, but
it affects the visual rhythm and consistency of the same description role.

## Evidence

- `apps/web/app/settings/workspace/workspaces-page-client.tsx:178-180` uses `workspaces:manageYourWorkspacesAndWorkflows`, whose English value in `apps/web/src/locales/en/workspaces.json:81` has no terminal period while comparable top-level descriptions are complete sentences.
- `apps/web/components/settings/external-mcp-settings.tsx:153-157` renders tool helper text with a Unicode em dash before the translated description. The repository guidance requires ordinary punctuation instead of U+2014 in user-facing copy.

## User impact

Descriptions that serve the same role can look unfinished or use different visual separators. The em dash also violates the enforced punctuation convention and can be detected by the i18n checks.

## Proposed direction

Choose a consistent description convention, preferably complete sentences with
terminal punctuation for page-level explanatory copy. Replace the em dash with
a colon, period, or styled separator that is not part of the translated text.
Keep imperative labels intentionally short only when they are classified as
labels rather than descriptions.

## Verification

- Review page descriptions and inline helper rows in the pseudo-locale.
- Run the web i18n checks after copy changes.
- Confirm the separator remains readable without changing the helper's semantic
  structure.

## Progress

- 2026-08-16: Opened from the delayed top-level settings audit.
