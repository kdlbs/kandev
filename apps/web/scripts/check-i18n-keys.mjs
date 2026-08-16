#!/usr/bin/env node
/**
 * Verify translation keys and catalogs agree.
 *
 * With key-based i18next the English copy lives ONLY in `src/locales/en/*.json`
 * — it cannot be regenerated from source the way Lingui's source-text keys
 * could. So the CI gate is a DRIFT check, not a re-extraction:
 *
 *   - every `t("ns:key")` / `<Trans i18nKey="ns:key">` in source has a catalog
 *     entry  -> missing keys are an ERROR (they would render as the raw key)
 *   - every catalog entry is referenced somewhere -> orphans are a WARNING
 *   - `en` and `pseudo` have the same key sets -> drift is an ERROR
 *   - real locales (anything that is not `en`/`pseudo`, discovered from the
 *     directory listing rather than named here) -> parity issues are an ERROR
 *
 * ## Why real locales gate too
 *
 * They did not used to. Gating them meant an ordinary English-only PR failed CI
 * for work nobody in the merge path could do - #2261 added 13 `en` chat keys and
 * left `main` red because #2243 had introduced `zh-cn` 20 minutes earlier. So
 * the pass was advisory, and the cost of that showed up exactly where you would
 * expect: pt-pt was 312 keys short and zh-hk/zh-tw 415, none of it visible in a
 * green build.
 *
 * The four catalogs are now complete, so the failure mode is the other one: a
 * PR that adds an `en` key and no translation silently re-opens the gap. This
 * gates, and the cost is that adding user-facing copy means adding it in five
 * languages. That is the actual cost of shipping five languages.
 *
 * ## The identical-to-English check
 *
 * The structural pass compares SHAPES, so a catalog copy-pasted wholesale from
 * `en` passes every part of it. `untranslatedValueIssues` closes that, in two
 * tiers (see `scripts/lib/i18n-catalogs.mjs`): values the shared `looksLikeCopy`
 * predicate says are not copy need no explanation, and the prose that survives
 * it is declared in a `_verbatim.json` registry with a reason. Stale registry
 * entries are themselves an error, so the files cannot rot into the shrinking
 * allowlist this was meant not to be.
 *
 * Usage: node scripts/check-i18n-keys.mjs [--strict-orphans]
 */
import fs from "node:fs";
import path from "node:path";

import { noLiteralStringOptions } from "../eslint.i18n.options.mjs";
import {
  discoverRealLocales,
  formatParityIssue,
  formatVerbatimIssue,
  readLocaleNamespaces,
  readVerbatimRegistry,
  realLocaleParityIssues,
  staleVerbatimEntries,
  untranslatedValueIssues,
} from "./lib/i18n-catalogs.mjs";
import { looksLikeCopy } from "./lib/looks-like-copy.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const LOCALES = path.join(ROOT, "src", "locales");
const STRICT_ORPHANS = process.argv.includes("--strict-orphans");

function readCatalog(locale) {
  const out = new Map(); // "ns:key" -> value
  for (const [namespace, messages] of readLocaleNamespaces(LOCALES, locale)) {
    for (const [key, value] of messages) out.set(`${namespace}:${key}`, value);
  }
  return out;
}

function sourceFiles() {
  const out = [];
  const walk = (dir) => {
    if (!fs.existsSync(dir)) return;
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, e.name);
      if (e.isDirectory()) {
        if (["node_modules", "dist", "e2e", "locales"].includes(e.name)) continue;
        walk(full);
      } else if (/\.(tsx?|mts)$/.test(e.name) && !/\.test\.tsx?$/.test(e.name)) {
        out.push(full);
      }
    }
  };
  for (const r of ["components", "app", "lib", "src", "hooks"]) walk(path.join(ROOT, r));
  return out;
}

const en = readCatalog("en");

// Namespaces that actually exist, so a bare "foo:bar" string elsewhere in the
// code isn't mistaken for a translation key.
const NAMESPACES = new Set([...en.keys()].map((k) => k.split(":")[0]));

// Direct call sites: t("ns:key"), globalT("ns:key"), i18nKey="ns:key".
const CALL_RE = /(?:\bt\(|\bglobalT\(|i18nKey=)\s*["']([a-zA-Z0-9_]+:[a-zA-Z0-9_.-]+)["']/g;
// Keys held in constants/tables and resolved dynamically later (`t(item.label)`),
// which is how the former Lingui `msg` descriptors were converted.
const LITERAL_RE = /["']([a-zA-Z0-9_]+:[a-zA-Z0-9_.-]+)["']/g;

const used = new Map(); // key -> Set(file); drives the "missing key" check
const referencedLiterals = new Set(); // only clears orphans, never reports missing
const note = (key, file) => {
  if (!used.has(key)) used.set(key, new Set());
  used.get(key).add(path.relative(ROOT, file));
};
for (const file of sourceFiles()) {
  const code = fs.readFileSync(file, "utf8");
  for (const m of code.matchAll(CALL_RE)) note(m[1], file);
  // A bare "ns:key" literal is ambiguous — testids and storage keys share the
  // shape — so it may only mark a catalog entry as used, never demand one.
  for (const m of code.matchAll(LITERAL_RE)) {
    if (NAMESPACES.has(m[1].split(":")[0])) referencedLiterals.add(m[1]);
  }
}
const pseudo = readCatalog("pseudo");

/** Plural keys live as `key_one` / `key_other`; the source references `key`. */
function isSatisfied(catalog, key) {
  if (catalog.has(key)) return true;
  return catalog.has(`${key}_one`) || catalog.has(`${key}_other`);
}

const missing = [...used.keys()].filter((k) => !isSatisfied(en, k)).sort();
const pluralBases = new Set(
  [...en.keys()].filter((k) => /_(one|other)$/.test(k)).map((k) => k.replace(/_(one|other)$/, "")),
);
const orphans = [...en.keys()]
  .filter((k) => {
    const base = k.replace(/_(one|other)$/, "");
    if (used.has(k) || referencedLiterals.has(k)) return false;
    return !(pluralBases.has(base) && (used.has(base) || referencedLiterals.has(base)));
  })
  .sort();

const enKeys = new Set(en.keys());
const pseudoKeys = new Set(pseudo.keys());
const pseudoMissing = [...enKeys].filter((k) => !pseudoKeys.has(k));
const pseudoExtra = [...pseudoKeys].filter((k) => !enKeys.has(k));
const sourceNamespaces = readLocaleNamespaces(LOCALES, "en");
const realLocales = discoverRealLocales(LOCALES);
const localeNamespaces = new Map(
  realLocales.map((locale) => [locale, readLocaleNamespaces(LOCALES, locale)]),
);
const excludes = noLiteralStringOptions.words?.exclude ?? [];
const isCopy = (value) => looksLikeCopy(value, excludes);
const sharedVerbatim = readVerbatimRegistry(LOCALES);
const ownVerbatim = new Map(
  realLocales.map((locale) => [locale, readVerbatimRegistry(LOCALES, locale)]),
);
const realLocaleIssues = realLocales.flatMap((locale) => [
  ...realLocaleParityIssues(sourceNamespaces, localeNamespaces.get(locale), locale),
  ...untranslatedValueIssues(sourceNamespaces, localeNamespaces.get(locale), locale, isCopy, {
    shared: sharedVerbatim,
    own: ownVerbatim.get(locale),
  }),
]);

// Registry hygiene, so a `_verbatim.json` cannot outlive what it explains.
const flatLocales = realLocales.map((locale) => ({
  locale,
  flat: new Map(
    [...localeNamespaces.get(locale)].flatMap(([ns, messages]) =>
      [...messages].map(([key, value]) => [`${ns}:${key}`, value]),
    ),
  ),
}));
const verbatimIssues = [
  ...staleVerbatimEntries(sharedVerbatim, en, flatLocales),
  ...realLocales.flatMap((locale) =>
    staleVerbatimEntries(ownVerbatim.get(locale), en, flatLocales, locale),
  ),
];

let failed = false;

if (missing.length) {
  failed = true;
  console.error(`\n✖ ${missing.length} key(s) used in source but missing from the en catalog:`);
  for (const k of missing.slice(0, 40)) {
    console.error(`  ${k}  (${[...used.get(k)].slice(0, 2).join(", ")})`);
  }
  if (missing.length > 40) console.error(`  … and ${missing.length - 40} more`);
}

if (pseudoMissing.length || pseudoExtra.length) {
  failed = true;
  console.error(
    `\n✖ pseudo catalog is out of sync with en ` +
      `(${pseudoMissing.length} missing, ${pseudoExtra.length} extra).` +
      `\n  Run: pnpm run i18n:pseudo`,
  );
}

if (realLocaleIssues.length) {
  failed = true;
  console.error(`\n✖ ${realLocaleIssues.length} real-locale catalog issue(s):\n`);
  for (const issue of realLocaleIssues.slice(0, 60)) {
    console.error(`  ${formatParityIssue(issue)}`);
  }
  if (realLocaleIssues.length > 60) {
    console.error(`  … and ${realLocaleIssues.length - 60} more`);
  }
  console.error(
    `\n  Add the translation, or - if the value is prose that reads the same in\n` +
      `  that language - declare it in src/locales/<locale>/_verbatim.json with a\n` +
      `  reason. Brand nouns, acronyms and placeholder-only frames need neither.\n` +
      `  For zh-tw / zh-hk run: pnpm run i18n:zh-hant\n`,
  );
}

if (verbatimIssues.length) {
  failed = true;
  console.error(`\n✖ ${verbatimIssues.length} stale verbatim registry entr(ies):\n`);
  for (const issue of verbatimIssues) console.error(`  ${formatVerbatimIssue(issue)}`);
  console.error("");
}

if (orphans.length) {
  const label = STRICT_ORPHANS ? "✖" : "⚠";
  console[STRICT_ORPHANS ? "error" : "warn"](
    `\n${label} ${orphans.length} catalog entr(ies) not referenced in source:`,
  );
  for (const k of orphans.slice(0, 20)) console.warn(`  ${k}`);
  if (orphans.length > 20) console.warn(`  … and ${orphans.length - 20} more`);
  if (STRICT_ORPHANS) failed = true;
}

if (!failed) {
  // Say only what was actually gated. The previous wording claimed "pseudo and
  // zh-cn in sync", which was misleading twice over: it implied a real locale
  // gates the build, and it read as "translated" for a check that cannot see
  // English left inside a translated value.
  const gated = realLocales.length ? ` ${realLocales.join(", ")} complete.` : "";
  console.log(
    `✓ i18n keys OK — ${used.size} key(s) referenced, ${en.size} en entr(ies), ` +
      `${orphans.length} orphan(s), pseudo in sync.${gated}`,
  );
}
process.exit(failed ? 1 : 0);
