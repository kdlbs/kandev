#!/usr/bin/env node
/**
 * Verify every `<Trans>` element's catalog message against its JSX children.
 *
 * react-i18next resolves a `<n>` tag in the message by index into the element's
 * runtime children, so the message and the JSX are coupled: edit the children
 * and every index after the edit silently repoints at a different node. The copy
 * does not blank out — it renders duplicated fragments with empty tags, which is
 * easy to miss and impossible to spot from the catalog alone.
 *
 * It also rejects a React element passed through `values`. i18next interpolation
 * stringifies its values, so `{{value0}}` with an element renders the literal
 * text `[object Object]` on screen — elements belong in the children, addressed
 * by an `<n>` tag. A string-valued placeholder is fine and is left alone.
 *
 * A message is wrong when a `<n>` it uses does not land on an element child.
 * That is the exact signature of stale indices, and it is what this check fails
 * on. It cannot judge whether `<1>` wraps the *intended* element — only that it
 * wraps an element at all — which is enough to catch every drift observed so far.
 *
 * It also rejects a message that still carries `<n>` tag markup while being
 * rendered through a plain `t()` call. `t()` returns a string, so the tags reach
 * the user verbatim — `task:color` shipped as the literal text "<0></0> Color".
 *
 * Usage: node scripts/check-trans-indices.mjs [<dir> ...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
const DIRS = TARGETS.length ? TARGETS : ["components", "app"];

const catalogs = {};
for (const file of fs.readdirSync(EN)) {
  if (!file.endsWith(".json")) continue;
  catalogs[file.replace(/\.json$/, "")] = JSON.parse(fs.readFileSync(path.join(EN, file), "utf8"));
}
function message(qualified) {
  const [ns, key] = qualified.includes(":") ? qualified.split(":") : ["common", qualified];
  const cat = catalogs[ns];
  if (!cat) return null;
  // Plural keys carry the suffix in the catalog but not at the call site.
  return cat[key] ?? cat[`${key}_other`] ?? cat[`${key}_one`] ?? null;
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

/**
 * The children React actually receives. JSX drops text that is whitespace-only
 * AND spans a line break, so a prettier-wrapped element list has fewer children
 * than the AST shows — and getting this wrong would shift every index.
 */
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
    } else if (/\.tsx?$/.test(entry.name) && !/\.(test|d)\.tsx?$/.test(entry.name)) out.push(full);
  }
  return out;
}

const problems = [];
let checked = 0;

/** A React element in `values` always renders as "[object Object]". */
function checkValuesForElements(node, key, where) {
  const attr = node.openingElement.attributes.find(
    (a) => a.type === "JSXAttribute" && a.name?.name === "values",
  );
  const object = attr?.value?.expression;
  if (object?.type !== "ObjectExpression") return;
  for (const prop of object.properties) {
    if (prop.type !== "ObjectProperty") continue;
    let hasJsx = false;
    walk(prop.value, (n) => {
      if (n.type === "JSXElement" || n.type === "JSXFragment") hasJsx = true;
    });
    if (!hasJsx) continue;
    const name = prop.key?.name ?? prop.key?.value;
    problems.push(
      `${where}  ${key}: values.${name} is a React element — it will render as "[object Object]"`,
    );
  }
}

/**
 * An auto-named `{{valueN}}` used twice means one expression was folded into two
 * places — typically a notice that then renders both as the sentence and as a
 * link's label. Named placeholders (`{{count}}`) legitimately repeat.
 */
function checkDuplicatePlaceholders(msg, key, where) {
  const seen = new Map();
  for (const [, name] of msg.matchAll(/\{\{(value\d+)\}\}/g)) {
    seen.set(name, (seen.get(name) ?? 0) + 1);
  }
  for (const [name, count] of seen) {
    if (count > 1) {
      problems.push(
        `${where}  ${key}: {{${name}}} appears ${count} times — one expression cannot be two ` +
          `different pieces of copy`,
      );
    }
  }
}

/** Every `<n>` in the message must land on an element child. */
function checkTagIndices(msg, children, key, where) {
  const indices = [...msg.matchAll(/<(\d+)>/g)].map((m) => Number(m[1]));
  for (const i of new Set(indices)) {
    const child = children[i];
    if (!child) problems.push(`${where}  ${key}: <${i}> has no child`);
    else if (child.type !== "JSXElement")
      problems.push(`${where}  ${key}: <${i}> is ${child.type}, not an element`);
  }
}

/**
 * A `<n>` tag is only meaningful inside `<Trans>`, which substitutes the child
 * at that position. `t()` returns a plain string, so the markup is rendered to
 * the user verbatim — `task:color` shipped as the literal text
 * "<0></0> Color" in the task context menu.
 *
 * This happens when a `<Trans>` is later flattened to a `t()` call (the icon
 * becomes a plain sibling) and the catalog message keeps its now-vestigial
 * tags. Nothing else catches it: the key exists and is referenced, so the
 * key/orphan check is happy, and the index check only inspects `<Trans>`.
 */
function checkPlainCallTags(node, source, rel) {
  if (node.type !== "CallExpression") return;
  if (node.callee?.type !== "Identifier" || !/^(t|translate)$/.test(node.callee.name)) return;
  const arg = node.arguments[0];
  if (arg?.type !== "StringLiteral") return;
  const [ns, key] = arg.value.includes(":") ? arg.value.split(":") : ["common", arg.value];
  const cat = catalogs[ns];
  if (!cat) return;
  // A count-bearing call resolves to the _one/_other form, so the flat entry is
  // shadowed and never rendered.
  const plural = `${key}_other` in cat || `${key}_one` in cat;
  const message = plural ? undefined : cat[key];
  if (typeof message !== "string" || !/<\/?\d+>/.test(message)) return;
  const line = source.slice(0, node.start).split("\n").length;
  problems.push(
    `${rel}:${line}  ${arg.value} renders <Trans> tag markup through t() — ` +
      `the user sees ${JSON.stringify(message)} verbatim`,
  );
}

/** The static i18nKey on a `<Trans>`, or null when it is computed. */
function transKey(node) {
  const attr = node.openingElement.attributes.find(
    (a) => a.type === "JSXAttribute" && a.name?.name === "i18nKey",
  );
  return attr?.value?.type === "StringLiteral" ? attr.value.value : null;
}

for (const dir of DIRS) {
  const abs = path.isAbsolute(dir) ? dir : path.join(ROOT, dir);
  if (!fs.existsSync(abs)) continue;
  for (const file of listFiles(abs)) {
    const src = fs.readFileSync(file, "utf8");
    const hasTrans = src.includes("<Trans");
    if (!hasTrans && !/\bt\(\s*"/.test(src)) continue;
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
    const rel = path.relative(ROOT, file);
    walk(ast, (node) => {
      checkPlainCallTags(node, src, rel);
      if (node.type !== "JSXElement" || node.openingElement?.name?.name !== "Trans") return;
      const key = transKey(node);
      if (!key) return; // dynamic key — nothing static to verify
      const msg = message(key);
      if (msg == null) return; // missing keys are check-i18n-keys.mjs's job
      checked += 1;
      const where = `${path.relative(ROOT, file)}:${src.slice(0, node.start).split("\n").length}`;
      checkValuesForElements(node, key, where);
      checkDuplicatePlaceholders(msg, key, where);
      checkTagIndices(msg, runtimeChildren(node), key, where);
    });
  }
}

if (problems.length) {
  console.error(`✗ ${problems.length} <Trans> tag problem(s):\n`);
  for (const p of problems) console.error(`  ${p}`);
  console.error(
    `\nFor an index mismatch: make the <n> indices match the element positions in` +
      `\nthe children, counting every child (text and expressions included), or run` +
      `\n\`node scripts/fix-trans-indices.mjs --write components app\`.` +
      `\n\nFor markup rendered through t(): the tags are vestigial — the icon is a` +
      `\nplain JSX sibling now, so delete the <n></n> pair from the message.`,
  );
  process.exit(1);
}
console.log(`✓ <Trans> indices OK — ${checked} element(s) checked.`);
