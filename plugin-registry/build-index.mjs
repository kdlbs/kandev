// Build the Kandev marketplace catalog index (index.json) from plugins.yaml.
//
// This is the "formulae.brew.sh"-style enrichment step of the plugin
// marketplace: it reads the curated pointer list (plugins.yaml), resolves each
// entry against the GitHub API to discover its latest release, package asset,
// star count, last-push time, and manifest presentation metadata, and emits a
// single static index.json document. That document is the fetch contract Kandev
// consumes (docs/specs/plugins/marketplace.md → "Data model" → index.json);
// additional corporate/team sources serve the same shape.
//
// Zero dependencies: Node stdlib + global fetch only, matching the repo's other
// automation scripts (e.g. scripts/validate-public-docs.mjs). plugins.yaml is a
// small, schema-constrained pointer list (plugin-registry/schema.json), so a
// focused parser reads it without pulling a YAML library into CI.
//
// Robustness: one bad entry (missing release, deleted asset, API hiccup) never
// fails the whole build — its error is logged to stderr and it is skipped. A
// repo whose star lookup fails is emitted with `stars: null`, never `0`, so a
// transient outage can't corrupt the catalog's ranking.
//
// Auth: GitHub API calls use GITHUB_TOKEN when present (in CI, secrets.GITHUB_TOKEN).
// That works for the public repos here; at larger scale a PAT with `public_repo`
// scope set as GITHUB_TOKEN gives higher, more predictable rate limits.

import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const GITHUB_API = "https://api.github.com";
const API_VERSION = "2022-11-28";
const USER_AGENT = "kandev-plugin-registry-index-builder";

// Bump only with a coordinated backend parser change — this is a hard contract.
const SCHEMA_VERSION = 1;
const SOURCE_NAME = "Kandev Official";
// The canonical Pages URL is filled in by the client from its source config, so
// the document does not need to know where it is hosted.
const SOURCE_URL = "";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PLUGINS_YAML = path.join(HERE, "plugins.yaml");
const OUTPUT_JSON = path.join(HERE, "index.json");

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
  if (value.length >= 2 && (value[0] === '"' || value[0] === "'") && value[value.length - 1] === value[0]) {
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
  if (process.env.GITHUB_TOKEN) headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
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
  `https://raw.githubusercontent.com/${repo}/${ref}/${filePath.replace(/^\/+/, "")}`;

/**
 * Fetch the plugin's manifest.yaml at the release tag for presentation metadata
 * (display_name/description/categories/min_kandev_version/icon). Returns {} on
 * any failure — the manifest only enriches presentation, so a miss must never
 * fail the entry.
 */
async function fetchManifest(repo, ref) {
  try {
    const response = await fetchWithTimeout(rawUrl(repo, ref, "manifest.yaml"), {
      headers: { "User-Agent": USER_AGENT },
    });
    if (!response.ok) throw new Error(`status ${response.status}`);
    return parseManifestFields(await response.text());
  } catch (error) {
    console.error(`warning: ${repo}: manifest.yaml not read (${error.message})`);
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
  const scalarKeys = ["display_name", "description", "author", "min_kandev_version", "icon"];
  for (const rawLine of text.split("\n")) {
    const line = stripComment(rawLine);
    const match = line.match(/^([a-z_]+):\s*(.*)$/);
    if (!match) continue;
    const [, key, value] = match;
    if (scalarKeys.includes(key) && value.trim() !== "") out[key] = unquote(value.trim());
    else if (key === "categories" && value.trim().startsWith("[")) out.categories = parseScalar(value.trim());
  }
  return out;
}

// --- Enrichment --------------------------------------------------------------

/** Strip a leading `v` from a release tag so versions compare cleanly. */
function normalizeVersion(tag) {
  return tag && /^v\d/.test(tag) ? tag.slice(1) : tag;
}

/** Pick this plugin's package tarball from the release assets. */
function pickPackageAsset(assets, pluginId, version) {
  const tarballs = assets.filter((a) => (a.name || "").endsWith(".tar.gz"));
  if (tarballs.length === 0) return { error: "release has no .tar.gz asset" };
  const exact = tarballs.find((a) => a.name === `${pluginId}-${version}.tar.gz`);
  return { asset: exact || tarballs[0] };
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
export async function buildEntry(spec) {
  const pluginId = spec.id;
  const repo = spec.repo;
  if (!pluginId || !repo) return { error: `entry missing id/repo: ${JSON.stringify(spec)}` };

  let release;
  try {
    release = await githubJson(`/repos/${repo}/releases/latest`);
  } catch (error) {
    return { error: `${pluginId}: no latest release (${error.message})` };
  }

  const tag = release.tag_name || "";
  const version = normalizeVersion(tag);
  const { asset, error: assetError } = pickPackageAsset(release.assets || [], pluginId, version);
  if (assetError) return { error: `${pluginId}: ${assetError}` };

  const manifest = tag ? await fetchManifest(repo, tag) : {};
  const iconUrl = manifest.icon && tag ? rawUrl(repo, tag, manifest.icon) : null;
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
    // Advisory provenance digest; null is a valid, documented value.
    package_sha256: null,
    stars: meta.stars,
    updated_at: meta.updatedAt || release.published_at || null,
  };
  return { record };
}

/** Repo metadata → stars (null on failure, never 0), last-push time, owner. */
async function fetchRepoMeta(repo, pluginId) {
  try {
    const repoMeta = await githubJson(`/repos/${repo}`);
    return {
      stars: Number.isInteger(repoMeta.stargazers_count) ? repoMeta.stargazers_count : null,
      updatedAt: repoMeta.pushed_at || null,
      author: repoMeta.owner?.login || repo.split("/", 1)[0],
    };
  } catch (error) {
    console.error(`warning: ${pluginId}: star/metadata lookup failed, emitting stars=null (${error.message})`);
    return { stars: null, updatedAt: null, author: repo.split("/", 1)[0] };
  }
}

// --- Orchestration -----------------------------------------------------------

export async function buildIndex(specs) {
  const records = [];
  const errors = [];
  for (const spec of specs) {
    const { record, error } = await buildEntry(spec);
    if (error) {
      errors.push(error);
      console.error(`skip: ${error}`);
      continue;
    }
    records.push(record);
  }
  const document = {
    schema_version: SCHEMA_VERSION,
    generated_at: new Date().toISOString().replace(/\.\d{3}Z$/, "Z"),
    source: { name: SOURCE_NAME, url: SOURCE_URL },
    plugins: records,
  };
  return { document, errors };
}

async function main() {
  const text = await fs.readFile(PLUGINS_YAML, "utf8");
  const specs = parsePluginsYaml(text);
  // An empty list is expected at launch (no plugin repos yet) — it produces a
  // valid, empty index.json and is NOT an error. Only a non-empty list that
  // resolves to zero entries (below) indicates a real failure.
  const { document, errors } = await buildIndex(specs);
  await fs.writeFile(OUTPUT_JSON, `${JSON.stringify(document, null, 2)}\n`, "utf8");

  console.error(
    `Built index.json: ${document.plugins.length} built, ${errors.length} skipped, ` +
      `${specs.length} listed. Output: ${OUTPUT_JSON}`,
  );
  // A non-empty list that produced zero entries is almost certainly a bad token
  // or total outage — fail so CI never publishes an empty catalog over a good one.
  if (specs.length > 0 && document.plugins.length === 0) {
    console.error("error: no entries could be built; refusing to publish empty index");
    process.exitCode = 1;
  }
}

// Run only when invoked directly (not when imported by a test).
if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  main().catch((error) => {
    console.error(`fatal: ${error.message}`);
    process.exitCode = 1;
  });
}
