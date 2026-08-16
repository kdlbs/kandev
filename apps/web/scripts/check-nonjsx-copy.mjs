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
 * SCOPE: the WHOLE `apps/web` source tree, by exclusion rather than by
 * allowlist. This used to default to `i18nGuardFiles` — the eslint rule's
 * allowlist — which left 1141 of 2564 source files judged by neither gate, and
 * non-JSX positions are exactly what eslint structurally cannot see. Since this
 * check has no lint-rule cost (it runs as one script, not per-PR-file), there is
 * no reason to scope it: an exclusion list covers new directories automatically,
 * so the hole cannot silently reopen the next time someone adds one.
 *
 * The eslint half of the contract is still `i18nGuardFiles`, and
 * `check-new-code-i18n.mjs` runs this same detector over added and changed lines
 * everywhere.
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

import { noLiteralStringOptions } from "../eslint.i18n.options.mjs";
import { findNonJsxCopy } from "./lib/nonjsx-copy.mjs";
import { collectScannedFiles } from "./lib/nonjsx-scope.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const patterns = process.argv.slice(2).filter((arg) => !arg.startsWith("--"));
const files = collectScannedFiles(fs, ROOT, patterns);

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
      `  \`i18next/no-literal-string\` is jsx-only, so it cannot reach these —\n` +
      `  config tables, helper returns, parameter defaults, and toast/setter\n` +
      `  arguments. Route each through t(), or mark it\n` +
      `  \`// i18n-exempt: <reason>\` if it is a persisted value, an agent-facing\n` +
      `  prompt, a wire value, or a diagnostic the user never sees.\n` +
      `  See docs/i18n.md.\n`,
  );
  for (const { file, line, kind, context, value } of problems) {
    const where = context ? `${kind} ${context}` : kind;
    console.error(`  ${file}:${line}  [${where}]  ${JSON.stringify(value)}`);
  }
  console.error("");
}

if (problems.length || unexplained.length) {
  // Report the scope on the failure path too. A shrinking scope and a clean run
  // look identical otherwise, and this check's whole value is the denominator.
  console.error(`  (${files.size} file(s) scanned)\n`);
  process.exit(1);
}

console.log(`✓ no non-JSX copy — ${files.size} guarded file(s) checked.`);
