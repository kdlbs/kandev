#!/usr/bin/env node
/**
 * Fail when `t()` is called at module scope.
 *
 * The module-level `t` from `@/lib/i18n` resolves when the module is imported —
 * before a locale is necessarily active, and exactly once. A `const ROWS = [{
 * label: t("k") }]` therefore pins that copy to the boot locale forever: the
 * strings render, so nothing looks broken, but switching language leaves them in
 * the old one. Module-scope JSX (`const BADGE = <span>{t("k")}</span>`) has the
 * same problem — the element is built once.
 *
 * The fix is always to defer: store the catalog key and resolve it at render
 * (see `lib/i18n/option-label.ts`), or turn the element into a component.
 *
 * The pseudo-locale cannot catch this class — the text *is* translated, just
 * frozen at the boot locale — which is why this check exists (docs/i18n.md).
 *
 * Usage: node scripts/check-module-scope-t.mjs [<dir> ...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
const DIRS = TARGETS.length ? TARGETS : ["components", "app", "hooks", "lib"];
const FUNCTIONS = new Set([
  "FunctionDeclaration",
  "FunctionExpression",
  "ArrowFunctionExpression",
  "ObjectMethod",
  "ClassMethod",
]);

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

/** A `t("key")` call that no enclosing function defers. */
function isModuleScopeT(node, ancestors) {
  if (node.type !== "CallExpression") return false;
  if (node.callee?.type !== "Identifier" || node.callee.name !== "t") return false;
  if (node.arguments[0]?.type !== "StringLiteral") return false;
  return !ancestors.some((a) => FUNCTIONS.has(a.type));
}

/** Walk carrying the ancestor chain so module scope can be told from a body. */
function visit(node, ancestors, source, rel) {
  if (!node || typeof node.type !== "string") return;
  if (isModuleScopeT(node, ancestors)) {
    const line = source.slice(0, node.start).split("\n").length;
    problems.push(`${rel}:${line}  t("${node.arguments[0].value}") at module scope`);
  }
  const next = ancestors.concat([node]);
  for (const key of Object.keys(node)) {
    if (key === "loc" || key.endsWith("Comments")) continue;
    const child = node[key];
    if (Array.isArray(child)) {
      for (const c of child) if (c && typeof c.type === "string") visit(c, next, source, rel);
    } else if (child && typeof child.type === "string") visit(child, next, source, rel);
  }
}

for (const dir of DIRS) {
  const abs = path.isAbsolute(dir) ? dir : path.join(ROOT, dir);
  if (!fs.existsSync(abs)) continue;
  for (const file of listFiles(abs)) {
    const source = fs.readFileSync(file, "utf8");
    if (!/\bt\(\s*"/.test(source)) continue;
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
    visit(ast, [], source, path.relative(ROOT, file));
  }
}

if (problems.length) {
  console.error(`✗ ${problems.length} module-scope t() call(s):\n`);
  for (const p of problems) console.error(`  ${p}`);
  console.error(
    `\nThese resolve once at import and never update on a locale switch.` +
      `\nStore the key and resolve it at render, or make the value a component.`,
  );
  process.exit(1);
}
console.log("✓ no module-scope t() — all copy resolves at render.");
