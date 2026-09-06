// Build the Kandev marketplace catalog index (index.json) from plugins.yaml.
//
// This is the "formulae.brew.sh"-style enrichment step of the plugin
// marketplace: it reads the curated pointer list (plugins.yaml), resolves each
// entry against the GitHub API to discover its latest release, package asset,
// star count, last-push time, and manifest presentation metadata, and emits a
// single static index.json document. That document is the fetch contract Kandev
// consumes (docs/specs/plugins/requirements/marketplace.md → "Data model" → index.json);
// additional corporate/team sources serve the same shape.
//
// The script itself uses only Node stdlib + global fetch. Archive validation is
// deliberately delegated to the repository's Go package verifier so registry
// publication and supported host installation share the same integrity rules.
// plugins.yaml is a small, schema-constrained pointer list, so a focused parser
// reads it without pulling a YAML library into CI.
//
// Robustness: one bad entry retains its prior validated record while unrelated
// valid releases advance. A failure without a trusted prior, or an all-failed
// build, aborts before index.json is replaced. A repo whose star lookup fails
// is emitted with `stars: null`, never `0`, so a transient outage cannot
// corrupt the catalog's ranking.
//
// Auth: GitHub API calls use GITHUB_TOKEN when present (in CI, secrets.GITHUB_TOKEN).
// That works for the public repos here; at larger scale a PAT with `public_repo`
// scope set as GITHUB_TOKEN gives higher, more predictable rate limits.

import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { execFile } from "node:child_process";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const GITHUB_API =
  process.env.PLUGIN_REGISTRY_GITHUB_API || "https://api.github.com";
const API_VERSION = "2022-11-28";
const USER_AGENT = "kandev-plugin-registry-index-builder";

// Bump only with a coordinated backend parser change — this is a hard contract.
const SCHEMA_VERSION = 1;
const SOURCE_NAME = "Kandev Official";
// The canonical Pages URL is filled in by the client from its source config, so
// the document does not need to know where it is hosted.
const SOURCE_URL = "";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PLUGINS_YAML =
  process.env.PLUGIN_REGISTRY_PLUGINS_YAML || path.join(HERE, "plugins.yaml");
const OUTPUT_JSON =
  process.env.PLUGIN_REGISTRY_OUTPUT || path.join(HERE, "index.json");
const RAW_BASE =
  process.env.PLUGIN_REGISTRY_RAW_BASE || "https://raw.githubusercontent.com";
const MAX_PACKAGE_DOWNLOAD_SIZE = 200 << 20;
const SAFE_PLUGIN_ID = /^[a-z0-9][a-z0-9-]*$/;
const SAFE_VERSION = /^[0-9A-Za-z][0-9A-Za-z.+-]*$/;
const execFileAsync = promisify(execFile);

// --- Minimal plugins.yaml parser --------------------------------------------

/**
 * Parse the curated plugins list. plugins.yaml is intentionally a flat
 * `plugins:` sequence of maps with only scalar / inline-array fields
 * (id, repo, featured, categories), enforced by plugin-registry/schema.json —
 * so this focused reader is sufficient and avoids a YAML dependency in CI.
 *
 * @param {string} text Raw plugins.yaml contents.
 * @returns {Array<Record<string, unknown>>} The parsed plugin specs.
 */
export function parsePluginsYaml(text) {
  const specs = [];
  let current = null;
  let inPlugins = false;

  for (const rawLine of text.split("\n")) {
    const line = stripComment(rawLine);
    if (line.trim() === "") continue;

    if (!inPlugins) {
      if (line.trim() === "plugins:") inPlugins = true;
      continue;
    }

    const item = line.match(/^\s*-\s*(.*)$/);
    if (item) {
      current = {};
      specs.push(current);
      assignField(current, item[1]);
      continue;
    }
    if (current && /^\s+\S/.test(line)) assignField(current, line.trim());
  }
  return specs;
}

/** Drop a trailing `# comment` (values here never contain `#`). */
function stripComment(line) {
  const hash = line.indexOf("#");
  return hash === -1 ? line : line.slice(0, hash);
}

/** Parse a `key: value` fragment onto target, coercing scalars/inline arrays. */
function assignField(target, fragment) {
  const colon = fragment.indexOf(":");
  if (colon === -1) return;
  const key = fragment.slice(0, colon).trim();
  if (!key) return;
  target[key] = parseScalar(fragment.slice(colon + 1).trim());
}

function parseScalar(value) {
  if (value === "") return "";
  if (value === "true") return true;
  if (value === "false") return false;
  if (value.startsWith("[") && value.endsWith("]")) {
    return value
      .slice(1, -1)
      .split(",")
      .map((part) => unquote(part.trim()))
      .filter((part) => part !== "");
  }
  return unquote(value);
}

function unquote(value) {
  if (
    value.length >= 2 &&
    (value[0] === '"' || value[0] === "'") &&
    value[value.length - 1] === value[0]
  ) {
    return value.slice(1, -1);
  }
  return value;
}

// --- GitHub helpers ----------------------------------------------------------

async function githubJson(apiPath) {
  const url = apiPath.startsWith("http") ? apiPath : `${GITHUB_API}${apiPath}`;
  const headers = {
    Accept: "application/vnd.github+json",
    "X-GitHub-Api-Version": API_VERSION,
    "User-Agent": USER_AGENT,
  };
  if (process.env.GITHUB_TOKEN)
    headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
  const response = await fetchWithTimeout(url, { headers });
  if (!response.ok) throw new Error(`GET ${url} -> ${response.status}`);
  return response.json();
}

async function fetchWithTimeout(url, options = {}, timeoutMs = 30000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...options, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

const rawUrl = (repo, ref, filePath) =>
  `${RAW_BASE}/${repo}/${ref}/${filePath.replace(/^\/+/, "")}`;

/**
 * Fetch the plugin's manifest.yaml at the release tag for presentation metadata
 * (display_name/description/categories/min_kandev_version/icon). Returns {} on
 * any failure — the manifest only enriches presentation, so a miss must never
 * fail the entry.
 */
async function fetchManifest(repo, ref) {
  try {
    const response = await fetchWithTimeout(
      rawUrl(repo, ref, "manifest.yaml"),
      {
        headers: { "User-Agent": USER_AGENT },
      },
    );
    if (!response.ok) throw new Error(`status ${response.status}`);
    return parseManifestFields(await response.text());
  } catch (error) {
    console.error(
      `warning: ${repo}: manifest.yaml not read (${error.message})`,
    );
    return {};
  }
}

/**
 * Read just the top-level manifest fields the index needs. The manifest is a
 * larger YAML doc, so this only extracts the flat scalar keys we care about
 * (display_name, description, author, min_kandev_version, icon) plus the
 * `categories` inline/simple list — enough for presentation without a YAML lib.
 */
export function parseManifestFields(text) {
  const out = {};
  const scalarKeys = [
    "display_name",
    "description",
    "author",
    "min_kandev_version",
    "icon",
  ];
  let inCategoryBlock = false;
  for (const rawLine of text.split("\n")) {
    const line = stripComment(rawLine);
    // Collect indented block-sequence items while inside `categories:`.
    const item = line.match(/^\s+-\s*(.+)$/);
    if (inCategoryBlock && item) {
      out.categories.push(unquote(item[1].trim()));
      continue;
    }
    const match = line.match(/^([a-z_]+):\s*(.*)$/);
    if (!match) continue;
    inCategoryBlock = false;
    const [, key, value] = match;
    const v = value.trim();
    if (scalarKeys.includes(key) && v !== "") {
      out[key] = unquote(v);
    } else if (key === "categories") {
      if (v.startsWith("[")) {
        out.categories = parseScalar(v); // inline: [a, b]
      } else {
        out.categories = []; // block sequence: subsequent `- item` lines
        inCategoryBlock = true;
      }
    }
  }
  return out;
}

// --- Enrichment --------------------------------------------------------------

/** Strip a leading `v` from a release tag so versions compare cleanly. */
export function normalizeVersion(tag) {
  return tag && /^v\d/.test(tag) ? tag.slice(1) : tag;
}

/** Keep release versions within the package path and archive naming contract. */
export function isSafeVersion(version) {
  return SAFE_VERSION.test(String(version || ""));
}

function parseVersion(version) {
  if (!isSafeVersion(version)) return null;
  const [withoutBuild] = version.split("+", 1);
  const prereleaseSeparator = withoutBuild.indexOf("-");
  const corePart =
    prereleaseSeparator === -1
      ? withoutBuild
      : withoutBuild.slice(0, prereleaseSeparator);
  const prereleasePart =
    prereleaseSeparator === -1
      ? undefined
      : withoutBuild.slice(prereleaseSeparator + 1);
  return {
    core: corePart.split("."),
    prerelease: prereleasePart?.split(".") || [],
  };
}

function compareVersionPart(left, right) {
  const leftNumeric = /^\d+$/.test(left);
  const rightNumeric = /^\d+$/.test(right);
  if (leftNumeric && rightNumeric) {
    const leftTrimmed = left.replace(/^0+(?=\d)/, "");
    const rightTrimmed = right.replace(/^0+(?=\d)/, "");
    if (leftTrimmed.length !== rightTrimmed.length)
      return leftTrimmed.length > rightTrimmed.length ? 1 : -1;
    return leftTrimmed === rightTrimmed
      ? 0
      : leftTrimmed > rightTrimmed
        ? 1
        : -1;
  }
  if (leftNumeric !== rightNumeric) return leftNumeric ? -1 : 1;
  return left === right ? 0 : left > right ? 1 : -1;
}

/** Compare the same safe version format accepted by buildEntry. */
export function compareVersions(left, right) {
  const a = parseVersion(left);
  const b = parseVersion(right);
  if (!a) return -1;
  if (!b) return 1;
  const coreCount = Math.max(a.core.length, b.core.length);
  for (let index = 0; index < coreCount; index += 1) {
    const order = compareVersionPart(
      a.core[index] || "0",
      b.core[index] || "0",
    );
    if (order !== 0) return order;
  }
  if (a.prerelease.length === 0 || b.prerelease.length === 0) {
    return a.prerelease.length === b.prerelease.length
      ? 0
      : a.prerelease.length === 0
        ? 1
        : -1;
  }
  const prereleaseCount = Math.max(a.prerelease.length, b.prerelease.length);
  for (let index = 0; index < prereleaseCount; index += 1) {
    const leftPart = a.prerelease[index];
    const rightPart = b.prerelease[index];
    if (leftPart === undefined || rightPart === undefined)
      return leftPart === undefined ? -1 : 1;
    const order = compareVersionPart(leftPart, rightPart);
    if (order !== 0) return order;
  }
  return 0;
}

/** Pick this plugin's package tarball from the release assets. */
function pickPackageAsset(assets, pluginId, version) {
  const expectedName = `${pluginId}-${version}.tar.gz`;
  const exact = assets.find((asset) => asset.name === expectedName);
  if (!exact?.browser_download_url)
    return { error: `release has no exact asset ${expectedName}` };
  return { asset: exact };
}

/** "agent-stats" -> "Agent Stats", the fallback when no manifest name is known. */
function humanize(pluginId) {
  return pluginId
    .split("-")
    .filter(Boolean)
    .map((part) => part[0].toUpperCase() + part.slice(1))
    .join(" ");
}

/**
 * Resolve a single plugins.yaml spec into a full index.json record.
 * @returns {Promise<{record?: object, error?: string}>}
 */
export async function buildEntry(
  spec,
  { verifyPackage = verifyReleasePackage } = {},
) {
  const pluginId = spec.id;
  const repo = spec.repo;
  if (!pluginId || !repo)
    return { error: `entry missing id/repo: ${JSON.stringify(spec)}` };
  if (!SAFE_PLUGIN_ID.test(pluginId)) {
    return { error: `${pluginId}: unsafe curated plugin ID` };
  }

  let release;
  try {
    release = await githubJson(`/repos/${repo}/releases/latest`);
  } catch (error) {
    return { error: `${pluginId}: no latest release (${error.message})` };
  }

  const tag = release.tag_name || "";
  const version = normalizeVersion(tag);
  if (!isSafeVersion(version)) {
    return {
      error: `${pluginId}: unsafe release version ${version || "<missing>"}`,
    };
  }
  const { asset, error: assetError } = pickPackageAsset(
    release.assets || [],
    pluginId,
    version,
  );
  if (assetError) return { error: `${pluginId}: ${assetError}` };

  let verified;
  try {
    verified = await verifyPackage({
      asset,
      checksumAsset: (release.assets || []).find(
        (candidate) => candidate.name === "checksums.txt",
      ),
      pluginId,
      version,
    });
  } catch (error) {
    return {
      error: `${pluginId}: package verification failed (${error.message})`,
    };
  }
  if (verified.id !== pluginId || verified.version !== version) {
    return {
      error:
        `${pluginId}: verified package identity ${verified.id}@${verified.version} ` +
        `does not match curated release ${pluginId}@${version}`,
    };
  }
  if (!/^[a-f0-9]{64}$/.test(verified.sha256 || "")) {
    return {
      error: `${pluginId}: package verifier returned an invalid SHA-256 digest`,
    };
  }

  const manifest = tag ? await fetchManifest(repo, tag) : {};
  const iconUrl =
    manifest.icon && tag ? rawUrl(repo, tag, manifest.icon) : null;
  const meta = await fetchRepoMeta(repo, pluginId);

  const record = {
    id: pluginId,
    // Presentation prefers the plugin's manifest, then id-derived / release /
    // plugins.yaml fallbacks, so the contract shape is stable even on a miss.
    name: manifest.display_name || humanize(pluginId),
    description: manifest.description || release.name || "",
    author: manifest.author || meta.author,
    categories: manifest.categories || spec.categories || [],
    icon_url: iconUrl,
    repo_url: `https://github.com/${repo}`,
    version: version || null,
    min_kandev_version: manifest.min_kandev_version ?? null,
    package_url: asset.browser_download_url,
    package_sha256: verified.sha256,
    stars: meta.stars,
    updated_at: meta.updatedAt || release.published_at || null,
  };
  return { record };
}

async function verifyReleasePackage({
  asset,
  checksumAsset,
  pluginId,
  version,
}) {
  const packageBytes = await fetchBytes(
    asset.browser_download_url,
    MAX_PACKAGE_DOWNLOAD_SIZE,
  );
  const tempDir = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-plugin-verify-"),
  );
  const archivePath = path.join(tempDir, asset.name);
  try {
    await fs.writeFile(archivePath, packageBytes, { mode: 0o600 });
    const verifier =
      process.env.PLUGIN_PACKAGE_VERIFIER ||
      path.join(HERE, "..", "apps", "backend", "bin", "plugin-package-verify");
    const { stdout } = await execFileAsync(
      verifier,
      [
        "--archive",
        archivePath,
        "--expected-id",
        pluginId,
        "--expected-version",
        version,
      ],
      { maxBuffer: 1 << 20, timeout: 60_000, killSignal: "SIGKILL" },
    );
    const result = JSON.parse(stdout);
    if (checksumAsset?.browser_download_url) {
      const checksumText = (
        await fetchBytes(checksumAsset.browser_download_url, 1 << 20)
      ).toString("utf8");
      const expected = checksumForAsset(checksumText, asset.name);
      if (!expected)
        throw new Error(`checksums.txt has no digest for ${asset.name}`);
      if (expected !== result.sha256)
        throw new Error(`release checksum mismatch for ${asset.name}`);
    }
    return result;
  } finally {
    await fs.rm(tempDir, { recursive: true, force: true });
  }
}

async function fetchBytes(url, maxBytes) {
  const response = await fetchWithTimeout(url);
  if (!response.ok) throw new Error(`GET ${url} -> ${response.status}`);
  return readResponseBytes(response, maxBytes);
}

export async function readResponseBytes(response, maxBytes) {
  const declaredSize = Number(response.headers?.get?.("content-length") || 0);
  if (declaredSize > maxBytes)
    throw new Error(`download exceeds ${maxBytes} bytes`);
  if (!response.body) return Buffer.alloc(0);

  const reader = response.body.getReader();
  const chunks = [];
  let received = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      received += value.byteLength;
      if (received > maxBytes) {
        await reader.cancel();
        throw new Error(`download exceeds ${maxBytes} bytes`);
      }
      chunks.push(Buffer.from(value));
    }
  } finally {
    reader.releaseLock();
  }
  return Buffer.concat(chunks, received);
}

function checksumForAsset(text, assetName) {
  for (const line of text.split("\n")) {
    const match = line.trim().match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (match && match[2] === assetName) return match[1].toLowerCase();
  }
  return null;
}

/** Repo metadata → stars (null on failure, never 0), last-push time, owner. */
async function fetchRepoMeta(repo, pluginId) {
  try {
    const repoMeta = await githubJson(`/repos/${repo}`);
    return {
      stars: Number.isInteger(repoMeta.stargazers_count)
        ? repoMeta.stargazers_count
        : null,
      updatedAt: repoMeta.pushed_at || null,
      author: repoMeta.owner?.login || repo.split("/", 1)[0],
    };
  } catch (error) {
    console.error(
      `warning: ${pluginId}: star/metadata lookup failed, emitting stars=null (${error.message})`,
    );
    return { stars: null, updatedAt: null, author: repo.split("/", 1)[0] };
  }
}

// --- Orchestration -----------------------------------------------------------

export async function buildIndex(
  specs,
  { priorDocument = null, verifyPackage, buildEntryFn = buildEntry } = {},
) {
  const records = [];
  const errors = [];
  const fatalErrors = [];
  const retained = [];
  let freshCount = 0;
  for (const spec of specs) {
    const { record, error } = await buildEntryFn(spec, { verifyPackage });
    if (error) {
      errors.push(error);
      const prior = trustedPriorRecord(priorDocument, spec);
      if (prior) {
        records.push(prior);
        retained.push(spec.id);
        console.error(`retain: ${error}`);
      } else {
        fatalErrors.push(`${error}; no trusted prior record for ${spec.id}`);
        console.error(`reject: ${error}`);
      }
      continue;
    }
    records.push(record);
    freshCount += 1;
  }
  const document = {
    schema_version: SCHEMA_VERSION,
    generated_at: new Date().toISOString().replace(/\.\d{3}Z$/, "Z"),
    source: { name: SOURCE_NAME, url: SOURCE_URL },
    plugins: records,
  };
  if (specs.length > 0 && freshCount === 0) {
    fatalErrors.push(
      "no fresh entries were built; refusing to replace the published catalog",
    );
  }
  return {
    document,
    errors,
    fatalErrors,
    retained,
    publishable: fatalErrors.length === 0,
  };
}

function trustedPriorRecord(priorDocument, spec) {
  if (
    priorDocument?.schema_version !== SCHEMA_VERSION ||
    !Array.isArray(priorDocument.plugins)
  ) {
    return null;
  }
  const matches = priorDocument.plugins.filter(
    (record) => record?.id === spec.id,
  );
  if (matches.length !== 1) return null;
  const prior = matches[0];
  if (
    prior.repo_url !== `https://github.com/${spec.repo}` ||
    typeof prior.version !== "string" ||
    typeof prior.package_url !== "string"
  ) {
    return null;
  }
  return prior;
}

async function main() {
  const text = await fs.readFile(PLUGINS_YAML, "utf8");
  const specs = parsePluginsYaml(text);
  const priorDocument = await readPriorDocument(
    process.env.PLUGIN_REGISTRY_PRIOR_INDEX,
  );
  // An empty list is expected at launch (no plugin repos yet) — it produces a
  // valid, empty index.json and is NOT an error. Only a non-empty list that
  // resolves to zero entries (below) indicates a real failure.
  const result = await buildIndex(specs, { priorDocument });

  await writeBuildReport(result, specs.length);
  if (!result.publishable) {
    for (const error of result.fatalErrors) emitWorkflowError(error);
    throw new Error(result.fatalErrors.join("; "));
  }
  await fs.writeFile(
    OUTPUT_JSON,
    `${JSON.stringify(result.document, null, 2)}\n`,
    "utf8",
  );

  console.error(
    `Built index.json: ${result.document.plugins.length} published, ${result.retained.length} retained, ` +
      `${result.errors.length} failed, ` +
      `${specs.length} listed. Output: ${OUTPUT_JSON}`,
  );
}

export async function readPriorDocument(priorPath) {
  if (!priorPath) return null;
  try {
    return JSON.parse(await fs.readFile(priorPath, "utf8"));
  } catch (error) {
    console.error(
      `warning: prior index at ${priorPath} is unusable; continuing without retention data (${error.message})`,
    );
    return null;
  }
}

async function writeBuildReport(result, listedCount) {
  const lines = [
    "## Plugin registry build",
    "",
    `- Listed: ${listedCount}`,
    `- Published: ${result.document.plugins.length}`,
    `- Retained last known-good: ${result.retained.length}`,
    `- Release failures: ${result.errors.length}`,
  ];
  for (const error of result.errors) {
    lines.push(`- ${error}`);
    emitWorkflowWarning(error);
  }
  const report = `${lines.join("\n")}\n`;
  if (process.env.GITHUB_STEP_SUMMARY)
    await fs.appendFile(process.env.GITHUB_STEP_SUMMARY, report);
  if (process.env.PLUGIN_REGISTRY_REPORT) {
    await fs.writeFile(process.env.PLUGIN_REGISTRY_REPORT, report, "utf8");
  }
}

function emitWorkflowWarning(message) {
  if (process.env.GITHUB_ACTIONS)
    console.error(`::warning::${escapeWorkflowCommand(message)}`);
}

function emitWorkflowError(message) {
  if (process.env.GITHUB_ACTIONS)
    console.error(`::error::${escapeWorkflowCommand(message)}`);
}

function escapeWorkflowCommand(message) {
  return String(message)
    .replaceAll("%", "%25")
    .replaceAll("\r", "%0D")
    .replaceAll("\n", "%0A");
}

// Run only when invoked directly (not when imported by a test).
if (
  process.argv[1] &&
  fileURLToPath(import.meta.url) === path.resolve(process.argv[1])
) {
  main().catch((error) => {
    console.error(`fatal: ${error.message}`);
    process.exitCode = 1;
  });
}
