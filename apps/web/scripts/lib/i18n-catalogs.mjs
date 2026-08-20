import fs from "node:fs";
import path from "node:path";

/** The verbatim registry, not a namespace. Underscore-prefixed so it sorts apart. */
export const VERBATIM_FILE = "_verbatim.json";

export function readLocaleNamespaces(localesDir, locale) {
  const dir = path.join(localesDir, locale);
  const namespaces = new Map();
  if (!fs.existsSync(dir)) return namespaces;
  for (const file of fs
    .readdirSync(dir)
    .filter((entry) => entry.endsWith(".json") && entry !== VERBATIM_FILE)
    .sort()) {
    const namespace = file.replace(/\.json$/, "");
    const messages = JSON.parse(fs.readFileSync(path.join(dir, file), "utf8"));
    namespaces.set(namespace, new Map(Object.entries(messages)));
  }
  return namespaces;
}

export function discoverRealLocales(localesDir) {
  if (!fs.existsSync(localesDir)) return [];
  return fs
    .readdirSync(localesDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && !["en", "pseudo"].includes(entry.name))
    .map((entry) => entry.name)
    .sort();
}

function interpolationPlaceholders(message) {
  return [
    ...new Set([...message.matchAll(/\{\{\s*-?\s*([^},\s]+)[^}]*\}\}/g)].map((match) => match[1])),
  ].sort();
}

function numericTagStructure(message) {
  const stack = [];
  const nodes = [];
  for (const match of message.matchAll(/<(\/)?(\d+)>/g)) {
    const closing = Boolean(match[1]);
    const tag = match[2];
    if (closing) {
      if (stack.at(-1) !== tag) return null;
      stack.pop();
      continue;
    }
    nodes.push([...stack, tag].join("/"));
    stack.push(tag);
  }
  return stack.length === 0 ? nodes.sort() : null;
}

function sameItems(left, right) {
  return left !== null && right !== null && JSON.stringify(left) === JSON.stringify(right);
}

function translatedValueIssue(value, context) {
  if (typeof value !== "string") {
    return { ...context, type: "non-string translated value" };
  }
  if (value.trim().length === 0) {
    return { ...context, type: "empty translated value" };
  }
  return null;
}

function messageParityIssues(sourceMessage, translatedMessage, context) {
  const issues = [];
  if (
    !sameItems(
      interpolationPlaceholders(sourceMessage),
      interpolationPlaceholders(translatedMessage),
    )
  ) {
    issues.push({ ...context, type: "interpolation placeholder mismatch" });
  }
  if (!sameItems(numericTagStructure(sourceMessage), numericTagStructure(translatedMessage))) {
    issues.push({ ...context, type: "Trans tag structure mismatch" });
  }
  return issues;
}

function translatedValueIssues(translated, locale) {
  const issues = [];
  for (const [namespace, translatedMessages] of translated) {
    for (const [key, value] of translatedMessages) {
      const valueIssue = translatedValueIssue(value, { locale, namespace, key });
      if (valueIssue) issues.push(valueIssue);
    }
  }
  return issues;
}

function namespaceParityIssues(source, translated, locale) {
  const issues = [];
  for (const namespace of source.keys()) {
    if (!translated.has(namespace)) {
      issues.push({ locale, namespace, type: "missing namespace" });
      continue;
    }
    const sourceMessages = source.get(namespace);
    const translatedMessages = translated.get(namespace);
    issues.push(
      ...messageParityIssuesForNamespace(sourceMessages, translatedMessages, locale, namespace),
    );
  }
  return issues;
}

function messageParityIssuesForNamespace(sourceMessages, translatedMessages, locale, namespace) {
  const issues = [];
  for (const key of sourceMessages.keys()) {
    if (!translatedMessages.has(key)) {
      issues.push({ locale, namespace, type: "missing key", key });
      continue;
    }
    const sourceMessage = sourceMessages.get(key);
    const translatedMessage = translatedMessages.get(key);
    if (typeof sourceMessage !== "string" || typeof translatedMessage !== "string") continue;
    issues.push(
      ...messageParityIssues(sourceMessage, translatedMessage, { locale, namespace, key }),
    );
  }
  for (const key of translatedMessages.keys()) {
    if (!sourceMessages.has(key)) {
      issues.push({ locale, namespace, type: "extra key", key });
    }
  }
  return issues;
}

function extraNamespaceIssues(source, translated, locale) {
  const issues = [];
  for (const namespace of translated.keys()) {
    if (!source.has(namespace)) {
      issues.push({ locale, namespace, type: "extra namespace" });
    }
  }
  return issues;
}

export function realLocaleParityIssues(source, translated, locale) {
  return [
    ...translatedValueIssues(translated, locale),
    ...namespaceParityIssues(source, translated, locale),
    ...extraNamespaceIssues(source, translated, locale),
  ];
}

export function formatParityIssue(issue) {
  const suffix = issue.key ? `: ${issue.key}` : "";
  return `${issue.locale} / ${issue.namespace}: ${issue.type}${suffix}`;
}

/**
 * Read a verbatim registry: `{ "ns:key": "<reason>" }`.
 *
 * `localesDir/_verbatim.json` is the SHARED registry — values every locale may
 * legitimately keep in English because they are proper nouns, technical terms,
 * or frames made only of placeholders. `localesDir/<locale>/_verbatim.json` is
 * the per-locale one, for the narrower case where the correct translation
 * happens to BE the English word ("Total", "Manual", "Menu" in pt-PT).
 *
 * Missing file reads as empty: a locale that translates everything needs none.
 */
export function readVerbatimRegistry(localesDir, locale = null) {
  const file = locale
    ? path.join(localesDir, locale, VERBATIM_FILE)
    : path.join(localesDir, VERBATIM_FILE);
  if (!fs.existsSync(file)) return new Map();
  return new Map(Object.entries(JSON.parse(fs.readFileSync(file, "utf8"))));
}

/** A reason must actually say something, exactly like an `i18n-exempt` marker. */
const MIN_REASON_LENGTH = 3;

function flatten(namespaces) {
  const flat = new Map();
  for (const [namespace, messages] of namespaces) {
    for (const [key, value] of messages) flat.set(`${namespace}:${key}`, value);
  }
  return flat;
}

/**
 * Values a locale left identical to English without saying why.
 *
 * The structural checks above compare SHAPES, so a catalog copy-pasted from `en`
 * passes every one of them. This is the check that does not.
 *
 * Two tiers, so the registry only ever holds real decisions:
 *
 *  1. RULE. If the English value is not copy by `looksLikeCopy` — a brand noun,
 *     an acronym, a wire value, a frame of nothing but placeholders — then every
 *     locale is expected to keep it verbatim and nothing needs declaring. That
 *     predicate is the same one `i18next/no-literal-string` and
 *     `check-nonjsx-copy.mjs` use to decide what counts as copy, so the three
 *     cannot disagree about what a translatable string is.
 *  2. DECLARATION. What survives tier 1 is prose, and prose that reads the same
 *     in the target language is a judgement a person made. It is declared in a
 *     registry with a reason, which is what separates "we checked, it is the
 *     same word" from "we never translated this".
 *
 * @param {Map<string, Map<string, string>>} source `en` namespaces
 * @param {Map<string, Map<string, string>>} translated the locale's namespaces
 * @param {string} locale
 * @param {(value: string) => boolean} isCopy
 * @param {{shared: Map<string, string>, own: Map<string, string>}} verbatim
 */
export function untranslatedValueIssues(source, translated, locale, isCopy, verbatim) {
  const en = flatten(source);
  const them = flatten(translated);
  const issues = [];
  for (const [id, value] of them) {
    if (!en.has(id) || en.get(id) !== value) continue;
    if (typeof value !== "string" || !isCopy(value)) continue;
    const reason = verbatim.own.get(id) ?? verbatim.shared.get(id);
    const [namespace, ...rest] = id.split(":");
    const key = rest.join(":");
    if (reason === undefined) {
      issues.push({ locale, namespace, key, type: "value identical to English" });
      continue;
    }
    if (typeof reason !== "string" || reason.trim().length < MIN_REASON_LENGTH) {
      issues.push({ locale, namespace, key, type: "verbatim entry with no reason" });
    }
  }
  return issues;
}

/**
 * Registry entries that guard nothing, so the files cannot accumulate debt.
 *
 * A per-locale entry is stale as soon as that locale translates the value; a
 * shared entry is stale only when NO locale keeps it verbatim, since it exists
 * for whichever ones still do.
 *
 * @param {Map<string, string>} registry
 * @param {Map<string, string>} en flattened `en` catalog
 * @param {Array<{locale: string, flat: Map<string, string>}>} locales
 * @param {string|null} owner locale name, or null for the shared registry
 */
export function staleVerbatimEntries(registry, en, locales, owner = null) {
  const stale = [];
  for (const id of registry.keys()) {
    if (!en.has(id)) {
      stale.push({ id, owner, type: "no such key in en" });
      continue;
    }
    const scope = owner ? locales.filter((l) => l.locale === owner) : locales;
    const kept = scope.some(({ flat }) => flat.get(id) === en.get(id));
    if (!kept) stale.push({ id, owner, type: "every locale translates this" });
  }
  return stale;
}

export function formatVerbatimIssue(issue) {
  const where = issue.owner ? `${issue.owner}/${VERBATIM_FILE}` : VERBATIM_FILE;
  return `${where}: ${issue.id} — ${issue.type}`;
}
