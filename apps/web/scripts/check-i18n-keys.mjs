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
 *   - every committed locale has the same key set as `en` -> drift is an ERROR
 *
 * Usage: node scripts/check-i18n-keys.mjs [--strict-orphans]
 */
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const LOCALES = path.join(ROOT, "src", "locales");
const STRICT_ORPHANS = process.argv.includes("--strict-orphans");

function readCatalog(locale) {
  const dir = path.join(LOCALES, locale);
  const out = new Map(); // "ns:key" -> value
  if (!fs.existsSync(dir)) return out;
  for (const file of fs.readdirSync(dir).filter((f) => f.endsWith(".json"))) {
    const ns = file.replace(/\.json$/, "");
    const entries = JSON.parse(fs.readFileSync(path.join(dir, file), "utf8"));
    for (const [key, value] of Object.entries(entries)) out.set(`${ns}:${key}`, value);
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
const translatedLocales = fs
  .readdirSync(LOCALES, { withFileTypes: true })
  .filter((entry) => entry.isDirectory() && entry.name !== "en")
  .map((entry) => entry.name)
  .sort();
const translations = new Map(translatedLocales.map((locale) => [locale, readCatalog(locale)]));

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
const catalogDrift = translatedLocales.map((locale) => {
  const localeKeys = new Set(translations.get(locale).keys());
  return {
    locale,
    missing: [...enKeys].filter((k) => !localeKeys.has(k)),
    extra: [...localeKeys].filter((k) => !enKeys.has(k)),
  };
});

let failed = false;

if (missing.length) {
  failed = true;
  console.error(`\n✖ ${missing.length} key(s) used in source but missing from the en catalog:`);
  for (const k of missing.slice(0, 40)) {
    console.error(`  ${k}  (${[...used.get(k)].slice(0, 2).join(", ")})`);
  }
  if (missing.length > 40) console.error(`  … and ${missing.length - 40} more`);
}

for (const { locale, missing: localeMissing, extra: localeExtra } of catalogDrift) {
  if (localeMissing.length || localeExtra.length) {
    failed = true;
    const hint = locale === "pseudo" ? "\n  Run: pnpm run i18n:pseudo" : "";
    console.error(
      `\n✖ ${locale} catalog is out of sync with en ` +
        `(${localeMissing.length} missing, ${localeExtra.length} extra).` +
        hint,
    );
  }
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
  console.log(
    `✓ i18n keys OK — ${used.size} key(s) referenced, ${en.size} en entr(ies), ` +
      `${orphans.length} orphan(s), ${translatedLocales.join(", ")} in sync.`,
  );
}
process.exit(failed ? 1 : 0);
