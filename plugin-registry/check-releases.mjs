// Detect newer releases for repositories already curated in plugins.yaml.
// This script intentionally accepts no repository or release-payload input:
// the checked-out allowlist is the only source of repositories it queries.

import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parsePluginsYaml } from "./build-index.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PLUGINS_YAML =
  process.env.PLUGIN_REGISTRY_PLUGINS_YAML || path.join(HERE, "plugins.yaml");
const OFFICIAL_INDEX =
  process.env.PLUGIN_REGISTRY_INDEX_URL ||
  "https://kdlbs.github.io/kandev/plugins/index.json";
const GITHUB_API =
  process.env.PLUGIN_REGISTRY_GITHUB_API || "https://api.github.com";
const USER_AGENT = "kandev-plugin-registry-release-detector";

export async function detectReleaseChanges(
  specs,
  currentIndex,
  { fetchLatestRelease = githubLatestRelease } = {},
) {
  if (
    currentIndex?.schema_version !== 1 ||
    !Array.isArray(currentIndex.plugins)
  ) {
    throw new Error(
      "current official index must use schema_version 1 with a plugins array",
    );
  }

  const candidates = [];
  const errors = [];
  const slaBreaches = [];
  for (const spec of specs) {
    let release;
    try {
      release = await fetchLatestRelease(spec);
    } catch (error) {
      errors.push(
        `${spec.id}: latest release lookup failed (${error.message})`,
      );
      continue;
    }
    const version = normalizeVersion(release?.tag_name);
    if (!parseSemver(version)) {
      errors.push(
        `${spec.id}: latest release tag ${release?.tag_name || "<missing>"} is not SemVer`,
      );
      continue;
    }
    const current = currentIndex.plugins.find(
      (record) =>
        record?.id === spec.id &&
        record.repo_url === `https://github.com/${spec.repo}`,
    );
    if (current && compareSemver(version, current.version) <= 0) continue;

    const expectedAsset = `${spec.id}-${version}.tar.gz`;
    const exactAsset = (release.assets || []).find(
      (asset) => asset.name === expectedAsset && asset.browser_download_url,
    );
    if (!exactAsset) {
      errors.push(
        `${spec.id}: newer release has no exact asset ${expectedAsset}`,
      );
      continue;
    }
    candidates.push(`${spec.id}@${version}`);
    if (
      release.published_at &&
      Date.now() - Date.parse(release.published_at) > 10 * 60 * 1000
    ) {
      slaBreaches.push(`${spec.id}@${version}`);
    }
  }
  return { rebuild: candidates.length > 0, candidates, errors, slaBreaches };
}

async function githubLatestRelease(spec) {
  const response = await fetch(
    `${GITHUB_API}/repos/${spec.repo}/releases/latest`,
    {
      headers: githubHeaders(),
      signal: AbortSignal.timeout(30_000),
    },
  );
  if (!response.ok) throw new Error(`GitHub API returned ${response.status}`);
  return response.json();
}

function githubHeaders() {
  const headers = {
    Accept: "application/vnd.github+json",
    "X-GitHub-Api-Version": "2022-11-28",
    "User-Agent": USER_AGENT,
  };
  if (process.env.GITHUB_TOKEN)
    headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
  return headers;
}

function normalizeVersion(tag) {
  return tag && /^v\d/.test(tag) ? tag.slice(1) : tag;
}

function parseSemver(version) {
  const match = String(version || "").match(
    /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/,
  );
  if (!match) return null;
  return {
    core: match.slice(1, 4),
    prerelease: match[4]?.split(".") || [],
  };
}

function compareNumericIdentifier(left, right) {
  if (left.length !== right.length) return left.length > right.length ? 1 : -1;
  return left === right ? 0 : left > right ? 1 : -1;
}

function compareSemver(left, right) {
  const a = parseSemver(left);
  const b = parseSemver(right);
  if (!a) return -1;
  if (!b) return 1;
  for (let index = 0; index < 3; index += 1) {
    const order = compareNumericIdentifier(a.core[index], b.core[index]);
    if (order !== 0) return order;
  }
  if (a.prerelease.length === 0 || b.prerelease.length === 0) {
    return a.prerelease.length === b.prerelease.length
      ? 0
      : a.prerelease.length === 0
        ? 1
        : -1;
  }
  const count = Math.max(a.prerelease.length, b.prerelease.length);
  for (let index = 0; index < count; index += 1) {
    const leftPart = a.prerelease[index];
    const rightPart = b.prerelease[index];
    if (leftPart === undefined || rightPart === undefined)
      return leftPart === undefined ? -1 : 1;
    if (leftPart === rightPart) continue;
    const leftNumeric = /^\d+$/.test(leftPart);
    const rightNumeric = /^\d+$/.test(rightPart);
    if (leftNumeric && rightNumeric)
      return compareNumericIdentifier(leftPart, rightPart);
    if (leftNumeric !== rightNumeric) return leftNumeric ? -1 : 1;
    return leftPart > rightPart ? 1 : -1;
  }
  return 0;
}

async function main() {
  const specs = parsePluginsYaml(await fs.readFile(PLUGINS_YAML, "utf8"));
  const currentResponse = await fetch(OFFICIAL_INDEX, {
    headers: { "User-Agent": USER_AGENT },
    signal: AbortSignal.timeout(30_000),
  });
  if (!currentResponse.ok)
    throw new Error(`official index returned ${currentResponse.status}`);
  const result = await detectReleaseChanges(
    specs,
    await currentResponse.json(),
  );

  if (process.env.GITHUB_OUTPUT) {
    await fs.appendFile(
      process.env.GITHUB_OUTPUT,
      `rebuild=${result.rebuild ? "true" : "false"}\n`,
    );
  }
  await emitSummary(result, specs.length);
  for (const error of result.errors) emitAnnotation("warning", error);
  for (const release of result.slaBreaches) {
    emitAnnotation(
      "warning",
      `${release} exceeded the 10-minute publication SLO before detection`,
    );
  }
  if (result.errors.length > 0 && !result.rebuild) {
    throw new Error(
      "release detection failed without a publishable curated candidate",
    );
  }
}

async function emitSummary(result, listedCount) {
  if (!process.env.GITHUB_STEP_SUMMARY) return;
  const lines = [
    "## Curated plugin release poll",
    "",
    `- Allowlisted repositories checked: ${listedCount}`,
    `- Rebuild requested: ${result.rebuild ? "yes" : "no"}`,
    `- Candidates: ${result.candidates.length ? result.candidates.join(", ") : "none"}`,
    `- Lookup or release errors: ${result.errors.length}`,
    `- 10-minute SLO breaches at detection: ${result.slaBreaches.length}`,
    "",
  ];
  await fs.appendFile(process.env.GITHUB_STEP_SUMMARY, lines.join("\n"));
}

function emitAnnotation(level, message) {
  if (!process.env.GITHUB_ACTIONS) return;
  const escaped = String(message)
    .replaceAll("%", "%25")
    .replaceAll("\r", "%0D")
    .replaceAll("\n", "%0A");
  console.error(`::${level}::${escaped}`);
}

if (
  process.argv[1] &&
  fileURLToPath(import.meta.url) === path.resolve(process.argv[1])
) {
  main().catch((error) => {
    emitAnnotation("error", error.message);
    console.error(`fatal: ${error.message}`);
    process.exitCode = 1;
  });
}
