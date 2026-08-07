/**
 * Labels for option lists that mix our own copy with text we must not translate.
 *
 * A picker's items usually come from two places at once: a static list we wrote
 * (translatable) and runtime values we were given — the user's agent-profile
 * names, a repository slug, an acronym like `CEO` or `QA`. Forcing one field to
 * cover both means either translating somebody's data or leaving our own copy in
 * English, so the two cases are separate fields and callers resolve through
 * {@link resolveOptionLabel}.
 *
 * Module-scope option lists must hold `labelKey`, never a `t()` call: `t()` at
 * import time resolves before a locale is active and never updates on a switch.
 */
export type OptionLabel = {
  /** Catalog key for copy we own. */
  labelKey?: string;
  /** Verbatim text — runtime data or a proper noun. Used when there is no key. */
  label?: string;
};

/** Display text for an option. `t` comes from the caller's `useTranslation`. */
export function resolveOptionLabel(
  t: (key: string) => string,
  option: OptionLabel | null | undefined,
): string {
  if (!option) return "";
  return option.labelKey ? t(option.labelKey) : (option.label ?? "");
}
