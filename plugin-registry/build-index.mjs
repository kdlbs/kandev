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
// fails a scheduled build — its error is logged to stderr and it is skipped. A
// pull request fails when native package or canvas validation rejects an entry,
// while ordinary release availability gaps remain skippable. A repo whose star
// lookup fails retains its previous count when one is available and otherwise
// uses `stars: null`, never inventing `0` for a transient outage.
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
const MAX_PLUGIN_PACKAGE_BYTES = 100 * 1024 * 1024;
const MAX_CANVAS_PACKAGE_BYTES = 10 * 1024 * 1024;
const PLUGIN_DOWNLOAD_TIMEOUT_MS = 60_000;
const PACKAGE_INSPECTOR_TIMEOUT_MS = 30_000;
const CANVAS_INSPECTOR_TIMEOUT_MS = 30_000;
const PREVIOUS_INDEX_URL = "https://kdlbs.github.io/kandev/plugins/index.json";

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
  let previewItemIndent = 0;

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
        previewItemIndent = item[1].length;
        assignField(currentPreview, item[2]);
        continue;
      }
      inPreviews = false;
      currentPreview = null;
      previewItemIndent = 0;
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
        previewItemIndent = 0;
        continue;
      }
      if (
        inPreviews &&
        currentPreview &&
        line.search(/\S/) > previewItemIndent
      ) {
        assignField(currentPreview, value);
        continue;
      }
      inPreviews = false;
      currentPreview = null;
      previewItemIndent = 0;
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
  `https://raw.githubusercontent.com/${repo}/${ref}/${filePath.replace(/^\/+/, "")}`;

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
function normalizeVersion(tag) {
  return tag && /^v\d/.test(tag) ? tag.slice(1) : tag;
}

/** Pick the exact package tarball from the release assets. */
function pickPackageAsset(assets, pluginId, version) {
  const exactName = `${pluginId}-${version}.tar.gz`;
  const exact = assets.find((asset) => asset.name === exactName);
  return exact
    ? { asset: exact }
    : { error: `release has no exact ${exactName} asset` };
}

function pickCanvasPackageAsset(assets, pluginId, version) {
  const exactName = `${pluginId}-${version}.tar.gz`;
  const exact = assets.find((asset) => asset.name === exactName);
  return exact
    ? { asset: exact }
    : { error: `release has no exact ${exactName} asset` };
}

function validatePreviews(previews, required) {
  if (required && (!Array.isArray(previews) || previews.length === 0)) {
    return "canvas entries require at least one preview image";
  }
  if (previews === undefined) return undefined;
  if (!Array.isArray(previews) || previews.length > 8)
    return "preview image count must be between 1 and 8";
  for (const preview of previews) {
    if (
      !preview ||
      typeof preview.url !== "string" ||
      typeof preview.alt !== "string"
    ) {
      return "preview images require url and alt";
    }
    const alt = preview.alt.trim();
    let parsed;
    try {
      parsed = new URL(preview.url);
    } catch {
      return "preview URLs must be absolute HTTPS URLs";
    }
    if (
      parsed.protocol !== "https:" ||
      !parsed.hostname ||
      parsed.username ||
      parsed.password ||
      parsed.hash ||
      preview.url.length > 2048
    ) {
      return "preview URLs must be HTTPS without credentials or fragments";
    }
    if (alt.length < 1 || alt.length > 300)
      return "preview alt text must be 1 to 300 characters";
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

const RESERVED_AUTHORS = new Set(["kandev", "kdlbs"]);

function normalizeReservedAuthor(value) {
  return String(value ?? "")
    .normalize("NFKC")
    .toLowerCase()
    .replace(
      /[\s\u00ad\u034f\u061c\u115f-\u1160\u17b4-\u17b5\u180b-\u180f\u200b-\u200f\u202a-\u202e\u2060-\u206f\u3164\ufeff\ufe00-\ufe0f]/gu,
      "",
    );
}

function isReservedAuthor(value) {
  return RESERVED_AUTHORS.has(normalizeReservedAuthor(value));
}

/**
 * Resolve a single plugins.yaml spec into a full index.json record.
 * @returns {Promise<{record?: object, error?: string, validationError?: boolean}>}
 */
export async function buildEntry(spec) {
  const pluginId = spec.id;
  const repo = spec.repo;
  if (!pluginId || !repo)
    return {
      error: `entry missing id/repo: ${JSON.stringify(spec)}`,
      validationError: true,
    };
  const kind = spec.kind || "plugin";
  const previewError = validatePreviews(spec.previews, kind === "canvas");
  if (previewError)
    return { error: `${pluginId}: ${previewError}`, validationError: true };
  if (kind !== "plugin" && kind !== "canvas")
    return {
      error: `${pluginId}: unsupported kind ${kind}`,
      validationError: true,
    };

  let release;
  try {
    release = await githubJson(`/repos/${repo}/releases/latest`);
  } catch (error) {
    return { error: `${pluginId}: no latest release (${error.message})` };
  }

  const tag = release.tag_name || "";
  const version = normalizeVersion(tag);
  const picker = kind === "canvas" ? pickCanvasPackageAsset : pickPackageAsset;
  const { asset, error: assetError } = picker(
    release.assets || [],
    pluginId,
    version,
  );
  if (assetError) return { error: `${pluginId}: ${assetError}` };

  const meta = await fetchRepoMeta(repo, pluginId);
  if (meta.error)
    return {
      error: `${pluginId}: ${meta.error}`,
      validationError: meta.validationError,
    };
  const official = spec.official === true;
  if (official && meta.login.toLowerCase() !== "kdlbs") {
    return {
      error: `${pluginId}: official entries must be owned by kdlbs`,
      validationError: true,
    };
  }

  const canonicalRepoURL = `https://github.com/${repo}`;
  const inspectorName =
    kind === "canvas"
      ? "KANDEV_CANVAS_PACKAGE_INSPECTOR"
      : "KANDEV_PLUGIN_PACKAGE_INSPECTOR";
  const maxPackageBytes =
    kind === "canvas" ? MAX_CANVAS_PACKAGE_BYTES : MAX_PLUGIN_PACKAGE_BYTES;
  let inspected;
  if (process.env[inspectorName]) {
    inspected = await inspectPackageAsset(
      asset.browser_download_url,
      pluginId,
      inspectorName,
      maxPackageBytes,
    );
    if (inspected.error)
      return {
        error: `${pluginId}: ${inspected.error}`,
        validationError: true,
      };
  } else {
    // A missing inspector cannot create publisher evidence. Keep ordinary
    // catalog discovery available for development and legacy tests, but fail
    // an explicitly official entry rather than publishing an unverified
    // first-party listing from an incomplete build environment.
    if (official || kind === "canvas")
      return {
        error: `${pluginId}: ${inspectorName} is not configured`,
        validationError: true,
      };
    const manifest = tag ? await fetchManifest(repo, tag) : {};
    inspected = {
      descriptor: {
        ...manifest,
        id: pluginId,
        version,
        kind,
        repo_url: canonicalRepoURL,
      },
      digest: null,
    };
  }
  const inspectedDescriptor = inspected.descriptor;
  if (
    inspectedDescriptor.id !== pluginId ||
    inspectedDescriptor.version !== version ||
    inspectedDescriptor.kind !== kind
  ) {
    return {
      error: `${pluginId}: inspected package identity does not match the registry entry`,
      validationError: true,
    };
  }

  if (inspectedDescriptor.repo_url) {
    if (
      canonicalizeRepoURL(inspectedDescriptor.repo_url) !==
      canonicalizeRepoURL(canonicalRepoURL)
    ) {
      return {
        error: `${pluginId}: inspected package repository does not match the registry entry`,
        validationError: true,
      };
    }
  }
  if (!official && isReservedAuthor(inspectedDescriptor.author)) {
    return {
      error: `${pluginId}: reserved Kandev author claims must come from an official kdlbs entry`,
      validationError: true,
    };
  }
  const publisher = inspected.digest
    ? {
        schema_version: 1,
        repository_id: meta.repositoryID,
        owner_id: meta.ownerID,
        login: meta.login,
        repository: meta.repository,
        official,
      }
    : undefined;
  const record = {
    id: pluginId,
    kind,
    // Canvas presentation comes from the inspected archive. Plugins retain
    // the existing manifest-first projection and fallback behavior.
    name: inspectedDescriptor.display_name || humanize(pluginId),
    description: inspectedDescriptor.description || release.name || "",
    author: inspectedDescriptor.author || "",
    categories: inspectedDescriptor.categories || spec.categories || [],
    icon_url:
      inspectedDescriptor.icon && tag
        ? rawUrl(repo, tag, inspectedDescriptor.icon)
        : null,
    repo_url: canonicalRepoURL,
    version: version || null,
    min_kandev_version: inspectedDescriptor.min_kandev_version ?? null,
    ...(kind === "canvas" ? { license: inspectedDescriptor.license } : {}),
    package_url: asset.browser_download_url,
    package_sha256: inspected.digest,
    ...(publisher ? { publisher } : {}),
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

async function inspectPackageAsset(
  assetURL,
  pluginId,
  inspectorEnv,
  maxPackageBytes,
) {
  const inspector = process.env[inspectorEnv];
  if (!inspector) return { error: `${inspectorEnv} is not configured` };
  let response;
  try {
    response = await fetchWithTimeout(
      assetURL,
      { headers: { Accept: "application/gzip", "User-Agent": USER_AGENT } },
      PLUGIN_DOWNLOAD_TIMEOUT_MS,
    );
  } catch (error) {
    return { error: `package download failed (${error.message})` };
  }
  if (!response.ok)
    return { error: `package download returned ${response.status}` };
  let body;
  try {
    body = await readBoundedResponse(response, maxPackageBytes);
  } catch {
    return { error: "package exceeds the package size limit" };
  }
  const digest = createHash("sha256").update(body).digest("hex");
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-plugin-registry-"),
  );
  const packagePath = path.join(directory, `${pluginId}.tar.gz`);
  try {
    await fs.writeFile(packagePath, body, { mode: 0o600 });
    const result = await execFile(inspector, ["--file", packagePath], {
      maxBuffer: 1024 * 1024,
      timeout: PACKAGE_INSPECTOR_TIMEOUT_MS,
    });
    let descriptor;
    try {
      descriptor = JSON.parse(result.stdout);
    } catch {
      return { error: "package inspector returned invalid JSON" };
    }
    return { descriptor, digest };
  } catch {
    return { error: "package inspection failed" };
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
}

async function readBoundedResponse(response, limit) {
  if (!response.body?.getReader)
    throw new Error("response body stream unavailable");
  const reader = response.body.getReader();
  const chunks = [];
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) return Buffer.concat(chunks);
      const chunk = Buffer.from(value);
      total += chunk.length;
      if (total > limit) {
        await reader.cancel();
        throw new Error("response exceeds limit");
      }
      chunks.push(chunk);
    }
  } finally {
    reader.releaseLock();
  }
}

function canonicalizeRepoURL(value) {
  try {
    const parsed = new URL(value);
    if (
      parsed.protocol !== "https:" ||
      parsed.hostname !== "github.com" ||
      parsed.username ||
      parsed.password ||
      parsed.search ||
      parsed.hash
    )
      return "";
    return `${parsed.origin}${parsed.pathname.replace(/\/+$/, "")}`;
  } catch {
    return "";
  }
}

/** Repo metadata → stars (null on failure, never 0), last-push time, owner. */
async function fetchRepoMeta(repo, pluginId) {
  try {
    const repoMeta = await githubJson(`/repos/${repo}`);
    const owner = repoMeta.owner;
    const repository = repoMeta.full_name;
    if (
      !Number.isSafeInteger(repoMeta.id) ||
      !Number.isSafeInteger(owner?.id) ||
      typeof owner.login !== "string" ||
      typeof repository !== "string"
    ) {
      return {
        error: "repository ownership metadata is incomplete",
        validationError: true,
      };
    }
    if (
      repository.toLowerCase() !== repo.toLowerCase() ||
      repository.split("/", 1)[0].toLowerCase() !== owner.login.toLowerCase()
    ) {
      return {
        error:
          "repository ownership metadata does not match the registry pointer",
        validationError: true,
      };
    }
    return {
      stars: Number.isInteger(repoMeta.stargazers_count)
        ? repoMeta.stargazers_count
        : null,
      updatedAt: repoMeta.pushed_at || null,
      repositoryID: String(repoMeta.id),
      ownerID: String(owner.id),
      login: owner.login,
      repository,
    };
  } catch (error) {
    console.error(
      `warning: ${pluginId}: repository ownership lookup failed (${error.message})`,
    );
    return { error: `repository ownership lookup failed (${error.message})` };
  }
}

// --- Orchestration -----------------------------------------------------------

export async function buildIndex(specs, previousDocument) {
  const records = [];
  const errors = [];
  const nativeErrors = [];
  const canvasErrors = [];
  const previousStars = new Map(
    (previousDocument?.plugins ?? [])
      .filter((entry) => entry && typeof entry.id === "string" && Number.isInteger(entry.stars))
      .map((entry) => [entry.id, entry.stars]),
  );
  for (const spec of specs) {
    const { record, error, validationError } = await buildEntry(spec);
    if (error) {
      errors.push(error);
      if (validationError) nativeErrors.push(error);
      if ((spec.kind || "plugin") === "canvas") canvasErrors.push(error);
      console.error(`skip: ${error}`);
      continue;
    }
    if (record.stars === null && previousStars.has(record.id)) {
      record.stars = previousStars.get(record.id);
    }
    records.push(record);
  }
  const document = {
    schema_version: SCHEMA_VERSION,
    generated_at: new Date().toISOString().replace(/\.\d{3}Z$/, "Z"),
    source: { name: SOURCE_NAME, url: SOURCE_URL },
    plugins: records,
  };
  return { document, errors, nativeErrors, canvasErrors };
}

// Pull-request validation must fail for rejected package identity/provenance
// checks and canvas entries. Scheduled publication intentionally keeps its
// existing behavior of logging and skipping invalid entries so one bad release
// does not prevent healthy entries from being published.
export function pullRequestValidationFailed(nativeErrors, canvasErrors = []) {
  return nativeErrors.length > 0 || canvasErrors.length > 0;
}

export async function main(options = {}) {
  const text = options.specs ? null : await fs.readFile(PLUGINS_YAML, "utf8");
  const specs = options.specs ?? parsePluginsYaml(text);
  const outputPath = options.outputPath ?? OUTPUT_JSON;
  const eventName = options.eventName ?? process.env.GITHUB_EVENT_NAME;
  // An empty list is expected at launch (no plugin repos yet) — it produces a
  // valid, empty index.json and is NOT an error. Only a non-empty list that
  // resolves to zero entries (below) indicates a real failure.
  const previousDocument =
    options.previousDocument ?? (options.specs ? undefined : await loadPreviousDocument());
  const { document, errors, nativeErrors, canvasErrors } = await buildIndex(
    specs,
    previousDocument,
  );
  await fs.writeFile(
    outputPath,
    `${JSON.stringify(document, null, 2)}\n`,
    "utf8",
  );

  console.error(
    `Built index.json: ${document.plugins.length} built, ${errors.length} skipped, ` +
      `${specs.length} listed. Output: ${outputPath}`,
  );
  let exitCode = 0;
  // A non-empty list that produced zero entries is almost certainly a bad token
  // or total outage — fail so CI never publishes an empty catalog over a good one.
  if (specs.length > 0 && document.plugins.length === 0) {
    console.error(
      "error: no entries could be built; refusing to publish empty index",
    );
    exitCode = 1;
  }
  if (
    eventName === "pull_request" &&
    pullRequestValidationFailed(nativeErrors, canvasErrors)
  ) {
    console.error("error: pull-request validation found invalid entries");
    exitCode = 1;
  }
  if (exitCode !== 0) process.exitCode = exitCode;
  return { document, errors, nativeErrors, canvasErrors, exitCode };
}

async function loadPreviousDocument() {
  try {
    const response = await fetchWithTimeout(
      PREVIOUS_INDEX_URL,
      { headers: { Accept: "application/json", "User-Agent": USER_AGENT } },
      PLUGIN_DOWNLOAD_TIMEOUT_MS,
    );
    if (!response.ok) return undefined;
    const document = await response.json();
    return document && typeof document === "object" ? document : undefined;
  } catch (error) {
    console.error(`warning: previous catalog lookup failed (${error.message})`);
    return undefined;
  }
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
