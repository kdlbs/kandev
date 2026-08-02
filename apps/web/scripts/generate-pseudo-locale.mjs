#!/usr/bin/env node
/**
 * Generate the `pseudo` catalog from the `en` source catalog.
 *
 * Every message is transliterated to accented look-alikes so that any text
 * still rendering as plain ASCII under the pseudo locale is, by definition, a
 * string that was never routed through `t()`. This is the completeness oracle
 * for the i18n sweep (see docs/i18n.md).
 *
 * `{{interpolation}}` placeholders and <0>tag</0> markers are preserved
 * verbatim — transforming them would break interpolation and Trans nesting.
 *
 * Covers the Go catalog too (apps/backend/internal/i18n/locales). The backend
 * renders its own user-facing copy — error pages and the shared-task page — so
 * leaving it out would make the pseudo locale a partial oracle, and the backend
 * has a test asserting every `en` key exists in `pseudo`.
 *
 * Usage: node scripts/generate-pseudo-locale.mjs
 */
import fs from "node:fs";
import path from "node:path";

const LOCALES = path.resolve(import.meta.dirname, "..", "src", "locales");
const SRC = path.join(LOCALES, "en");
const OUT = path.join(LOCALES, "pseudo");
const BACKEND_LOCALES = path.resolve(
  import.meta.dirname,
  "..",
  "..",
  "backend",
  "internal",
  "i18n",
  "locales",
);

const MAP = {
  a: "à",
  b: "ƀ",
  c: "ć",
  d: "ď",
  e: "ē",
  f: "ƒ",
  g: "ĝ",
  h: "ĥ",
  i: "ĩ",
  j: "ĵ",
  k: "ķ",
  l: "ĺ",
  m: "ḿ",
  n: "ń",
  o: "ō",
  p: "ƥ",
  q: "q",
  r: "ŕ",
  s: "ś",
  t: "ţ",
  u: "ũ",
  v: "v",
  w: "ŵ",
  x: "x",
  y: "ŷ",
  z: "ź",
  A: "À",
  B: "Ɓ",
  C: "Ć",
  D: "Ď",
  E: "Ē",
  F: "Ƒ",
  G: "Ĝ",
  H: "Ĥ",
  I: "Ĩ",
  J: "Ĵ",
  K: "Ķ",
  L: "Ĺ",
  M: "Ḿ",
  N: "Ń",
  O: "Ō",
  P: "Ƥ",
  Q: "Q",
  R: "Ŕ",
  S: "Ś",
  T: "Ţ",
  U: "Ũ",
  V: "V",
  W: "Ŵ",
  X: "X",
  Y: "Ŷ",
  Z: "Ź",
};

/** Accent letters outside {{...}} placeholders and <n>…</n> tag markers. */
function pseudolocalize(text) {
  // Split on placeholders/tags so their contents survive untouched.
  const parts = text.split(/(\{\{[^}]*\}\}|<\/?\d+>)/g);
  return parts
    .map((part, i) => (i % 2 === 1 ? part : part.replace(/[A-Za-z]/g, (c) => MAP[c] ?? c)))
    .join("");
}

function transform(value) {
  if (typeof value === "string") return pseudolocalize(value);
  if (Array.isArray(value)) return value.map(transform);
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, transform(v)]));
  }
  return value;
}

fs.mkdirSync(OUT, { recursive: true });
let files = 0;
let messages = 0;
for (const file of fs.readdirSync(SRC).filter((f) => f.endsWith(".json"))) {
  const source = JSON.parse(fs.readFileSync(path.join(SRC, file), "utf8"));
  const out = transform(source);
  fs.writeFileSync(path.join(OUT, file), JSON.stringify(out, null, 2) + "\n");
  files += 1;
  messages += Object.keys(source).length;
}
// The Go catalog is a single flat file per locale rather than a directory.
const backendSrc = path.join(BACKEND_LOCALES, "en.json");
let backendMessages = 0;
if (fs.existsSync(backendSrc)) {
  const source = JSON.parse(fs.readFileSync(backendSrc, "utf8"));
  fs.writeFileSync(
    path.join(BACKEND_LOCALES, "pseudo.json"),
    JSON.stringify(transform(source), null, 2) + "\n",
  );
  backendMessages = Object.keys(source).length;
}
console.log(
  `pseudo locale: ${files} namespace(s), ${messages} message(s)` +
    ` (+ ${backendMessages} backend message(s))`,
);
