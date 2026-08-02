#!/usr/bin/env node
/**
 * Repoint stale `<n>` tags in `<Trans>` catalog messages at the right children.
 *
 * The wording in these messages is correct; only the indices drifted, because
 * editing a `<Trans>` element's children renumbers every child after the edit
 * while the catalog keeps the old numbers. So the fix is a remap, not a rewrite:
 * take the tags in the order they appear in the message and point them at the
 * element children in the order they appear in the JSX.
 *
 * Declines — and reports — anything where that correspondence is not obviously
 * right: a different number of tags than element children, or nested tags. Those
 * need a human to decide which element each phrase belongs to.
 *
 * Usage: node scripts/fix-trans-indices.mjs [--write] [<dir> ...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const WRITE = process.argv.includes("--write");
const args = process.argv.slice(2).filter((a) => !a.startsWith("--"));
const DIRS = args.length ? args : ["components", "app"];

const catalogs = {};
for (const file of fs.readdirSync(EN)) {
  if (!file.endsWith(".json")) continue;
  catalogs[file.replace(/\.json$/, "")] = JSON.parse(fs.readFileSync(path.join(EN, file), "utf8"));
}
/** Every catalog entry a call-site key may resolve to (plurals included). */
function entriesFor(qualified) {
  const [ns, key] = qualified.includes(":") ? qualified.split(":") : ["common", qualified];
  const cat = catalogs[ns];
  if (!cat) return [];
  return [key, `${key}_one`, `${key}_other`].filter((k) => k in cat).map((k) => [ns, k]);
}

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

/** JSX drops text that is whitespace-only and spans a line break. */
const runtimeChildren = (element) =>
  element.children.filter(
    (c) => !(c.type === "JSXText" && c.value.trim() === "" && c.value.includes("\n")),
  );

function listFiles(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (["node_modules", "dist", "e2e", "locales"].includes(entry.name)) continue;
      listFiles(full, out);
    } else if (/\.tsx$/.test(entry.name) && !/\.test\.tsx$/.test(entry.name)) out.push(full);
  }
  return out;
}

const report = { fixed: 0, alreadyOk: 0, declinedCount: 0, declinedNested: 0 };
const declined = [];
const touched = new Set();

/**
 * Repoint one catalog message's tags at `elementIdx`, or record why it can't be
 * done automatically.
 */
function remapMessage(ns, catKey, children, elementIdx, where) {
  const msg = catalogs[ns][catKey];
  if (typeof msg !== "string") return;
  const opens = [...msg.matchAll(/<(\d+)>/g)].map((m) => Number(m[1]));
  if (!opens.length) return;
  if (opens.every((i) => children[i]?.type === "JSXElement")) {
    report.alreadyOk += 1;
    return;
  }
  // Nested tags need a human: the remap below assumes a flat sequence.
  if (/<(\d+)>(?:(?!<\/\1>).)*<\d+>/s.test(msg)) {
    report.declinedNested += 1;
    declined.push(`${where}  ${ns}:${catKey} — nested tags`);
    return;
  }
  const distinct = [...new Set(opens)];
  if (distinct.length !== elementIdx.length) {
    report.declinedCount += 1;
    declined.push(
      `${where}  ${ns}:${catKey} — ${distinct.length} tag(s) but ` +
        `${elementIdx.length} element child(ren)`,
    );
    return;
  }
  const remap = new Map(distinct.map((old, i) => [old, elementIdx[i]]));
  const next = msg.replace(/<(\/?)(\d+)>/g, (m, slash, n) => {
    const to = remap.get(Number(n));
    return to === undefined ? m : `<${slash}${to}>`;
  });
  if (next !== msg) {
    catalogs[ns][catKey] = next;
    touched.add(ns);
    report.fixed += 1;
  }
}

for (const dir of DIRS) {
  const abs = path.isAbsolute(dir) ? dir : path.join(ROOT, dir);
  if (!fs.existsSync(abs)) continue;
  for (const file of listFiles(abs)) {
    const src = fs.readFileSync(file, "utf8");
    if (!src.includes("<Trans")) continue;
    let ast;
    try {
      ast = parseSync(src, {
        filename: file,
        babelrc: false,
        configFile: false,
        sourceType: "module",
        parserOpts: { plugins: ["typescript", "jsx"] },
      });
    } catch {
      continue;
    }
    walk(ast, (node) => {
      if (node.type !== "JSXElement" || node.openingElement?.name?.name !== "Trans") return;
      const keyAttr = node.openingElement.attributes.find(
        (a) => a.type === "JSXAttribute" && a.name?.name === "i18nKey",
      );
      const key = keyAttr?.value?.type === "StringLiteral" ? keyAttr.value.value : null;
      if (!key) return;
      const rel = path.relative(ROOT, file);
      const line = src.slice(0, node.start).split("\n").length;
      const children = runtimeChildren(node);
      const elementIdx = children
        .map((c, i) => (c.type === "JSXElement" ? i : -1))
        .filter((i) => i >= 0);

      for (const [ns, catKey] of entriesFor(key)) {
        remapMessage(ns, catKey, children, elementIdx, `${rel}:${line}`);
      }
    });
  }
}

if (WRITE) {
  for (const ns of touched) {
    const sorted = Object.fromEntries(
      Object.entries(catalogs[ns]).sort(([a], [b]) => a.localeCompare(b)),
    );
    fs.writeFileSync(path.join(EN, `${ns}.json`), JSON.stringify(sorted, null, 2) + "\n");
  }
}
if (declined.length) {
  console.log(`Declined ${declined.length} message(s) — fix these by hand:`);
  for (const d of declined) console.log(`  ${d}`);
  console.log("");
}
console.log(JSON.stringify({ ...report, wrote: WRITE }, null, 2));
