/**
 * Pure helpers for `check-removed-literals.mjs`: extracting every user-visible
 * string from a source file, and deciding whether a string a change removed is
 * still accounted for by the catalog.
 *
 * Split out so the matching rules can be unit-tested against the failure they
 * exist to catch, without needing a git checkout or a real diff.
 */
import { parseSync } from "@babel/core";

export function walk(node, visit) {
  if (!node || typeof node.type !== "string") return;
  visit(node);
  for (const key of Object.keys(node)) {
    if (key === "loc" || key.endsWith("Comments")) continue;
    const child = node[key];
    if (Array.isArray(child)) {
      for (const c of child) if (c && typeof c.type === "string") walk(c, visit);
    } else if (child && typeof child.type === "string") walk(child, visit);
  }
}

/**
 * Every user-visible string shape in one file.
 *
 * JSX TEXT is the load-bearing case and the one a regex over quoted strings
 * misses entirely: `<Link>Back to Workspaces</Link>` carries no quotes, and it
 * is the majority of migrated copy. Template quasis matter because the guard
 * validates them too.
 */
export function literals(source, filename) {
  let ast;
  try {
    ast = parseSync(source, {
      filename,
      babelrc: false,
      configFile: false,
      sourceType: "module",
      parserOpts: { plugins: ["typescript", "jsx"] },
    });
  } catch {
    return null; // Unparseable side of the diff — reported by the caller.
  }
  const found = new Set();
  walk(ast, (node) => {
    if (node.type === "StringLiteral") found.add(node.value);
    else if (node.type === "JSXText") found.add(node.value);
    else if (node.type === "TemplateLiteral")
      for (const q of node.quasis) found.add(q.value.cooked ?? q.value.raw);
  });
  return new Set([...found].map((s) => s.replace(/\s+/g, " ").trim()).filter(Boolean));
}

/**
 * A message and its source prose compare equal once the markup a migration adds
 * is removed: `<Trans>` tag indices, `{{interpolations}}`, and the whitespace
 * JSX collapses. Without this the check is pure noise — every `<Trans>` and
 * every value extraction reports.
 */
export function normalize(value) {
  return value
    .replace(/<\/?\d+>/g, "")
    .replace(/\{\{[^}]*\}\}/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

/**
 * A message as a pattern, with each `{{value}}` free to be anything.
 *
 * This is what accounts for a VALUE EXTRACTION — the commonest benign finding.
 * `"Files over 5 MiB are skipped…"` in the source becomes
 * `"Files over {{limit}} are skipped…"` in the catalog, which is the same
 * sentence with the units pulled out, not a rewrite.
 */
export function messagePattern(value) {
  const escaped = value
    .replace(/<\/?\d+>/g, "")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
    .replace(/\\\{\\\{[^}]*\\\}\\\}/g, "[\\s\\S]+?");
  return new RegExp(`^${escaped}$`);
}

/**
 * Is this removed string still accounted for by some catalog message?
 *
 * Three ways, in increasing looseness:
 *   1. it IS a message;
 *   2. it is an INSTANCE of one, with interpolated values filled in;
 *   3. it is a FRAGMENT of one — a `<Trans>` splits its sentence into a JSX text
 *      node per element boundary, so the halves either side of a `<code>` arrive
 *      here separately and can never match a whole message.
 *
 * (3) trades a little sensitivity for usability: a fragment could in principle
 * be a coincidental substring of an unrelated message. That is a far better
 * trade than the alternative, because a check nobody runs catches nothing — and
 * the failure this exists for is not subtle. A rewritten sentence is not a
 * substring of the sentence that replaced it.
 */
export function accountedFor(value, { exact, normalized, patterns }) {
  if (exact.has(value)) return true;
  const norm = normalize(value);
  if (normalized.has(norm)) return true;
  if (patterns.some((re) => re.test(norm) || re.test(value))) return true;
  return [...normalized].some((message) => message.length >= norm.length && message.includes(norm));
}

/** Prose, as opposed to a classname, enum token, path or identifier. */
export function looksLikeCopy(value) {
  if (value.length < 4) return false;
  if (!/[A-Za-z]{3}/.test(value)) return false;
  if (!/[A-Z]/.test(value) && !/[a-z]+ [a-z]+/.test(value)) return false;
  if (/^[/~.]/.test(value)) return false; // paths
  if (/^(https?|file|ssh|git):\/\//.test(value)) return false; // URLs
  if (/^[a-z][a-zA-Z0-9]*$/.test(value)) return false; // camelCase identifier
  if (/^[a-z0-9]+([-_][a-z0-9]+)+$/.test(value)) return false; // kebab/snake token
  // A `namespace:key` catalog reference. Swapping one `t()` key for another
  // registers as a removed literal, because the key is itself a StringLiteral —
  // so every migration that re-points a call site reports its own old key. It is
  // an identifier, never copy, and leaving it in reads alarmingly to someone
  // expecting a clean run.
  if (/^[a-z][a-zA-Z0-9]*:[a-zA-Z0-9_]+$/.test(value)) return false;
  return true;
}

/**
 * Can this message anchor a pattern, or would its pattern match anything?
 *
 * A message that is NOTHING BUT a placeholder — `settings:externalMcpConfigPath`
 * is literally `"{{path}}"` — compiles to `/^[\s\S]+?$/`, which matches every
 * string in the repository. Because rule 2 runs before rule 3, ONE such key
 * makes `accountedFor` return true for everything and the whole check becomes a
 * silent no-op that still prints ✓. That is strictly worse than not running it,
 * because it converts "I verified" into "I ran something".
 *
 * The requirement is TWO WORDS of prose, not merely a non-empty remainder and
 * not merely one word. The empty case is the extreme of a continuum, not a
 * distinct bug — 44 interpolating messages have a residue of six characters or
 * fewer — and one short word clears any "is there a word?" bar while leaving the
 * pattern almost unbounded:
 *
 *   "{{a}} —"        -> /^[\s\S]+? —$/          matches anything ending " —"
 *   "{{count}}m"     -> /^[\s\S]+?m$/           matches "Confirm", "Custom", "System"
 *   "Delete {{name}}" -> /^Delete [\s\S]+?$/    matches EVERY "Delete …" sentence
 *
 * Measured on two migrations, requiring two words dropped 340 pattern-bearing
 * messages to 292, added ZERO findings to either diff, and made ten more
 * rewrites detectable on one of them. It costs nothing real and closes the
 * largest remaining hole.
 *
 * The honest limit of this rule: `"{{count}} of {{total}}"` (a legitimate value
 * extraction) and `"{{a}} of {{b}}"` (far too permissive to anchor anything)
 * both reduce to the residue `"of"` and compile to the SAME regex, so nothing
 * computed from the message can accept one and reject the other. Both are
 * rejected here, which means a string like `"3 of 9"` is REPORTED and classified
 * by a human. That is the correct direction for an advisory check: a false
 * positive costs someone ten seconds, a false negative is the failure this
 * exists to prevent.
 *
 * A rejected message is still matched exactly and as a fragment — it only loses
 * the wildcard, which is all it could honestly support.
 */
function canAnchorPattern(message) {
  const words = normalize(message)
    .split(/\s+/)
    .filter((word) => /[A-Za-z]{2,}/.test(word));
  return words.length >= 2;
}

/** Build the three lookup shapes `accountedFor` needs from a set of messages. */
export function buildCatalog(values) {
  const set = values instanceof Set ? values : new Set(values);
  return {
    exact: set,
    normalized: new Set([...set].map(normalize)),
    patterns: [...set].filter((v) => v.includes("{{") && canAnchorPattern(v)).map(messagePattern),
  };
}
