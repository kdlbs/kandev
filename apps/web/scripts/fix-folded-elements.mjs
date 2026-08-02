#!/usr/bin/env node
/**
 * Un-fold React elements that were wrapped into a `<Trans>` `values` entry.
 *
 * `wrap-trans.mjs` turned an icon beside a label into an interpolation:
 *
 *   <Trans i18nKey="task:approve" values={{ value0: <IconCheck /> }}>
 *     <IconCheck /> Approve
 *   </Trans>
 *   // catalog: "{{value0}} Approve"
 *
 * i18next interpolation stringifies its values, so a React element becomes the
 * literal text `[object Object]` on screen. Elements belong in the children,
 * addressed by an `<n>` tag — never in `values`.
 *
 * A decorative icon does not belong in the message at all, so this rewrites the
 * element to a plain sibling and the copy to a single `t()` call:
 *
 *   <IconCheck />
 *   {t("task:approve")}
 *   // catalog: "Approve"
 *
 * Only `<Trans>` elements whose `values` hold JSX are touched; a string-valued
 * `{{valueN}}` interpolates correctly and is left alone.
 *
 * Usage: node scripts/fix-folded-elements.mjs [--write] <dir> [...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const WRITE = process.argv.includes("--write");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
if (!TARGETS.length) {
  console.error("usage: fix-folded-elements.mjs [--write] <dir> [...]");
  process.exit(2);
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

const containsJsx = (node) => {
  let found = false;
  walk(node, (n) => {
    if (n.type === "JSXElement" || n.type === "JSXFragment") found = true;
  });
  return found;
};

function listFiles(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (["node_modules", "dist"].includes(entry.name)) continue;
      listFiles(full, out);
    } else if (/\.tsx$/.test(entry.name) && !/\.test\.tsx$/.test(entry.name)) out.push(full);
  }
  return out;
}

const catalogs = {};
function catalogFor(ns) {
  if (!catalogs[ns]) {
    const file = path.join(EN, `${ns}.json`);
    catalogs[ns] = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : {};
  }
  return catalogs[ns];
}

const report = { rewritten: 0, files: 0, declined: [] };
const touchedNamespaces = new Set();

/** Strip `{{valueN}}` (and any tag that wrapped only it) from a message. */
function stripFolded(message, names) {
  let out = message;
  for (const name of names) {
    // A tag whose entire content was the placeholder goes with it.
    out = out.replace(new RegExp(`<(\\d+)>\\s*\\{\\{${name}\\}\\}\\s*</\\1>`, "g"), "");
    out = out.replace(new RegExp(`\\{\\{${name}\\}\\}`, "g"), "");
  }
  return out.replace(/\s{2,}/g, " ").trim();
}

/** `values` entries holding JSX — those are what render as "[object Object]". */
function foldedElementProps(node) {
  const attr = node.openingElement.attributes.find(
    (a) => a.type === "JSXAttribute" && a.name?.name === "values",
  );
  const object = attr?.value?.expression;
  if (object?.type !== "ObjectExpression") return null;
  const folded = object.properties.filter(
    (p) =>
      p.type === "ObjectProperty" && /^value\d+$/.test(String(p.key?.name)) && containsJsx(p.value),
  );
  if (!folded.length) return null;
  return { folded, others: object.properties.filter((p) => !folded.includes(p)) };
}

/** A `{t(...)}` child IS the copy; the replacement t() call supersedes it. */
function carriesCopy(child) {
  let found = false;
  walk(child, (n) => {
    if (n.type === "CallExpression" && n.callee?.type === "Identifier" && n.callee.name === "t")
      found = true;
  });
  return found;
}

/**
 * How to rewrite one `<Trans>`, or null when it should be left to a human.
 * Returns the source edit plus the message the catalog should end up with.
 */
function planRewrite(node, original, rel) {
  const keyAttr = node.openingElement.attributes.find(
    (a) => a.type === "JSXAttribute" && a.name?.name === "i18nKey",
  );
  if (keyAttr?.value?.type !== "StringLiteral") return null;
  const props = foldedElementProps(node);
  if (!props) return null;

  const qualified = keyAttr.value.value;
  const [ns, key] = qualified.includes(":") ? qualified.split(":") : ["common", qualified];
  const line = original.slice(0, node.start).split("\n").length;
  const decline = (why) => {
    report.declined.push(`${rel}:${line} ${qualified} (${why})`);
    return null;
  };

  // Any other interpolation must survive, so a mixed case needs a human.
  if (props.others.length) return decline("other values present");
  const kept = node.children.filter(
    (c) => (c.type === "JSXElement" || c.type === "JSXExpressionContainer") && !carriesCopy(c),
  );
  if (!kept.length) return decline("no element child to keep");

  const message = catalogFor(ns)[key];
  if (typeof message !== "string") return decline("message not in catalog");
  const stripped = stripFolded(
    message,
    props.folded.map((p) => String(p.key.name)),
  );
  if (!stripped) return decline("nothing left after stripping");

  return {
    ns,
    key,
    message: stripped,
    edit: {
      start: node.start,
      end: node.end,
      text: `${kept.map((c) => original.slice(c.start, c.end)).join("\n      ")}\n      {t("${qualified}")}`,
    },
  };
}

function transform(file) {
  const original = fs.readFileSync(file, "utf8");
  if (!original.includes("<Trans")) return;
  let ast;
  try {
    ast = parseSync(original, {
      filename: file,
      babelrc: false,
      configFile: false,
      sourceType: "module",
      parserOpts: { plugins: ["typescript", "jsx"] },
    });
  } catch {
    return;
  }

  const edits = [];
  walk(ast, (node) => {
    if (node.type !== "JSXElement" || node.openingElement?.name?.name !== "Trans") return;
    const plan = planRewrite(node, original, path.relative(ROOT, file));
    if (!plan) return;
    edits.push(plan.edit);
    catalogs[plan.ns][plan.key] = plan.message;
    touchedNamespaces.add(plan.ns);
    report.rewritten += 1;
  });

  if (!edits.length) return;
  edits.sort((a, b) => b.start - a.start);
  let out = original;
  let lastStart = Infinity;
  for (const e of edits) {
    if (e.end > lastStart) continue;
    out = out.slice(0, e.start) + e.text + out.slice(e.end);
    lastStart = e.start;
  }
  report.files += 1;
  if (WRITE) fs.writeFileSync(file, out);
}

for (const target of TARGETS) {
  const abs = path.isAbsolute(target) ? target : path.join(ROOT, target);
  if (!fs.existsSync(abs)) continue;
  for (const file of fs.statSync(abs).isDirectory() ? listFiles(abs) : [abs]) transform(file);
}

if (WRITE) {
  for (const ns of touchedNamespaces) {
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
console.log(
  JSON.stringify({ rewritten: report.rewritten, files: report.files, wrote: WRITE }, null, 2),
);
