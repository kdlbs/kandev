#!/usr/bin/env node
/**
 * Migration fidelity: every string a change REMOVES from source must reappear
 * verbatim in a catalog.
 *
 * A directory migration replaces literals with `t()` calls. If the English a
 * user saw is gone from the source and is not a catalog value either, then the
 * copy silently CHANGED — and nothing else in the toolchain notices:
 *
 *   live surface  : "Create a diagnostic ZIP with frontend and backend logs."
 *   reused key    : "Download a bounded diagnostic ZIP containing frontend and backend logs."
 *
 * That shape survives `pnpm lint`, all four `i18n:check` gates, the ratchet and
 * the whole unit suite, because every string IS externalized, the key DOES
 * exist and the catalogs ARE in sync. It happens when two surfaces render the
 * same copy, one of them was migrated earlier (often a dead route), and the
 * later migration adopts the existing, plausible-looking key. The pseudo-locale
 * cannot see it either: the text is translated, just into different words.
 *
 * This check compares the literals present before and after, so it reports at
 * the point of the edit and covers every route in the change automatically,
 * rather than relying on whichever one happens to have an e2e assertion
 * pinning its sentence.
 *
 * NOT part of `i18n:check`, and deliberately not in CI: it is diff-based, so it
 * needs a base ref, and a finding is a prompt to CLASSIFY rather than proof of
 * a bug. Benign findings are normal and expected — a value extracted into an
 * interpolation, a placeholder hoisted to a constant, a literal that was never
 * copy. It narrows the diff for a human; it does not decide.
 *
 * Usage: node scripts/check-removed-literals.mjs [--base <ref>] [--all]
 *
 *   --all   also report literals that look like identifiers/values, not just
 *           the ones that look like prose.
 */
import fs from "node:fs";
import path from "node:path";
import { git, resolveBase, REPO_ROOT } from "./lib/git-base.mjs";
import { changedFiles, WEB_PREFIX } from "./lib/changed-files.mjs";
import { accountedFor, buildCatalog, literals, looksLikeCopy } from "./lib/removed-literals.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const LOCALES = path.join(ROOT, "src", "locales", "en");
const REPORT_ALL = process.argv.includes("--all");

function catalogValues() {
  const out = new Set();
  for (const file of fs.readdirSync(LOCALES).filter((f) => f.endsWith(".json"))) {
    for (const value of Object.values(
      JSON.parse(fs.readFileSync(path.join(LOCALES, file), "utf8")),
    ))
      if (typeof value === "string") out.add(value);
  }
  return out;
}

function sideOf(ref, repoPath) {
  try {
    return git(["show", `${ref}:${repoPath}`], { quiet: true });
  } catch {
    return ""; // Absent on that side (added or deleted).
  }
}

const resolved = resolveBase();
if (resolved.skip) {
  console.log(`↷ skipped removed-literal check — ${resolved.skip}`);
  process.exit(0);
}
const { base } = resolved;

const values = catalogValues();
const catalog = buildCatalog(values);
const files = [
  ...new Set([...changedFiles(base, "M", REPO_ROOT), ...changedFiles(base, "D", REPO_ROOT)]),
];

const findings = [];
let unparseable = 0;
for (const repoPath of files) {
  const before = literals(sideOf(base, repoPath), repoPath);
  const after = literals(sideOf("HEAD", repoPath), repoPath) ?? new Set();
  if (before === null) {
    unparseable += 1;
    continue;
  }
  for (const value of before) {
    if (after.has(value)) continue;
    if (!REPORT_ALL && !looksLikeCopy(value)) continue;
    if (accountedFor(value, catalog)) continue;
    findings.push({ file: repoPath.slice(WEB_PREFIX.length), value });
  }
}

if (unparseable) console.log(`ℹ ${unparseable} file(s) could not be parsed on the base side.`);

if (findings.length === 0) {
  console.log(
    `✓ removed literals OK — every string ${files.length} changed file(s) dropped is still a catalog value.`,
  );
  process.exit(0);
}

console.log(
  `\nStrings removed from source that do NOT reappear verbatim in a catalog (${findings.length}):\n`,
);
for (const { file, value } of findings.sort(
  (a, b) => a.file.localeCompare(b.file) || a.value.localeCompare(b.value),
))
  console.log(`  ${file}\n    ${JSON.stringify(value)}\n`);
console.log(
  "Each is one of:\n" +
    "  - a VALUE extracted into an interpolation (reconstruct it and compare — it should be byte-identical)\n" +
    "  - a literal hoisted to a constant, or one that was never copy\n" +
    "  - COPY THAT CHANGED, because a reused key's English differs from what this surface rendered\n" +
    "\nThe last one is the bug this check exists for. Classify every entry before merging.\n",
);
process.exit(1);
