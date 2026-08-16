/**
 * `looksLikeCopy`: does a string read as something a user is meant to understand?
 *
 * Lives in its own module, free of any dependency, because three checks share it
 * and they must not disagree about what a translatable string is:
 *
 *   - `nonjsx-copy.mjs`, for copy in positions eslint cannot see
 *   - `check-new-code-i18n.mjs`, through the same detector
 *   - `check-i18n-keys.mjs`, to decide whether a locale value left identical to
 *     English is a missing translation or a brand noun
 *
 * The last one is why this is separate: `nonjsx-copy.mjs` pulls in `@babel/core`
 * to parse, and the catalog checker has no business loading a parser to ask
 * whether "Webhook" is a word.
 */
/**
 * Strings that are not translatable copy even when they read like prose.
 *
 * Deliberately narrow. Anything judgement-shaped (a persisted default, an
 * agent-facing prompt) is NOT listed here — it takes an `i18n-exempt` comment
 * with a reason, so the decision stays visible at the call site instead of being
 * silently absorbed by a pattern in this file.
 */
const NOT_COPY = [
  /^\s*$/,
  /^[^A-Za-z]*$/,
  // CSS: selectors, media queries, declarations, and class lists.
  /^[.#[(]/,
  /^[a-z-]+\s*:\s*[^;]+$/i,
  /(^|\s)(flex|grid|hidden|absolute|relative|sticky|fixed|inline-\w+|block|truncate|overflow-\w+|shrink|grow|group|peer)(\s|$)/,
  /(^|\s)[a-z-]+-(\[|\d|auto|full|px|none|foreground|background|muted|primary|secondary|border|card)/,
  // Identifiers, keys, paths, and wire values.
  /^[a-z][A-Za-z0-9]*$/,
  /^[A-Z][A-Z0-9_]*$/,
  /^[\w.-]+:[\w.-]+$/,
  /^[~./]/,
  /^(https?|file|ssh|git|data|mailto):/,
  // Log and debug prefixes are developer output, never shown to a user.
  /^\[[A-Za-z][\w\s]*\]/,
  // Single short word: too little signal to call it copy either way.
  /^\S{1,3}$/,
];

/** A single hyphenated CSS token: `text-muted-foreground`, `gap-2`. */
const HYPHENATED_TOKEN = /^[a-z][\w-]*$/;

/**
 * A whitespace-separated list of hyphenated tokens — a class list, not prose.
 *
 * Checked token by token rather than with one regex. The single-regex form
 * (`/^[a-z][\w-]*(-[\w-]+)+(\s+...)*$/`) nests a quantifier inside a
 * quantifier over an alphabet that includes `-`, so `"a"` followed by many `-`
 * has exponentially many parses and CodeQL flagged it as backtracking-prone.
 * Splitting first makes it linear and says what it means.
 *
 * Rejecting every all-lowercase multi-word string would be the easy version and
 * it hides real copy: "every hour", "ended (session restart)" and "watching" are
 * all lowercase and all shown to a user. Requiring a hyphen in EVERY token is
 * what separates them, since no English phrase has one.
 */
function isClassTokenList(text) {
  const tokens = text.split(/\s+/);
  return tokens.every((token) => token.includes("-") && HYPHENATED_TOKEN.test(token));
}

/**
 * True when `value` reads as something a user is meant to understand.
 *
 * @param {string} value
 * @param {string[]} [extraExcludes]
 */
export function looksLikeCopy(value, extraExcludes = []) {
  if (typeof value !== "string") return false;
  const text = value.trim();
  if (!text) return false;
  if (NOT_COPY.some((pattern) => pattern.test(text))) return false;
  if (isClassTokenList(text)) return false;
  // The eslint guard's own exclusions, so the two agree on what a brand noun,
  // acronym, unit or keyboard glyph is. The plugin anchors every entry.
  if (extraExcludes.some((pattern) => new RegExp(`^${pattern}$`).test(text))) return false;
  const words = text.split(/\s+/).filter((word) => /[A-Za-z]{2,}/.test(word));
  if (words.length >= 2) return true;
  // One word only counts when it is capitalized prose ("Staged", "Walkthrough").
  return /^[A-Z][a-z]{3,}[.!?]?$/.test(text);
}
