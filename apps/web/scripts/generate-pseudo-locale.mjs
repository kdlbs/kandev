#!/usr/bin/env node
/**
 * Generate the `pseudo` catalog from the `en` source catalog.
 *
 * Every message is transliterated to accented look-alikes so that any text
 * still rendering as plain ASCII under the pseudo locale is, by definition, a
 * string that was never routed through `t()`. This is the completeness oracle
 * for the i18n sweep (see docs/i18n.md).
 *
 * `{{interpolation}}` placeholders, <0>tag</0> markers, and Markdown code spans
 * are preserved verbatim. Code spans can contain callable names or commands.
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
import { transform } from "./lib/pseudo-locale.mjs";

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
