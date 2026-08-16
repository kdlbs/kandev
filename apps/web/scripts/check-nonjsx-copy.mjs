#!/usr/bin/env node
/**
 * Fail on user-facing copy that `i18next/no-literal-string` cannot see.
 *
 * That rule is `jsx-only`, so a migrated file can hold a config table, a helper
 * that returns a label, a prop default, or a toast title in plain English and
 * still report clean. docs/i18n.md calls these the guard's blind spots and names
 * the pseudo-locale as the only completeness check — but the pseudo walk is
 * manual, and it cannot see copy that never becomes a text node or that is
 * interpolated into a translated frame. This closes the gap mechanically.
 *
 * SCOPE: only the paths in `i18nGuardFiles`, exactly like the eslint rule. A
 * file is judged whole once its directory is migrated, and never before. That is
 * the same ratchet the rest of this system uses — it can only tighten, and it
 * never asks anyone to migrate a line they did not write. The repo-wide half of
 * the contract is `check-new-code-i18n.mjs`, which runs this same detector over
 * added and changed lines everywhere.
 *
 * Silence a finding with `// i18n-exempt: <reason>` on the enclosing statement.
 * A reason is required, because every shape here has a legitimate form worth
 * telling apart from a defect: a value persisted in whichever locale created it,
 * a prompt sent verbatim to a model, a wire value matched with `===`.
 *
 * Usage: node scripts/check-nonjsx-copy.mjs [<glob> ...]
 */
import fs from "node:fs";
import path from "node:path";

import { i18nGuardFiles, noLiteralStringOptions } from "../eslint.i18n.options.mjs";
import { findNonJsxCopy } from "./lib/nonjsx-copy.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const patterns = process.argv.slice(2).filter((arg) => !arg.startsWith("--"));
const globs = patterns.length ? patterns : i18nGuardFiles;

const files = new Set();
for (const glob of globs) {
  for (const match of fs.globSync(glob, { cwd: ROOT })) {
    if (!/\.tsx?$/.test(match)) continue;
    if (/\.(test|spec|d)\.tsx?$/.test(match)) continue;
    files.add(match);
  }
}

const excludes = noLiteralStringOptions.words?.exclude ?? [];
const problems = [];
const unexplained = [];

for (const file of [...files].sort()) {
  const source = fs.readFileSync(path.join(ROOT, file), "utf8");
  const { findings, markersWithoutReason } = findNonJsxCopy(source, { filename: file, excludes });
  for (const finding of findings) problems.push({ file, ...finding });
  for (const line of markersWithoutReason) unexplained.push({ file, line });
}

if (unexplained.length) {
  console.error(`\n✖ ${unexplained.length} i18n-exempt marker(s) with no reason.\n`);
  for (const { file, line } of unexplained) {
    console.error(`  ${file}:${line}  write \`// i18n-exempt: <why this is not copy>\``);
  }
}

if (problems.length) {
  console.error(
    `\n✖ ${problems.length} hardcoded user-facing string(s) in a position lint cannot see.\n\n` +
      `  These sit on paths already listed in i18nGuardFiles, so the copy around\n` +
      `  them is translated and these are regressions. Route each through t(), or\n` +
      `  mark it \`// i18n-exempt: <reason>\` if it is a persisted value, an\n` +
      `  agent-facing prompt, or a wire value. See docs/i18n.md.\n`,
  );
  for (const { file, line, kind, context, value } of problems) {
    const where = context ? `${kind} ${context}` : kind;
    console.error(`  ${file}:${line}  [${where}]  ${JSON.stringify(value)}`);
  }
  console.error("");
}

if (problems.length || unexplained.length) process.exit(1);

console.log(`✓ no non-JSX copy — ${files.size} guarded file(s) checked.`);
