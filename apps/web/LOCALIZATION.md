## Web localization

Translate all new user-facing copy with `t()` or `<Trans>`.
Add English keys to `src/locales/en/<namespace>.json` and translations for all supported locales.
Use `useTranslation()` in components. Use `<Trans>` for markup; a `t()` child corrupts its tag indices.
Do not translate domain data, identifiers, test IDs, discriminants, comparison tokens, or map keys.
Do not call `t()` at module scope. Use `_one` and `_other` plural keys with `count`.
Do not use the Unicode em dash (U+2014) in user-facing copy or locale values.
Keep the initialization in `lib/i18n/provider.tsx`; the app needs it to render.

`i18next/no-literal-string` applies to all `.ts` and `.tsx` files, excluding tests,
test helpers, and `e2e/`. `i18n:ratchet` checks new and changed lines.
`scripts/check-nonjsx-copy.mjs` also checks plain helpers, defaults, tables, and toast arguments.
For non-copy values, prefer `Symbol()` for private sentinels. Otherwise use a line comment:
`// i18n-exempt: <reason>`. A block comment or a marker without a reason does not pass.
Append externalized paths to `i18nGuardFiles`; never remove an entry.

The locale check rejects missing or extra keys, missing placeholders or tags, empty values,
and unchanged English copy. For genuinely identical prose, add a reason to the locale's
`_verbatim.json` or the shared registry. Non-copy values need no declaration.
For `zh-tw` and `zh-hk`, run `pnpm run i18n:zh-hant` instead of translating by hand.
Use the pseudo-locale to check copy that static checks cannot see.

Run `pnpm run i18n:check` from `apps/web`; it requires Node 24.
For full instructions, see [the i18n guide](../../docs/i18n.md) and
[requirements](../../docs/specs/platform/requirements/i18n.md).
