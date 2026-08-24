import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
export const DEFAULT_CATALOGUE_PATH = path.resolve(
  SCRIPT_DIR,
  "../apps/backend/internal/agent/agents/managed_npm_runtime_versions.json",
);
const STABLE_SEMVER = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$/;

export function parseStableVersion(value) {
  if (typeof value !== "string" || !STABLE_SEMVER.test(value)) {
    throw new Error(`value is not a stable SemVer: ${String(value)}`);
  }
  return value;
}

function assertCatalogue(catalogue) {
  if (!catalogue || typeof catalogue !== "object" || Array.isArray(catalogue)) {
    throw new Error("runtime pin catalogue must be a JSON object");
  }
  const packages = Object.keys(catalogue);
  if (packages.length === 0) {
    throw new Error("runtime pin catalogue is empty");
  }
  for (const packageName of packages) {
    if (!packageName.trim()) {
      throw new Error("runtime pin catalogue contains an empty package name");
    }
    parseStableVersion(catalogue[packageName]);
  }
}

export function updatePins(catalogue, latestByPackage) {
  assertCatalogue(catalogue);
  if (!latestByPackage || typeof latestByPackage !== "object") {
    throw new Error("latest version lookup did not return an object");
  }

  const next = {};
  let changed = false;
  for (const packageName of Object.keys(catalogue)) {
    const latest = latestByPackage[packageName];
    if (latest === undefined || latest === null || latest === "") {
      throw new Error(`latest version is missing for ${packageName}`);
    }
    parseStableVersion(latest);
    next[packageName] = latest;
    changed ||= latest !== catalogue[packageName];
  }
  return { catalogue: next, changed };
}

export function formatCatalogue(catalogue) {
  assertCatalogue(catalogue);
  return `${JSON.stringify(catalogue, Object.keys(catalogue).sort(), 2)}\n`;
}

async function fetchLatest(packageName, fetchImpl) {
  const response = await fetchImpl(
    `https://registry.npmjs.org/${encodeURIComponent(packageName)}`,
    { headers: { accept: "application/json" } },
  );
  if (!response.ok) {
    throw new Error(`npm lookup for ${packageName} returned HTTP ${response.status}`);
  }
  const metadata = await response.json();
  const latest = metadata?.["dist-tags"]?.latest;
  parseStableVersion(latest);
  return latest;
}

export async function fetchLatestVersions(packages, fetchImpl = globalThis.fetch) {
  if (typeof fetchImpl !== "function") {
    throw new Error("fetch is unavailable");
  }
  const pairs = await Promise.all(
    packages.map(async (packageName) => [packageName, await fetchLatest(packageName, fetchImpl)]),
  );
  return Object.fromEntries(pairs);
}

export async function readCatalogue(cataloguePath = DEFAULT_CATALOGUE_PATH) {
  const raw = await fs.readFile(cataloguePath, "utf8");
  const catalogue = JSON.parse(raw);
  assertCatalogue(catalogue);
  return catalogue;
}

export async function writeCatalogueAtomically(cataloguePath, catalogue) {
  const directory = path.dirname(cataloguePath);
  const temporaryPath = path.join(
    directory,
    `.${path.basename(cataloguePath)}.${process.pid}.tmp`,
  );
  await fs.writeFile(temporaryPath, formatCatalogue(catalogue), "utf8");
  await fs.rename(temporaryPath, cataloguePath);
}

export async function updateCatalogue({
  cataloguePath = DEFAULT_CATALOGUE_PATH,
  fetchImpl = globalThis.fetch,
} = {}) {
  const catalogue = await readCatalogue(cataloguePath);
  const latestByPackage = await fetchLatestVersions(Object.keys(catalogue), fetchImpl);
  const result = updatePins(catalogue, latestByPackage);
  if (result.changed) {
    await writeCatalogueAtomically(cataloguePath, result.catalogue);
  }
  return result;
}

function outputChanged(outputPath, changed) {
  if (!outputPath) {
    return;
  }
  return fs.appendFile(outputPath, `changed=${changed ? "true" : "false"}\n`);
}

export function parseArguments(argv) {
  let cataloguePath = DEFAULT_CATALOGUE_PATH;
  let outputPath = "";
  let positionalPathSeen = false;

  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--github-output") {
      outputPath = argv[index + 1] ?? "";
      if (!outputPath || outputPath.startsWith("--")) {
        throw new Error("--github-output requires a path");
      }
      index += 1;
      continue;
    }
    if (argument.startsWith("--")) {
      throw new Error(`unknown option: ${argument}`);
    }
    if (positionalPathSeen) {
      throw new Error("only one catalogue path is supported");
    }
    cataloguePath = argument;
    positionalPathSeen = true;
  }

  return { cataloguePath, outputPath };
}

async function main(argv = process.argv.slice(2)) {
  const { cataloguePath, outputPath } = parseArguments(argv);
  const result = await updateCatalogue({ cataloguePath });
  await outputChanged(outputPath, result.changed);
  process.stdout.write(result.changed ? "Managed runtime pins changed.\n" : "Managed runtime pins are current.\n");
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(`Managed runtime pin update failed: ${error.message}`);
    process.exitCode = 1;
  });
}
