#!/usr/bin/env node
/**
 * Fail when English pluralization is smuggled through a `t()` interpolation.
 *
 *   t("ns:task", { length: n, value1: n === 1 ? "" : "s" })   // ✗
 *   t("ns:task", { count: n })                                // ✓ ns:task_one/_other
 *
 * The first form renders correctly in English and is untranslatable everywhere
 * else: the plural rule lives at the call site, so a language with three or six
 * plural forms has nowhere to express them, and a translator editing the catalog
 * cannot fix it. i18next already solves this with `_one`/`_other` keys selected
 * from `count`.
 *
 * This is the check for what `fix-inline-plurals.mjs` repairs. It also rejects a
 * catalog message that hardcodes the morpheme next to a placeholder
 * (`{{count}} task(s)` / `{{count}} tasks`), which is the same defect written
 * one layer down.
 *
 * Usage: node scripts/check-inline-plurals.mjs [<dir> ...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
const DIRS = TARGETS.length ? TARGETS : ["components", "app", "hooks", "lib"];

/** English plural morphemes an interpolation must never carry. */
const MORPHEME = /^(s|es|'s|)$/;

function walk(node, visit) {
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

function listFiles(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (["node_modules", "dist", "e2e", "locales"].includes(e.name)) continue;
      listFiles(full, out);
    } else if (/\.tsx?$/.test(e.name) && !/\.(test|d)\.tsx?$/.test(e.name)) out.push(full);
  }
  return out;
}

const problems = [];

/** `{ valueN: n === 1 ? "" : "s" }` on a t() call. */
function checkCall(node, source, rel) {
  if (node.type !== "CallExpression") return;
  if (node.callee.type !== "Identifier" || node.callee.name !== "t") return;
  const options = node.arguments[1];
  if (options?.type !== "ObjectExpression") return;
  for (const prop of options.properties) {
    if (prop.type !== "ObjectProperty") continue;
    const v = prop.value;
    if (
      v.type === "ConditionalExpression" &&
      v.consequent.type === "StringLiteral" &&
      v.alternate.type === "StringLiteral" &&
      MORPHEME.test(v.consequent.value) &&
      MORPHEME.test(v.alternate.value)
    ) {
      const line = source.slice(0, node.start).split("\n").length;
      const name = String(prop.key.name ?? prop.key.value);
      problems.push(
        `${rel}:${line}  {{${name}}} carries an English plural ending — use a ` +
          `\`count\` value with _one/_other keys instead`,
      );
    }
  }
}

for (const dir of DIRS) {
  const abs = path.isAbsolute(dir) ? dir : path.join(ROOT, dir);
  if (!fs.existsSync(abs)) continue;
  for (const file of listFiles(abs)) {
    const source = fs.readFileSync(file, "utf8");
    if (!source.includes('? "') && !source.includes("? '")) continue;
    let ast;
    try {
      ast = parseSync(source, {
        filename: file,
        babelrc: false,
        configFile: false,
        sourceType: "module",
        parserOpts: { plugins: ["typescript", "jsx"] },
      });
    } catch {
      continue;
    }
    const rel = path.relative(ROOT, file);
    walk(ast, (node) => checkCall(node, source, rel));
  }
}

/**
 * A message that writes the morpheme itself — `{{count}} task(s)` or a bare
 * `{{count}} tasks` on a key with no `_one` sibling — is the same bug in the
 * catalog rather than the call site.
 */
for (const file of fs.readdirSync(EN).filter((f) => f.endsWith(".json"))) {
  const ns = file.replace(/\.json$/, "");
  const cat = JSON.parse(fs.readFileSync(path.join(EN, file), "utf8"));
  for (const [key, message] of Object.entries(cat)) {
    if (typeof message !== "string" || !message.includes("{{count}}")) continue;
    if (/\(s\)|\(es\)/.test(message)) {
      problems.push(`${ns}:${key}  writes "(s)" — use _one/_other plural keys instead`);
    }
    if (/_(one|other|zero|two|few|many)$/.test(key)) continue;
    if (!(`${key}_one` in cat) && !(`${key}_other` in cat)) {
      problems.push(
        `${ns}:${key}  interpolates {{count}} but has no _one/_other form — ` +
          `every locale gets one wording for all counts`,
      );
    }
  }
}

if (problems.length) {
  console.error(`✗ ${problems.length} inline-plural problem(s):\n`);
  for (const p of problems) console.error(`  ${p}`);
  console.error(
    `\nRun \`node scripts/fix-inline-plurals.mjs --write components app hooks lib\`` +
      `\nto convert the call-site form automatically.`,
  );
  process.exit(1);
}
console.log("✓ no inline English plurals — count-bearing messages use _one/_other.");
