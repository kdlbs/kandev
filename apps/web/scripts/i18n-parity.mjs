#!/usr/bin/env node
/**
 * Report translation-catalog parity: how many `en` keys each real locale is
 * missing, and which keys it still carries that `en` no longer defines.
 *
 * WHY THIS EXISTS
 *
 * Real-locale parity is deliberately advisory (#2272): Chinese is maintained by
 * a third party and must never be able to fail our builds. That decision stands.
 * Its cost is that drift is silent — pt-PT shipped complete and then fell 1,791
 * keys behind, an entire absent namespace among them, without any check going
 * red. The measurement existed only as a snippet people pasted into chat logs,
 * which is the same as not existing. This makes it a command.
 *
 * It is a REPORT, NOT A GATE. It exits 0 whatever it finds, and it is wired into
 * neither CI, nor pre-commit, nor `i18n:check`. Making it fail a build would
 * quietly overturn #2272. The one exception is input it cannot read (a malformed
 * catalog throws and exits non-zero) — the same line `i18n-sweep.mjs` draws,
 * because an unparseable file is a broken input, not a finding.
 *
 * Locales and namespaces are both discovered from disk via the same helpers
 * `check-i18n-keys.mjs` uses, so the two can never disagree about what a locale
 * is. Adding `fr/` tomorrow extends the report with no edit here; a hardcoded
 * list is the exact shape that let the last gap hide.
 *
 * WHAT A CLEAN RUN DOES *NOT* TELL YOU
 *
 * 1. It counts MISSING KEYS ONLY. A key whose English text changed after it was
 *    translated still counts as present, so the locale looks complete while the
 *    translation says something the source no longer says. Stale translations are
 *    invisible to this script and no check in this repo detects them today.
 * 2. It measures PRESENCE, NOT QUALITY. Every key can be present and every one of
 *    them wrong, machine-translated, or in the wrong regional variant, and this
 *    prints a clean table. The pseudo-locale (Settings > General > Appearance)
 *    and human review remain the only checks on whether the copy is any good.
 *
 * One deliberate difference from `check-i18n-keys.mjs`: there, a wholly-absent
 * namespace scores as a single advisory issue. Here it counts as every key it
 * would have contained. That difference is the whole point — 889 absent keys
 * reported as "1" is precisely how an entire missing namespace stayed invisible.
 *
 * Usage: node scripts/i18n-parity.mjs [localesDir]
 */
import path from "node:path";

import { discoverRealLocales, readLocaleNamespaces } from "./lib/i18n-catalogs.mjs";

const DEFAULT_LOCALES_DIR = path.join(import.meta.dirname, "..", "src", "locales");
const LOCALES_DIR = process.argv[2] ?? DEFAULT_LOCALES_DIR;

const SOURCE_LOCALE = "en";

/**
 * Per-namespace counts for one locale.
 *
 * `missing` treats an absent namespace file as all of its `en` keys missing —
 * see the note above on why this deliberately diverges from the advisory check.
 */
function localeReport(sourceNamespaces, translatedNamespaces) {
  const missingByNamespace = new Map();
  const extraKeys = [];
  const absentNamespaces = [];

  for (const [namespace, sourceMessages] of sourceNamespaces) {
    const translatedMessages = translatedNamespaces.get(namespace);
    if (!translatedMessages) {
      missingByNamespace.set(namespace, sourceMessages.size);
      absentNamespaces.push(namespace);
      continue;
    }
    let missing = 0;
    for (const key of sourceMessages.keys()) if (!translatedMessages.has(key)) missing += 1;
    missingByNamespace.set(namespace, missing);
    for (const key of translatedMessages.keys()) {
      if (!sourceMessages.has(key)) extraKeys.push(`${namespace}: ${key}`);
    }
  }

  // A namespace the locale has and `en` does not is extra in its entirety.
  const extraNamespaces = [...translatedNamespaces.keys()].filter(
    (ns) => !sourceNamespaces.has(ns),
  );

  return { missingByNamespace, extraKeys, extraNamespaces, absentNamespaces };
}

function pad(value, width, align = "right") {
  const text = String(value);
  return align === "right" ? text.padStart(width) : text.padEnd(width);
}

function renderTable(sourceNamespaces, locales, reports) {
  const namespaces = [...sourceNamespaces.keys()];
  const rowLabel = (ns) => `${ns}.json`;

  const nameWidth = Math.max("namespace".length, ...namespaces.map((ns) => rowLabel(ns).length), 5);
  const columnWidth = (locale, values) =>
    Math.max(locale.length, ...values.map((v) => String(v).length), 4) + 2;

  const sourceTotal = namespaces.reduce((sum, ns) => sum + sourceNamespaces.get(ns).size, 0);
  const sourceValues = [...namespaces.map((ns) => sourceNamespaces.get(ns).size), sourceTotal];
  const sourceWidth = columnWidth(SOURCE_LOCALE, sourceValues);

  const localeTotals = locales.map((locale) =>
    namespaces.reduce((sum, ns) => sum + reports.get(locale).missingByNamespace.get(ns), 0),
  );
  const localeWidths = locales.map((locale, index) =>
    columnWidth(locale, [
      ...namespaces.map((ns) => reports.get(locale).missingByNamespace.get(ns)),
      localeTotals[index],
    ]),
  );

  const lines = [];
  lines.push(
    pad("namespace", nameWidth, "left") +
      pad(SOURCE_LOCALE, sourceWidth) +
      locales.map((locale, i) => pad(locale, localeWidths[i])).join(""),
  );

  const ruleWidth = nameWidth + sourceWidth + localeWidths.reduce((a, b) => a + b, 0);
  const rule = "-".repeat(ruleWidth);
  lines.push(rule);

  for (const ns of namespaces) {
    lines.push(
      pad(rowLabel(ns), nameWidth, "left") +
        pad(sourceNamespaces.get(ns).size, sourceWidth) +
        locales
          .map((locale, i) => pad(reports.get(locale).missingByNamespace.get(ns), localeWidths[i]))
          .join(""),
    );
  }

  lines.push(rule);
  lines.push(
    pad("TOTAL", nameWidth, "left") +
      pad(sourceTotal, sourceWidth) +
      localeTotals.map((total, i) => pad(total, localeWidths[i])).join(""),
  );
  return lines.join("\n");
}

const sourceNamespaces = readLocaleNamespaces(LOCALES_DIR, SOURCE_LOCALE);
const locales = discoverRealLocales(LOCALES_DIR);
const reports = new Map(
  locales.map((locale) => [
    locale,
    localeReport(sourceNamespaces, readLocaleNamespaces(LOCALES_DIR, locale)),
  ]),
);

// Say what was measured over, always. A tool that reports "0 missing" without
// naming its denominator is indistinguishable from one that scanned nothing.
const sourceKeyCount = [...sourceNamespaces.values()].reduce((sum, m) => sum + m.size, 0);
console.log(
  `scanned ${sourceNamespaces.size} namespace(s) / ${sourceKeyCount} ${SOURCE_LOCALE} key(s) ` +
    `across ${locales.length} locale(s) from ${LOCALES_DIR}`,
);

if (sourceNamespaces.size === 0) {
  console.log(`\nNo ${SOURCE_LOCALE} catalog found — nothing to compare against.`);
} else if (locales.length === 0) {
  console.log(`\nNo real locales found (${SOURCE_LOCALE} and pseudo are excluded).`);
} else {
  console.log(`\nMISSING KEYS — the ${SOURCE_LOCALE} column is the denominator, not a finding.\n`);
  console.log(renderTable(sourceNamespaces, locales, reports));

  const absent = locales.flatMap((locale) =>
    reports.get(locale).absentNamespaces.map((ns) => `${locale} / ${ns}: namespace file absent`),
  );
  if (absent.length) {
    console.log(`\nABSENT NAMESPACES — counted above as every key they would hold.\n`);
    for (const line of absent) console.log(`  ${line}`);
  }

  const extras = locales.flatMap((locale) => [
    ...reports.get(locale).extraNamespaces.map((ns) => `${locale} / ${ns}: whole namespace`),
    ...reports.get(locale).extraKeys.map((entry) => `${locale} / ${entry}`),
  ]);
  console.log(
    `\nEXTRA KEYS — present in the locale, absent from ${SOURCE_LOCALE}; ` +
      `a rename or deletion nobody propagated.\n`,
  );
  if (extras.length) {
    for (const line of extras) console.log(`  ${line}`);
    console.log(`\n  ${extras.length} extra entr(ies).`);
  } else {
    console.log("  (none)");
  }
}

// Advisory by design — see the header. Never exit non-zero on a finding.
process.exit(0);
