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
import { createHash } from "node:crypto";
import { execFile as execFileCallback } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
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
const execFile = promisify(execFileCallback);
const MAX_CANVAS_PACKAGE_BYTES = 10 * 1024 * 1024;

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
  let inPreviews = false;
  let currentPreview = null;

  for (const rawLine of text.split("\n")) {
    const line = stripComment(rawLine);
    if (line.trim() === "") continue;

    if (!inPlugins) {
      if (line.trim() === "plugins:") inPlugins = true;
      continue;
    }

    const item = line.match(/^(\s*)-\s*(.*)$/);
    if (item) {
      if (inPreviews && current && item[1].length > 2) {
        currentPreview = {};
        current.previews ??= [];
        current.previews.push(currentPreview);
        assignField(currentPreview, item[2]);
        continue;
      }
      inPreviews = false;
      currentPreview = null;
      current = {};
      specs.push(current);
      assignField(current, item[2]);
      continue;
    }
    if (current && /^\s+\S/.test(line)) {
      const value = line.trim();
      if (value === "previews:") {
        current.previews = [];
        inPreviews = true;
        currentPreview = null;
        continue;
      }
      if (inPreviews && currentPreview && line.search(/\S/) > 2) {
        assignField(currentPreview, value);
        continue;
      }
      inPreviews = false;
      currentPreview = null;
      assignField(current, value);
    }
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

function pickCanvasPackageAsset(assets, pluginId, version) {
  const exactName = `${pluginId}-${version}.tar.gz`;
  const exact = assets.find((asset) => asset.name === exactName);
  return exact ? { asset: exact } : { error: `release has no exact ${exactName} asset` };
}

function validatePreviews(previews, required) {
  if (required && (!Array.isArray(previews) || previews.length === 0)) {
    return "canvas entries require at least one preview image";
  }
  if (previews === undefined) return undefined;
  if (!Array.isArray(previews) || previews.length > 8) return "preview image count must be between 1 and 8";
  for (const preview of previews) {
    if (!preview || typeof preview.url !== "string" || typeof preview.alt !== "string") {
      return "preview images require url and alt";
    }
    const alt = preview.alt.trim();
    let parsed;
    try {
      parsed = new URL(preview.url);
    } catch {
      return "preview URLs must be absolute HTTPS URLs";
    }
    if (parsed.protocol !== "https:" || !parsed.hostname || parsed.username || parsed.password || parsed.hash || preview.url.length > 2048) {
      return "preview URLs must be HTTPS without credentials or fragments";
    }
    if (alt.length < 1 || alt.length > 300) return "preview alt text must be 1 to 300 characters";
  }
  return undefined;
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
  const kind = spec.kind || "plugin";
  const previewError = validatePreviews(spec.previews, kind === "canvas");
  if (previewError) return { error: `${pluginId}: ${previewError}` };
  if (kind !== "plugin" && kind !== "canvas") return { error: `${pluginId}: unsupported kind ${kind}` };

  let release;
  try {
    release = await githubJson(`/repos/${repo}/releases/latest`);
  } catch (error) {
    return { error: `${pluginId}: no latest release (${error.message})` };
  }

  const tag = release.tag_name || "";
  const version = normalizeVersion(tag);
  const picker = kind === "canvas" ? pickCanvasPackageAsset : pickPackageAsset;
  const { asset, error: assetError } = picker(release.assets || [], pluginId, version);
  if (assetError) return { error: `${pluginId}: ${assetError}` };

  const manifest = tag ? await fetchManifest(repo, tag) : {};
  const iconUrl = manifest.icon && tag ? rawUrl(repo, tag, manifest.icon) : null;
  const meta = await fetchRepoMeta(repo, pluginId);

  let packageSHA256 = null;
  let inspectedDescriptor = null;
  if (kind === "canvas") {
    const inspected = await inspectCanvasAsset(asset.browser_download_url, pluginId);
    if (inspected.error) return { error: `${pluginId}: ${inspected.error}` };
    if (inspected.descriptor.id !== pluginId || inspected.descriptor.version !== version || inspected.descriptor.kind !== "canvas") {
      return { error: `${pluginId}: inspected package identity does not match the registry entry` };
    }
    inspectedDescriptor = inspected.descriptor;
    packageSHA256 = inspected.digest;
  }

  const record = {
    id: pluginId,
    kind,
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
    ...(kind === "canvas" ? { license: inspectedDescriptor.license } : {}),
    package_url: asset.browser_download_url,
    package_sha256: packageSHA256,
    ...(kind === "canvas"
      ? {
          permissions: {
            reads: inspectedDescriptor.api_read || [],
            writes: inspectedDescriptor.api_write || [],
            events: inspectedDescriptor.events || [],
            shared_state: Boolean(inspectedDescriptor.state),
            external_origins: inspectedDescriptor.network_origins || [],
          },
        }
      : {}),
    ...(spec.previews ? { previews: spec.previews } : {}),
    stars: meta.stars,
    updated_at: meta.updatedAt || release.published_at || null,
  };
  return { record };
}

async function inspectCanvasAsset(assetURL, pluginId) {
  const inspector = process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR;
  if (!inspector) return { error: "canvas package inspector is not configured" };
  let response;
  try {
    response = await fetchWithTimeout(assetURL, { headers: { Accept: "application/gzip", "User-Agent": USER_AGENT } });
  } catch (error) {
    return { error: `canvas package download failed (${error.message})` };
  }
  if (!response.ok) return { error: `canvas package download returned ${response.status}` };
  const body = Buffer.from(await response.arrayBuffer());
  if (body.length > MAX_CANVAS_PACKAGE_BYTES) return { error: "canvas package exceeds the package size limit" };
  const digest = createHash("sha256").update(body).digest("hex");
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "kandev-canvas-registry-"));
  const packagePath = path.join(directory, `${pluginId}.tar.gz`);
  try {
    await fs.writeFile(packagePath, body, { mode: 0o600 });
    const result = await execFile(inspector, ["--file", packagePath], { maxBuffer: 1024 * 1024 });
    let descriptor;
    try {
      descriptor = JSON.parse(result.stdout);
    } catch {
      return { error: "canvas package inspector returned invalid JSON" };
    }
    return { descriptor, digest };
  } catch {
    return { error: "canvas package inspection failed" };
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
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
