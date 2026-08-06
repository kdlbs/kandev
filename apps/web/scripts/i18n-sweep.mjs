#!/usr/bin/env node
/**
 * Find user-facing copy the `i18next/no-literal-string` guard cannot see.
 *
 * The rule runs in `mode: "jsx-only"`, so it only inspects literals in JSX. Copy
 * assigned to a variable, used as a default parameter, held in a module-scope
 * config object, or built inside a plain `.ts` helper is invisible to it — and so
 * is an English plural morpheme, because `words.exclude` matches a bare "s".
 *
 * Two sections, deliberately separated:
 *
 *   1. English plural concatenation — `{n} file{n === 1 ? "" : "s"}`. A defect.
 *      No gate we own sees it: `no-literal-string` is jsx-only, and
 *      `check-inline-plurals.mjs` only inspects morphemes passed through `t()`.
 *   2. Prose literals in non-JSX positions — for eye review, not auto-flagged.
 *
 * This is a human tool, not a gate: it always exits 0 and is wired into neither
 * CI nor pre-commit. The eye-review half needs judgement, and a check that fires
 * on every clean run teaches people to ignore it.
 *
 * Usage: node scripts/i18n-sweep.mjs <dir> [<dir> ...]
 */
import fs from "node:fs";
import path from "node:path";

/**
 * Match EVERY literal, not just long ones: skipping short ones desynchronises
 * quote pairing and captures the gap between two strings as if it were copy.
 * On `if (size === "xs") return "h-6 w-6…"`, a length filter applied here drops
 * `"xs"`, pairs its closing quote with the next string's opening quote, and
 * reports `") return "` as copy. Length is filtered after the match, never
 * during it.
 */
const LITERAL = /"((?:[^"\\\n]|\\.)*)"/g;
/** Positions that are configuration or markup, never copy. */
const NOISE_CTX =
  /(className|classNames|data-testid|data-legacy-testid|viewBox|xmlns|setAttribute|stroke|d=|style=)/;
/**
 * Values that are identifiers/CSS/markup rather than prose.
 *
 * Note what this actually rejects, because it is broader than it looks and the
 * numbers below were validated with it in place: `[a-z0-9-]+` and
 * `[A-Z][A-Z0-9_]*` each match a single leading character before `.*$` swallows
 * the rest, so **any value starting with a letter or digit is dropped**. What
 * survives is prose starting with punctuation — bracketed log prefixes, media
 * queries, diff markers, bullet separators. The eye-review section is therefore
 * a narrow sample of non-JSX copy, not an inventory of it; a short section is
 * not evidence that a directory is clean. The pseudo-locale remains the
 * completeness check. Kept as-is deliberately: this is a faithful port, and
 * loosening it changes every count the tool has been calibrated against.
 */
const NOISE_VAL =
  /^(use (client|strict)|https?:\/\/|[a-z0-9-]+|[A-Z][A-Z0-9_]*|.*(?:rgba?\(|hsl\(|!important|px|rem)).*$/;
const TAILWIND =
  /(^|\s)(flex|grid|absolute|relative|h-|w-|p-|m-|text-|bg-|border|rounded|opacity-|transition|group|hover:|min-|max-)/;
/**
 * An English plural morpheme concatenated onto a count — forbidden, and invisible
 * to check-inline-plurals.mjs, which only inspects morphemes passed through t().
 */
const PLURAL = /\?\s*"(s|es)"\s*:\s*""|\?\s*""\s*:\s*"(s|es)"/;
/** Already a catalog key (`namespace:key`), so not a literal to migrate. */
const CATALOG_KEY = /\b\w+:[a-zA-Z]/;
/** Lines that cannot hold rendered copy. */
const SKIP_LINE = /^(import |\/\/|\*|\/\*)/;

const MIN_LENGTH = 4;

/** `str.isupper()` for a single character: has case, and that case is upper. */
function isUpper(char) {
  return char.toLowerCase() !== char.toUpperCase() && char === char.toUpperCase();
}

function isProse(value) {
  if (!/[A-Za-z]/.test(value)) return false;
  if (!(value.includes(" ") || isUpper(value[0]))) return false;
  if (TAILWIND.test(value) || NOISE_VAL.test(value)) return false;
  return true;
}

/**
 * Python `repr()` for a string, so output reads the same as the original tool:
 * single quotes, unless the value contains one and no double quote.
 */
function repr(value) {
  const escaped = value.replace(/\\/g, "\\\\");
  if (escaped.includes("'") && !escaped.includes('"')) return `"${escaped}"`;
  return `'${escaped.replace(/'/g, "\\'")}'`;
}

/**
 * Walk without following build output. The tool is pointed at source directories,
 * but `.` is a plausible mistake and node_modules would take minutes.
 */
function listFiles(dir, out = []) {
  let entries;
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return out;
  }
  for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (["node_modules", "dist", ".git"].includes(entry.name)) continue;
      listFiles(full, out);
    } else if (/\.tsx?$/.test(entry.name) && !entry.name.includes(".test.")) {
      out.push(full);
    }
  }
  return out;
}

function sweep(dirs) {
  const plurals = [];
  const hits = [];

  for (const dir of dirs) {
    for (const file of listFiles(dir)) {
      const lines = fs.readFileSync(file, "utf8").split("\n");
      lines.forEach((line, index) => {
        const lineNumber = index + 1;
        if (SKIP_LINE.test(line.trimStart())) return;
        if (PLURAL.test(line)) {
          plurals.push(`${file}:${lineNumber}: ${line.trim().slice(0, 100)}`);
        }
        for (const match of line.matchAll(LITERAL)) {
          const value = match[1];
          if (value.length < MIN_LENGTH || !isProse(value)) continue;
          if (NOISE_CTX.test(line.slice(0, match.index))) continue;
          if (CATALOG_KEY.test(value)) continue;
          hits.push(`${file}:${lineNumber}: ${repr(value)}`);
        }
      });
    }
  }

  return { plurals, hits };
}

function report({ plurals, hits }) {
  const out = [];
  out.push(
    `=== ${plurals.length} English plural concatenation(s) — forbidden, undetected by any gate ===`,
  );
  for (const p of plurals) out.push(`  ${p}`);
  out.push("", `=== ${hits.length} literal(s) to review by eye ===`);
  out.push(
    "  Note: prompt builders and other agent-facing text are deliberately English —",
    "  the content is sent verbatim to a model, so translating it would be a bug.",
    "  components/github/pr-checks-section.tsx and pr-ci-popover.tsx are known-good",
    "  examples. Judge each hit; nothing in this section is automatically a defect.",
  );
  for (const h of hits) out.push(`  ${h}`);
  return out.join("\n");
}

const DIRS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
if (!DIRS.length) {
  console.error("Usage: node scripts/i18n-sweep.mjs <dir> [<dir> ...]");
  process.exit(2);
}
console.log(report(sweep(DIRS)));
