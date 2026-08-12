#!/usr/bin/env node
/**
 * Convert zh-cn catalogs to zh-tw / zh-hk (OpenCC + product glossary).
 *
 * Usage:
 *   node scripts/convert-zh-cn-to-zh-hant.mjs [--locale zh-tw|zh-hk|all] [--namespace name]
 *   node scripts/convert-zh-cn-to-zh-hant.mjs --write [--backend]
 *   node scripts/convert-zh-cn-to-zh-hant.mjs --write --locale all
 *
 * Dry-run (default) prints a summary; --write materializes JSON catalogs.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  TARGET_LOCALES,
  convertCatalog,
  loadGlossary,
  residualSimplifiedInCatalog,
} from "./lib/zh-hant-convert.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const WEB_LOCALES = path.resolve(HERE, "..", "src", "locales");
const BACKEND_LOCALES = path.resolve(HERE, "..", "..", "backend", "internal", "i18n", "locales");
const SOURCE_LOCALE = "zh-cn";

function parseArgs(argv) {
  const args = {
    locale: "all",
    namespace: null,
    write: false,
    backend: false,
  };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--write") args.write = true;
    else if (arg === "--backend") args.backend = true;
    else if (arg === "--locale") args.locale = argv[++i];
    else if (arg === "--namespace") args.namespace = argv[++i];
    else if (arg === "--help" || arg === "-h") args.help = true;
    else throw new Error(`unknown argument: ${arg}`);
  }
  return args;
}

function resolveLocales(localeArg) {
  if (localeArg === "all") return [...TARGET_LOCALES];
  if (!TARGET_LOCALES.includes(localeArg)) {
    throw new Error(`--locale must be one of ${TARGET_LOCALES.join("|")}|all`);
  }
  return [localeArg];
}

function listNamespaces(sourceDir, namespaceFilter) {
  const files = fs
    .readdirSync(sourceDir)
    .filter((name) => name.endsWith(".json"))
    .map((name) => name.replace(/\.json$/, ""))
    .sort();
  if (!namespaceFilter) return files;
  if (!files.includes(namespaceFilter)) {
    throw new Error(`namespace ${namespaceFilter} not found in ${sourceDir}`);
  }
  return [namespaceFilter];
}

function writeJson(filePath, data) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, `${JSON.stringify(data, null, 2)}\n`, "utf8");
}

function convertWeb({ locales, namespace, write }) {
  const sourceDir = path.join(WEB_LOCALES, SOURCE_LOCALE);
  const namespaces = listNamespaces(sourceDir, namespace);
  let totalKeys = 0;
  let residual = 0;

  for (const locale of locales) {
    for (const ns of namespaces) {
      const sourcePath = path.join(sourceDir, `${ns}.json`);
      const messages = JSON.parse(fs.readFileSync(sourcePath, "utf8"));
      const converted = convertCatalog(messages, locale);
      const hits = residualSimplifiedInCatalog(converted);
      totalKeys += Object.keys(converted).length;
      residual += hits.length;
      if (hits.length > 0) {
        console.warn(
          `[warn] ${locale}/${ns}: ${hits.length} key(s) still look simplified (e.g. ${hits.slice(0, 3).join(", ")})`,
        );
      }
      if (write) {
        writeJson(path.join(WEB_LOCALES, locale, `${ns}.json`), converted);
      }
    }
    console.log(
      `${write ? "wrote" : "would write"} web ${locale}: ${namespaces.length} namespace(s)`,
    );
  }
  return { totalKeys, residual };
}

function convertBackend({ locales, write }) {
  const sourcePath = path.join(BACKEND_LOCALES, `${SOURCE_LOCALE}.json`);
  if (!fs.existsSync(sourcePath)) {
    throw new Error(`missing backend source catalog: ${sourcePath}`);
  }
  const messages = JSON.parse(fs.readFileSync(sourcePath, "utf8"));
  let residual = 0;
  for (const locale of locales) {
    const converted = convertCatalog(messages, locale);
    const hits = residualSimplifiedInCatalog(converted);
    residual += hits.length;
    if (hits.length > 0) {
      console.warn(`[warn] backend ${locale}: residual simplified in ${hits.join(", ")}`);
    }
    if (write) {
      writeJson(path.join(BACKEND_LOCALES, `${locale}.json`), converted);
    }
    console.log(
      `${write ? "wrote" : "would write"} backend ${locale}: ${Object.keys(converted).length} key(s)`,
    );
  }
  return { residual };
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    console.log(`Usage: node scripts/convert-zh-cn-to-zh-hant.mjs [options]
  --locale zh-tw|zh-hk|all   Target locale (default all)
  --namespace <name>         Only one web namespace
  --write                    Materialize files (default dry-run)
  --backend                  Also convert backend i18n catalog
`);
    return;
  }

  loadGlossary();
  const locales = resolveLocales(args.locale);
  const web = convertWeb({ locales, namespace: args.namespace, write: args.write });
  let backendResidual = 0;
  if (args.backend || args.write) {
    // Always convert backend when writing full catalogs so FE/BE stay in sync.
    const backend = convertBackend({ locales, write: args.write });
    backendResidual = backend.residual;
  }

  console.log(
    `done: ${web.totalKeys} web message(s); residual-simplified warnings: web=${web.residual} backend=${backendResidual}`,
  );
  if (!args.write) {
    console.log("(dry-run; pass --write to materialize)");
  }
}

main();
