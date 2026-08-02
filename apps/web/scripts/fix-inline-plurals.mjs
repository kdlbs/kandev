#!/usr/bin/env node
/**
 * Replace English pluralization smuggled through an interpolation with real
 * i18next plural keys.
 *
 * `externalize-strings.mjs` turns
 *
 *   `${n} task${n === 1 ? "" : "s"}`
 *
 * into `t("ns:task", { length: n, value1: n === 1 ? "" : "s" })` with the
 * catalog message `{{length}} task{{value1}}`. That renders correctly in English
 * and is untranslatable everywhere else: the plural rule is baked into the call
 * site, and languages with three or six plural forms have nowhere to put them.
 *
 * i18next already does this — a key with `_one`/`_other` suffixes selected by a
 * `count` value. So this rewrites the pair into
 *
 *   t("ns:task", { count: n })
 *   // ns:task_one   = "{{count}} task"
 *   // ns:task_other = "{{count}} tasks"
 *
 * Only calls whose morphology argument is a conditional between two short
 * lowercase string literals are touched; anything else is reported and left
 * alone. `check-inline-plurals.mjs` fails the build if one reappears.
 *
 * LIMITATION: the flat key is deleted in favour of the `_one`/`_other` pair, so a
 * second call site sharing that key loses it — and NOTHING catches that for you.
 * `check-i18n-keys.mjs` treats a bare `ns:key` as satisfied by `_one`/`_other`
 * (see `isSatisfied`), while i18next does NOT fall back to the base key when the
 * call passes no `count` — so the other call site renders the raw key at runtime.
 * After running this, grep the converted keys for call sites without a `count`
 * argument and give them their own key.
 *
 * Usage: node scripts/fix-inline-plurals.mjs [--write] <dir|file> [...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const WRITE = process.argv.includes("--write");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
if (!TARGETS.length) {
  console.error("usage: fix-inline-plurals.mjs [--write] <dir|file> [...]");
  process.exit(2);
}

/** English plural morphemes — the only strings this codemod will absorb. */
const MORPHEME = /^(s|es|'s|)$/;

function walk(node, visit, parent = null) {
  if (!node || typeof node.type !== "string") return;
  visit(node, parent);
  for (const key of Object.keys(node)) {
    if (key === "loc" || key.endsWith("Comments")) continue;
    const child = node[key];
    if (Array.isArray(child)) {
      for (const c of child) if (c && typeof c.type === "string") walk(c, visit, node);
    } else if (child && typeof child.type === "string") walk(child, visit, node);
  }
}

function listFiles(target, out = []) {
  const abs = path.isAbsolute(target) ? target : path.join(ROOT, target);
  if (!fs.existsSync(abs)) return out;
  if (!fs.statSync(abs).isDirectory()) {
    if (/\.tsx?$/.test(abs)) out.push(abs);
    return out;
  }
  for (const e of fs.readdirSync(abs, { withFileTypes: true })) {
    const full = path.join(abs, e.name);
    if (e.isDirectory()) {
      if (["node_modules", "dist", "e2e", "locales"].includes(e.name)) continue;
      listFiles(full, out);
    } else if (/\.tsx?$/.test(e.name) && !/\.test\.tsx?$/.test(e.name)) out.push(full);
  }
  return out;
}

const catalogs = {};
function catalog(ns) {
  if (!catalogs[ns]) {
    const file = path.join(EN, `${ns}.json`);
    catalogs[ns] = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : {};
  }
  return catalogs[ns];
}

/**
 * Which branch of `n === 1 ? a : b` is the singular one.
 * `=== 1` / `== 1` puts singular first; `!== 1`, `> 1`, `>= 2` invert it.
 */
function singularBranch(test) {
  if (test.type !== "BinaryExpression") return null;
  const { operator, left, right } = test;
  const literal = [left, right].find((n) => n.type === "NumericLiteral");
  const counted = [left, right].find((n) => n.type !== "NumericLiteral");
  if (!literal || !counted) return null;
  // `===`/`!==` are commutative, so operand order does not matter. `>`/`>=` are
  // not: `1 > n` means the opposite of `n > 1`, and treating them alike would
  // silently swap the singular and plural messages.
  const literalOnRight = right === literal;
  if (literal.value === 1 && /^===?$/.test(operator)) return { which: "consequent", counted };
  if (literal.value === 1 && /^!==?$/.test(operator)) return { which: "alternate", counted };
  if (!literalOnRight) return null;
  if (literal.value === 1 && operator === ">") return { which: "alternate", counted };
  if (literal.value === 2 && operator === ">=") return { which: "alternate", counted };
  return null;
}

/** A `{ valueN: n === 1 ? "" : "s" }` property — an English plural ending. */
function isMorphemeProp(prop) {
  if (prop.type !== "ObjectProperty") return false;
  const v = prop.value;
  if (v.type !== "ConditionalExpression") return false;
  if (v.consequent.type !== "StringLiteral" || v.alternate.type !== "StringLiteral") return false;
  return MORPHEME.test(v.consequent.value) && MORPHEME.test(v.alternate.value);
}

/** The `{ … }` argument of a t() call, or null. */
function optionsArg(call) {
  const arg = call.arguments[1];
  return arg?.type === "ObjectExpression" ? arg : null;
}

const report = { rewritten: 0, files: 0, declined: [] };
const touched = new Set();

/** The existing value that already renders the count, so it can become {{count}}. */
function numberPropFor(options, morph, countExpr, source) {
  return options.properties.find(
    (p) =>
      p.type === "ObjectProperty" &&
      p !== morph &&
      source.slice(p.value.start, p.value.end) === countExpr,
  );
}

/**
 * The message with the morpheme slot marked and the number renamed to
 * `{{count}}` — i18next selects a plural form from `count` and nothing else.
 * Returns null when no placeholder renders the number.
 */
function withCountPlaceholder(message, morphName, numberProp) {
  let base = message.replaceAll(`{{${morphName}}}`, "@@MORPH@@");
  if (numberProp) {
    const numberName = String(numberProp.key.name ?? numberProp.key.value);
    base = base.replaceAll(`{{${numberName}}}`, "{{count}}");
  }
  return base.includes("{{count}}") ? base : null;
}

/** The rewritten values list: `count` first, then whatever else was passed. */
function remainingValues(options, morph, numberProp, countExpr, source) {
  const keep = options.properties.filter((p) => p !== morph && p !== numberProp);
  const parts = keep.map((p) => source.slice(p.start, p.end));
  parts.unshift(countExpr === "count" ? "count" : `count: ${countExpr}`);
  return parts;
}

/**
 * Plan the rewrite of one t() call, or null when it carries no inline plural.
 * Returns the source edit plus the `_one`/`_other` messages for the catalog.
 */
function planCall(call, source, rel) {
  if (call.callee.type !== "Identifier" || call.callee.name !== "t") return null;
  const keyNode = call.arguments[0];
  if (keyNode?.type !== "StringLiteral") return null;
  const options = optionsArg(call);
  if (!options) return null;

  const morph = options.properties.find(isMorphemeProp);
  if (!morph) return null;

  const qualified = keyNode.value;
  const [ns, key] = qualified.includes(":") ? qualified.split(":") : ["common", qualified];
  const line = source.slice(0, call.start).split("\n").length;
  const decline = (why) => {
    report.declined.push(`${rel}:${line} ${qualified} (${why})`);
    return null;
  };

  const message = catalog(ns)[key];
  if (typeof message !== "string") return decline("message not in catalog");
  const morphName = String(morph.key.name ?? morph.key.value);
  if (!message.includes(`{{${morphName}}}`)) return decline(`message has no {{${morphName}}}`);

  const picked = singularBranch(morph.value.test);
  if (!picked) return decline("cannot tell which branch is singular");
  const singular = morph.value[picked.which].value;
  const plural = morph.value[picked.which === "consequent" ? "alternate" : "consequent"].value;

  // The number driving the plural. i18next selects the form from `count`.
  const countExpr = source.slice(picked.counted.start, picked.counted.end);
  const numberProp = numberPropFor(options, morph, countExpr, source);
  const base = withCountPlaceholder(message, morphName, numberProp);
  if (!base) return decline("no placeholder renders the count");
  const parts = remainingValues(options, morph, numberProp, countExpr, source);

  return {
    ns,
    key,
    one: base.replace("@@MORPH@@", singular),
    other: base.replace("@@MORPH@@", plural),
    edit: {
      start: options.start,
      end: options.end,
      text: `{ ${parts.join(", ")} }`,
    },
  };
}

function transform(file) {
  const source = fs.readFileSync(file, "utf8");
  if (!source.includes('? "') && !source.includes("? '")) return;
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
    return;
  }

  const rel = path.relative(ROOT, file);
  const edits = [];
  walk(ast, (node) => {
    if (node.type !== "CallExpression") return;
    const plan = planCall(node, source, rel);
    if (!plan) return;
    const cat = catalog(plan.ns);
    delete cat[plan.key];
    cat[`${plan.key}_one`] = plan.one;
    cat[`${plan.key}_other`] = plan.other;
    touched.add(plan.ns);
    edits.push(plan.edit);
    report.rewritten += 1;
  });
  if (!edits.length) return;

  edits.sort((a, b) => b.start - a.start);
  let out = source;
  for (const e of edits) out = out.slice(0, e.start) + e.text + out.slice(e.end);
  report.files += 1;
  if (WRITE) fs.writeFileSync(file, out);
}

for (const target of TARGETS) for (const file of listFiles(target)) transform(file);

if (WRITE) {
  for (const ns of touched) {
    const sorted = Object.fromEntries(
      Object.entries(catalogs[ns]).sort(([a], [b]) => a.localeCompare(b)),
    );
    // Match the existing catalogs' escaping so the diff stays scoped to real
    // message changes rather than re-encoding every non-ASCII character.
    const json = JSON.stringify(sorted, null, 2).replace(
      /[\u0080-\uffff]/g,
      (ch) => `\\u${ch.charCodeAt(0).toString(16).padStart(4, "0")}`,
    );
    fs.writeFileSync(path.join(EN, `${ns}.json`), json + "\n");
  }
}
if (report.declined.length) {
  console.log(`Declined ${report.declined.length} — fix by hand:`);
  for (const d of report.declined) console.log(`  ${d}`);
}
console.log(JSON.stringify({ ...report, declined: report.declined.length, wrote: WRITE }, null, 2));
