#!/usr/bin/env node
/**
 * Debt report for wall-clock waits in `apps/web/e2e`.
 *
 * **This is a report, not a gate.** It never exits non-zero on a count. The
 * conversion effort is actively *adding* `unverified` dwells as it replaces raw
 * sleeps with categorised ones — that is progress, because an `unverified`
 * dwell states its ignorance where a bare `waitForTimeout(500)` hid it — so a
 * ratchet on the number would fight the migration it exists to measure. The
 * gate is `check-new-e2e-sleeps.mjs`, and it gates the *form*, not the count.
 *
 * Three numbers, deliberately kept apart:
 *
 *   - **raw sleeps**, counted by the same eslint rule the gate uses, so the
 *     report and the gate cannot disagree about what a sleep is. This is what
 *     the conversion drives to zero, and reaching zero is what makes the
 *     graduation flip (rule to `"error"` on all of `e2e/**`) possible.
 *   - **dwell debt**, every categorised wait EXCEPT `negative-assertion`.
 *     `unverified` is called out inside it as the sub-total to attack first.
 *   - **injectLatency**, counted on its own. It is deliberate fixture
 *     configuration, correct by construction, and must never be converted; if
 *     it were folded into the sleep total, adding a legitimate latency
 *     simulation would look like a regression.
 *
 * `negative-assertion` is printed under a heading that says it is permanent.
 * A test asserting something never happens has no event to wait on, and
 * deleting its wait removes the window the regression needs in order to appear.
 *
 * Usage:  node scripts/report-e2e-waits.mjs [--json]
 */
import fs from "node:fs";
import path from "node:path";

import { ESLint } from "eslint";

import { SLEEP_EXEMPT_FILES, SLEEP_RULE_ID } from "../eslint-rules/no-unsanctioned-sleep.mjs";
import { inventorySource, readDwellCategories, summarize } from "./lib/e2e-wait-inventory.mjs";
import { WEB_DIR } from "./lib/git-base.mjs";

const E2E_DIR = path.join(WEB_DIR, "e2e");
const CAUSAL_WAITS = path.join(E2E_DIR, "helpers", "causal-waits.ts");

function collectTsFiles(dir) {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "node_modules" || entry.name === "test-results") continue;
      out.push(...collectTsFiles(full));
    } else if (entry.name.endsWith(".ts")) {
      out.push(full);
    }
  }
  return out;
}

const categories = readDwellCategories(fs.readFileSync(CAUSAL_WAITS, "utf8"));
const files = collectTsFiles(E2E_DIR);

// The helper's own definitions and its unit test would otherwise be counted as
// usage sites, inflating every category by the examples that define it. Same
// list the eslint configs use, so "exempt from the rule" and "not a usage site"
// cannot drift apart.
const exempt = new Set(SLEEP_EXEMPT_FILES.map((file) => path.join(WEB_DIR, file)));

// Sanctioned waits: our own AST walk, since the rule never sees them.
const inventories = files
  .filter((file) => !exempt.has(file))
  .map((file) => inventorySource(fs.readFileSync(file, "utf8"), file, categories));
const dwell = summarize(inventories, categories);

// Raw sleeps: the gate's own rule, so there is one definition of "a sleep".
const eslint = new ESLint({
  cwd: WEB_DIR,
  overrideConfigFile: path.join(WEB_DIR, "eslint.e2e-sleeps.config.mjs"),
  warnIgnored: false,
});
const results = await eslint.lintFiles(files);

const raw = { waitForTimeout: 0, promiseSleep: 0, byFile: {} };
for (const result of results) {
  const messages = result.messages.filter((message) => message.ruleId === SLEEP_RULE_ID);
  if (messages.length === 0) continue;
  raw.byFile[path.relative(WEB_DIR, result.filePath)] = messages.length;
  for (const message of messages) {
    if (message.messageId === "waitForTimeout") raw.waitForTimeout += 1;
    else raw.promiseSleep += 1;
  }
}
const rawTotal = raw.waitForTimeout + raw.promiseSleep;

if (process.argv.includes("--json")) {
  console.log(JSON.stringify({ raw: { ...raw, total: rawTotal }, dwell }, null, 2));
  process.exit(0);
}

const pad = (label) => label.padEnd(22);
console.log("\nE2E wall-clock wait inventory\n");

console.log(`  UNSANCTIONED — drive to zero, then graduate the rule to "error"`);
console.log(`    ${pad("waitForTimeout")}${raw.waitForTimeout}`);
console.log(`    ${pad("promise sleep")}${raw.promiseSleep}`);
console.log(`    ${pad("total")}${rawTotal}`);

console.log(`\n  DWELL DEBT — categorised, ${dwell.debt} of ${dwell.totalDwells} are debt`);
for (const category of categories) {
  if (category === "negative-assertion") continue;
  const count = dwell.byCategory[category] ?? 0;
  const note = category === "unverified" ? "   <- attack this first" : "";
  console.log(`    ${pad(category)}${count}${note}`);
}
if (dwell.unknown > 0) {
  console.log(`    ${pad("uncategorised")}${dwell.unknown}   <- miscategorised or unparsed`);
}

console.log(`\n  PERMANENT — not debt, do not drive down`);
console.log(`    ${pad("negative-assertion")}${dwell.permanent}`);

console.log(`\n  DELIBERATE STIMULUS — not a wait, counted apart from sleep debt`);
console.log(`    ${pad("injectLatency")}${dwell.injectLatency}`);

if (rawTotal > 0) {
  const worst = Object.entries(raw.byFile)
    .sort(([, a], [, b]) => b - a)
    .slice(0, 10);
  console.log(`\n  Top files by remaining raw sleeps (${Object.keys(raw.byFile).length} total):`);
  for (const [file, count] of worst) console.log(`    ${String(count).padStart(3)}  ${file}`);
}
console.log("");
