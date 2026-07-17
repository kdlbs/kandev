import { createHash } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const defaultDocsDir = path.join(repoRoot, "docs/public");

/**
 * Validate that every published Markdown page has frontmatter and appears
 * exactly once in the public navigation metadata.
 *
 * @param {string} [docsDir] Directory containing published docs and meta.json.
 * @returns {Promise<{pageCount: number}>} Number of validated published pages.
 */
export async function validatePublicDocs(docsDir = defaultDocsDir) {
  const meta = await readMeta(docsDir);
  const files = await collectMarkdownFiles(docsDir);
  const pagesBySlug = new Map();

  for (const file of files) {
    const markdown = await fs.readFile(path.join(docsDir, file), "utf8");
    await assertLocalLinks(docsDir, file, markdown);

    if (path.posix.basename(file).toLowerCase() === "readme.md") continue;

    assertFrontmatter(file, markdown);
    assertDocumentStructure(file, markdown);

    const slug = file.replace(/\.mdx?$/, "").replace(/\/index$/, "");
    const existing = pagesBySlug.get(slug);
    if (existing) {
      throw new Error(
        `multiple published files resolve to slug ${slug}: ${existing}, ${file}`,
      );
    }
    pagesBySlug.set(slug, file);
  }

  const listed = new Set();
  for (const entry of meta.pages) {
    if (isNavigationDecoration(entry)) continue;
    if (listed.has(entry)) {
      throw new Error(`meta.json lists page more than once: ${entry}`);
    }
    if (!pagesBySlug.has(entry)) {
      throw new Error(`meta.json references unknown page: ${entry}`);
    }

    listed.add(entry);
  }

  for (const slug of pagesBySlug.keys()) {
    if (!listed.has(slug)) {
      throw new Error(`meta.json is missing published page: ${slug}`);
    }
  }

  const featureMediaDir = path.join(docsDir, "media/feature-guides");
  if (await pathExists(featureMediaDir)) {
    await validateFeatureMedia({ docsDir });
  }

  if (path.resolve(docsDir) === defaultDocsDir) {
    await validateCoverageInventory({ repoRoot, docsDir });
  }

  return { pageCount: pagesBySlug.size };
}

/**
 * Validate reviewed, focused feature-guide media and its publication ownership.
 *
 * The checked-in manifest is the durable boundary between disposable capture
 * tooling and published documentation. Every clip must retain accepted QA,
 * exact file hashes, a real page/section owner, and a complete DocsVideo embed.
 *
 * @param {{docsDir: string}} paths Published docs root.
 * @returns {Promise<{clipCount: number}>} Number of validated clips.
 */
export async function validateFeatureMedia({ docsDir }) {
  const mediaDir = path.join(docsDir, "media/feature-guides");
  const manifestPath = path.join(mediaDir, "manifest.json");
  const notesPath = path.join(mediaDir, "NOTES.txt");
  const manifest = JSON.parse(await fs.readFile(manifestPath, "utf8"));

  if (manifest?.schema_version !== 1 || !Array.isArray(manifest.clips)) {
    throw new Error(
      "feature media manifest must use schema version 1 and contain clips",
    );
  }
  if (manifest.qa_status !== "accepted") {
    throw new Error("feature media manifest must have accepted QA status");
  }
  if (
    typeof manifest.generated_at !== "string" ||
    !Number.isFinite(Date.parse(manifest.generated_at))
  ) {
    throw new Error(
      "feature media manifest must record a valid generation time",
    );
  }
  if (manifest.clips.length < 1 || manifest.clips.length > 10) {
    throw new Error(
      "feature media manifest must contain between 1 and 10 clips",
    );
  }
  assertFeatureMediaContract(manifest.delivery_contract);

  const notes = await fs.readFile(notesPath, "utf8");
  if (!notes.trim()) {
    throw new Error(
      "feature media NOTES.txt must describe capture provenance and QA",
    );
  }

  const expectedFiles = new Set(["manifest.json", "NOTES.txt"]);
  const slugs = new Set();
  for (const clip of manifest.clips) {
    assertFeatureClipShape(clip);
    if (slugs.has(clip.slug)) {
      throw new Error(`feature media manifest repeats clip slug: ${clip.slug}`);
    }
    slugs.add(clip.slug);

    const pagePath = resolveInside(
      docsDir,
      clip.intended_docs.page,
      `${clip.slug} documentation page`,
    );
    if (!/\.mdx?$/i.test(pagePath)) {
      throw new Error(
        `${clip.slug} documentation owner must be a Markdown page`,
      );
    }
    const page = await fs.readFile(pagePath, "utf8");
    if (!collectHeadingTitles(page).has(clip.intended_docs.section)) {
      throw new Error(
        `${clip.slug} targets missing section in ${clip.intended_docs.page}: ${clip.intended_docs.section}`,
      );
    }
    if (!hasCompleteFeatureMediaEmbed(page, clip.filenames)) {
      throw new Error(
        `${clip.slug} is not embedded with its complete media triplet in ${clip.intended_docs.page}`,
      );
    }

    const expectedCodecs = { webm: "vp9", mp4: "h264", poster: "webp" };
    for (const [kind, extension] of Object.entries({
      webm: "webm",
      mp4: "mp4",
      poster: "webp",
    })) {
      const filename = clip.filenames[kind];
      if (filename !== `${clip.slug}.${extension}`) {
        throw new Error(
          `${clip.slug} has unexpected ${kind} filename: ${filename}`,
        );
      }
      const record = clip.files[kind];
      if (
        !record ||
        !Number.isInteger(record.bytes) ||
        record.bytes <= 0 ||
        record.codec !== expectedCodecs[kind] ||
        typeof record.sha256 !== "string" ||
        !/^[a-f0-9]{64}$/.test(record.sha256)
      ) {
        throw new Error(`${clip.slug} has invalid ${kind} file metadata`);
      }

      const filePath = resolveInside(
        mediaDir,
        filename,
        `${clip.slug} ${kind}`,
      );
      const contents = await fs.readFile(filePath);
      if (contents.byteLength !== record.bytes) {
        throw new Error(`${filename} byte count does not match manifest`);
      }
      const digest = createHash("sha256").update(contents).digest("hex");
      if (digest !== record.sha256) {
        throw new Error(`${filename} SHA-256 does not match manifest`);
      }
      expectedFiles.add(filename);
    }
  }

  const entries = await fs.readdir(mediaDir, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isFile() || !expectedFiles.has(entry.name)) {
      throw new Error(
        `feature media directory contains untracked file: ${entry.name}`,
      );
    }
  }
  for (const filename of expectedFiles) {
    if (!entries.some((entry) => entry.isFile() && entry.name === filename)) {
      throw new Error(
        `feature media directory is missing tracked file: ${filename}`,
      );
    }
  }

  return { clipCount: slugs.size };
}

function assertFeatureMediaContract(contract) {
  if (
    contract?.dimensions?.width !== 960 ||
    contract?.dimensions?.height !== 600 ||
    contract?.frame_rate !== 25 ||
    contract?.audio !== false ||
    JSON.stringify(contract?.video_formats) !==
      JSON.stringify(["vp9-webm", "h264-mp4"]) ||
    contract?.poster_format !== "webp"
  ) {
    throw new Error(
      "feature media manifest has an unsupported delivery contract",
    );
  }
}

function assertFeatureClipShape(clip) {
  if (!clip || typeof clip !== "object" || Array.isArray(clip)) {
    throw new Error("feature media manifest contains an invalid clip entry");
  }

  const fields = [
    [
      "slug",
      typeof clip.slug === "string" &&
        /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(clip.slug),
    ],
    ["title", typeof clip.title === "string" && clip.title.trim().length > 0],
    [
      "accessible_caption",
      typeof clip.accessible_caption === "string" &&
        clip.accessible_caption.trim().length > 0,
    ],
    [
      "source_scenario",
      typeof clip.source_scenario === "string" &&
        clip.source_scenario.trim().length >= 20,
    ],
    [
      "data_isolation",
      typeof clip.data_isolation === "string" &&
        clip.data_isolation.trim().length >= 20,
    ],
    [
      "duration_seconds",
      typeof clip.duration_seconds === "number" &&
        clip.duration_seconds >= 6 &&
        clip.duration_seconds <= 15,
    ],
    [
      "dimensions",
      clip.dimensions?.width === 960 && clip.dimensions?.height === 600,
    ],
    [
      "intended_docs",
      typeof clip.intended_docs?.page === "string" &&
        typeof clip.intended_docs?.section === "string" &&
        clip.intended_docs.section.trim().length > 0,
    ],
    [
      "filenames",
      Boolean(
        clip.filenames &&
          typeof clip.filenames === "object" &&
          !Array.isArray(clip.filenames),
      ),
    ],
    [
      "files",
      Boolean(
        clip.files && typeof clip.files === "object" && !Array.isArray(clip.files),
      ),
    ],
  ];
  const invalidField = fields.find(([, valid]) => !valid)?.[0];
  if (invalidField) {
    const owner =
      typeof clip.slug === "string" && clip.slug.length > 0
        ? clip.slug
        : "<unknown>";
    throw new Error(`feature media clip ${owner} has invalid ${invalidField}`);
  }
}

function hasCompleteFeatureMediaEmbed(markdown, filenames) {
  return [
    ...stripMarkdownCode(markdown).matchAll(/<DocsVideo\b[\s\S]*?\/>/g),
  ].some((match) => {
    const attributes = Object.fromEntries(
      [
        ...match[0].matchAll(
          /\b(webm|mp4|poster)\s*=\s*(?:"([^"]*)"|'([^']*)')/g,
        ),
      ].map((attribute) => [
        attribute[1],
        attribute[2] ?? attribute[3],
      ]),
    );

    return Object.entries(filenames).every(
      ([kind, filename]) =>
        attributes[kind]?.replace(/^\.\//, "") ===
        `media/feature-guides/${filename}`,
    );
  });
}

function collectHeadingTitles(markdown) {
  const titles = new Set();
  for (const match of stripMarkdownCode(markdown, {
    keepInlineCode: true,
  }).matchAll(/^ {0,3}#{1,6}[ \t]+(.+?)\s*#*\s*$/gm)) {
    titles.add(stripHeadingMarkup(match[1]));
  }
  return titles;
}

function stripHeadingMarkup(value) {
  const linkedText = value
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1");
  let text = "";
  let insideTag = false;

  for (const character of linkedText) {
    if (character === "<") {
      insideTag = true;
    } else if (character === ">" && insideTag) {
      insideTag = false;
    } else if (!insideTag) {
      text += character;
    }
  }

  if (insideTag) {
    throw new Error("heading contains unterminated inline HTML");
  }

  return text.replace(/[`*_~]/g, "").trim();
}

function resolveInside(root, relativePath, label) {
  const target = path.resolve(root, relativePath);
  const relative = path.relative(root, target);
  if (!relative || relative.startsWith("..") || path.isAbsolute(relative)) {
    throw new Error(`${label} must stay inside ${root}`);
  }
  return target;
}

async function pathExists(target) {
  try {
    await fs.access(target);
    return true;
  } catch {
    return false;
  }
}

/**
 * Validate the source-backed feature coverage inventory.
 *
 * The inventory assigns every published product area to documentation pages
 * and concrete implementation/test evidence. It also accounts for every
 * statically registered Settings route and Kandev MCP tool, so newly shipped
 * surfaces cannot silently bypass the public docs.
 *
 * @param {{repoRoot: string, docsDir: string}} paths Repository and docs roots.
 * @returns {Promise<{areaCount: number, settingsRouteCount: number, mcpToolCount: number}>}
 */
export async function validateCoverageInventory({ repoRoot, docsDir }) {
  const coveragePath = path.join(docsDir, "coverage.json");
  const coverage = JSON.parse(await fs.readFile(coveragePath, "utf8"));
  if (coverage?.version !== 1 || !Array.isArray(coverage.areas)) {
    throw new Error(
      "coverage.json must use version 1 and contain an areas array",
    );
  }
  if (coverage.areas.length === 0) {
    throw new Error("coverage.json must contain at least one coverage area");
  }

  const pageSlugs = new Set(
    (await collectMarkdownFiles(docsDir))
      .filter((file) => path.posix.basename(file).toLowerCase() !== "readme.md")
      .map((file) => file.replace(/\.mdx?$/, "").replace(/\/index$/, "")),
  );
  const areaIds = new Set();
  const coveredDocsPages = new Set();
  const coveredSettingsRoutes = new Set();
  const coveredMcpTools = new Set();

  for (const area of coverage.areas) {
    assertCoverageAreaShape(area);
    if (areaIds.has(area.id)) {
      throw new Error(`coverage.json repeats area id: ${area.id}`);
    }
    areaIds.add(area.id);

    for (const slug of area.docs) {
      if (!pageSlugs.has(slug)) {
        throw new Error(`${area.id} cites unknown docs page: ${slug}`);
      }
      coveredDocsPages.add(slug);
    }
    for (const source of area.sources) {
      await assertCoverageEvidence(repoRoot, area.id, "source", source);
    }
    for (const testFile of area.tests) {
      await assertCoverageEvidence(repoRoot, area.id, "test", testFile);
    }
    addUniqueCoverageValues(
      coveredSettingsRoutes,
      area.settingsRoutes ?? [],
      "settings route",
    );
    addUniqueCoverageValues(coveredMcpTools, area.mcpTools ?? [], "MCP tool");
  }

  for (const slug of pageSlugs) {
    if (!coveredDocsPages.has(slug)) {
      throw new Error(`coverage.json does not account for docs page: ${slug}`);
    }
  }

  const exclusions = coverage.exclusions ?? {};
  const excludedSettingsRoutes = validateCoverageExclusions(
    exclusions.settingsRoutes ?? [],
    "route",
    "settings route",
  );
  const excludedMcpTools = validateCoverageExclusions(
    exclusions.mcpTools ?? [],
    "tool",
    "MCP tool",
  );

  const shippedSettingsRoutes = await collectSettingsRoutes(repoRoot);
  assertCompleteSurfaceCoverage(
    shippedSettingsRoutes,
    coveredSettingsRoutes,
    excludedSettingsRoutes,
    "settings route",
  );
  const registeredMcpTools = await collectMcpTools(repoRoot);
  assertCompleteSurfaceCoverage(
    registeredMcpTools,
    coveredMcpTools,
    excludedMcpTools,
    "MCP tool",
  );

  return {
    areaCount: areaIds.size,
    settingsRouteCount: shippedSettingsRoutes.size,
    mcpToolCount: registeredMcpTools.size,
  };
}

function assertCoverageAreaShape(area) {
  if (!area || typeof area !== "object" || Array.isArray(area)) {
    throw new Error("coverage.json areas must contain objects");
  }
  if (
    typeof area.id !== "string" ||
    !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(area.id)
  ) {
    throw new Error("coverage.json area ids must use lowercase kebab-case");
  }
  assertStringArray(area, "audiences", area.id);
  assertStringArray(area, "docs", area.id);
  assertStringArray(area, "sources", area.id);
  assertStringArray(area, "tests", area.id);
  if (!new Set(["stable", "beta", "experimental"]).has(area.stability)) {
    throw new Error(`${area.id} has unsupported stability: ${area.stability}`);
  }
  for (const optionalField of ["settingsRoutes", "mcpTools"]) {
    if (area[optionalField] !== undefined) {
      assertStringArray(area, optionalField, area.id, { allowEmpty: true });
    }
  }
}

function assertStringArray(object, field, owner, { allowEmpty = false } = {}) {
  const value = object[field];
  if (
    !Array.isArray(value) ||
    (!allowEmpty && value.length === 0) ||
    !value.every((entry) => typeof entry === "string" && entry.length > 0)
  ) {
    throw new Error(
      `${owner} ${field} must be ${allowEmpty ? "an" : "a non-empty"} array of strings`,
    );
  }
}

async function assertCoverageEvidence(root, areaId, kind, relativePath) {
  const target = path.resolve(root, relativePath);
  const relative = path.relative(root, target);
  if (relative.startsWith("..") || path.isAbsolute(relative)) {
    throw new Error(
      `${areaId} cites ${kind} outside the repository: ${relativePath}`,
    );
  }
  let stats;
  try {
    stats = await fs.stat(target);
  } catch {
    throw new Error(`${areaId} cites missing ${kind}: ${relativePath}`);
  }
  if (!stats.isFile()) {
    throw new Error(`${areaId} cites non-file ${kind}: ${relativePath}`);
  }
}

function addUniqueCoverageValues(target, values, label) {
  for (const value of values) {
    if (target.has(value)) {
      throw new Error(
        `coverage.json assigns ${label} more than once: ${value}`,
      );
    }
    target.add(value);
  }
}

function validateCoverageExclusions(entries, key, label) {
  if (!Array.isArray(entries)) {
    throw new Error(`coverage.json excluded ${label}s must be an array`);
  }
  const values = new Set();
  for (const entry of entries) {
    if (
      !entry ||
      typeof entry !== "object" ||
      typeof entry[key] !== "string" ||
      typeof entry.reason !== "string" ||
      entry.reason.trim().length < 8
    ) {
      throw new Error(
        `coverage.json excluded ${label}s require ${key} and a reason`,
      );
    }
    if (values.has(entry[key])) {
      throw new Error(
        `coverage.json excludes ${label} more than once: ${entry[key]}`,
      );
    }
    values.add(entry[key]);
  }
  return values;
}

async function collectSettingsRoutes(root) {
  const source = await fs.readFile(
    path.join(root, "apps/web/src/settings-routes.tsx"),
    "utf8",
  );
  const routeTable = source.match(
    /const SETTINGS_ROUTES[\s\S]*?\n};\s*\n\s*export function SettingsRoutes/,
  )?.[0];
  if (!routeTable) {
    throw new Error("could not locate SETTINGS_ROUTES in settings-routes.tsx");
  }
  return new Set(
    [...routeTable.matchAll(/^\s*"(\/settings(?:\/[^"]*)?)":/gm)].map(
      (match) => match[1],
    ),
  );
}

async function collectMcpTools(root) {
  const serverDir = path.join(root, "apps/backend/internal/mcp/server");
  const files = await collectFilesWithExtension(serverDir, ".go");
  const tools = new Set();
  for (const file of files) {
    const source = await fs.readFile(file, "utf8");
    for (const match of source.matchAll(
      /mcp\.NewTool(?:WithRawSchema)?\(\s*"([^"]+)"/g,
    )) {
      tools.add(match[1]);
    }
  }
  return tools;
}

async function collectFilesWithExtension(dir, extension) {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  const files = await Promise.all(
    entries.map(async (entry) => {
      const target = path.join(dir, entry.name);
      if (entry.isDirectory())
        return collectFilesWithExtension(target, extension);
      return entry.isFile() && entry.name.endsWith(extension) ? [target] : [];
    }),
  );
  return files.flat();
}

function assertCompleteSurfaceCoverage(shipped, covered, excluded, label) {
  for (const value of covered) {
    if (excluded.has(value)) {
      throw new Error(`coverage.json both covers and excludes ${label}: ${value}`);
    }
  }
  for (const value of shipped) {
    if (!covered.has(value) && !excluded.has(value)) {
      throw new Error(`coverage.json does not account for ${label}: ${value}`);
    }
  }
  for (const value of [...covered, ...excluded]) {
    if (!shipped.has(value)) {
      throw new Error(`coverage.json references unknown ${label}: ${value}`);
    }
  }
}

/**
 * Read and validate the shape of public navigation metadata.
 *
 * @param {string} docsDir Directory containing meta.json.
 * @returns {Promise<{pages: string[]}>} Parsed navigation metadata.
 */
async function readMeta(docsDir) {
  const raw = await fs.readFile(path.join(docsDir, "meta.json"), "utf8");
  const meta = JSON.parse(raw);
  if (!meta || typeof meta !== "object" || Array.isArray(meta)) {
    throw new Error("meta.json must contain a JSON object");
  }
  if (
    !Array.isArray(meta.pages) ||
    !meta.pages.every((entry) => typeof entry === "string")
  ) {
    throw new Error("meta.json pages must be an array of strings");
  }

  return meta;
}

/**
 * Recursively collect Markdown paths relative to the published docs root.
 *
 * @param {string} dir Published docs root.
 * @param {string} [relativeDir] Directory relative to the docs root.
 * @returns {Promise<string[]>} Sorted relative Markdown paths.
 */
async function collectMarkdownFiles(dir, relativeDir = "") {
  const entries = await fs.readdir(path.join(dir, relativeDir), {
    withFileTypes: true,
  });
  const files = await Promise.all(
    entries.map(async (entry) => {
      const relativePath = path.posix.join(relativeDir, entry.name);
      if (entry.isDirectory()) {
        return collectMarkdownFiles(dir, relativePath);
      }

      return /\.mdx?$/.test(entry.name) ? [relativePath] : [];
    }),
  );

  return files.flat().sort();
}

/**
 * Require non-empty title and description fields in leading frontmatter.
 *
 * @param {string} file Relative page path used in validation errors.
 * @param {string} markdown Page source.
 * @returns {void}
 */
function assertFrontmatter(file, markdown) {
  const block = markdown.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/)?.[1];
  if (
    !block ||
    !/^title:\s*\S.*$/m.test(block) ||
    !/^description:\s*\S.*$/m.test(block)
  ) {
    throw new Error(
      `${file} must start with YAML frontmatter containing title and description`,
    );
  }
}

/**
 * Require one real page title immediately after frontmatter.
 *
 * @param {string} file Relative page path used in validation errors.
 * @param {string} markdown Page source.
 * @returns {void}
 */
function assertDocumentStructure(file, markdown) {
  const frontmatter = markdown.match(
    /^---\r?\n[\s\S]*?\r?\n---(?:\r?\n|$)/,
  )?.[0];
  const body = stripMarkdownCode(
    markdown.slice(frontmatter?.length ?? 0),
  ).trimStart();
  const headings = [...body.matchAll(/^# [^\n]+$/gm)];
  if (!body.startsWith("# ") || headings.length !== 1) {
    throw new Error(
      `${file} must begin with exactly one level-one heading after frontmatter`,
    );
  }
}

/**
 * Require every relative Markdown link or image to resolve on disk.
 *
 * @param {string} docsDir Published docs root.
 * @param {string} file Relative source path used in validation errors.
 * @param {string} markdown Page source.
 * @returns {Promise<void>}
 */
async function assertLocalLinks(docsDir, file, markdown) {
  const source = stripMarkdownCode(markdown);
  const definitionPattern = /^\s{0,3}\[([^\]\n]+)\]:\s*(\S.*)$/gm;
  const referencePattern = /!?\[([^\]\n]+)\]\[([^\]\n]*)\]/g;
  const shortcutReferencePattern = /(?<![!\\\[\]])\[([^\]\n]+)\](?![\[(:])/g;
  const destinations = collectInlineLinkDestinations(source);
  destinations.push(...collectDocsVideoDestinations(source));
  const definitions = new Map();

  for (const match of source.matchAll(definitionPattern)) {
    if (match[1].startsWith("^")) continue;
    const label = normalizeReferenceLabel(match[1]);
    definitions.set(label, match[2]);
    destinations.push(match[2]);
  }

  for (const match of source.matchAll(referencePattern)) {
    const label = normalizeReferenceLabel(match[2] || match[1]);
    if (!definitions.has(label)) {
      throw new Error(`${file} uses undefined Markdown reference: ${label}`);
    }
  }

  for (const match of source.matchAll(shortcutReferencePattern)) {
    const label = normalizeReferenceLabel(match[1]);
    // Admonitions, footnotes, and task boxes use brackets but are not links.
    if (
      !label ||
      label.startsWith("!") ||
      label.startsWith("^") ||
      /^(?:x|-)$/i.test(label)
    ) {
      continue;
    }
    if (!definitions.has(label)) {
      throw new Error(`${file} uses undefined Markdown reference: ${label}`);
    }
  }

  for (const destination of destinations) {
    const href = parseMarkdownDestination(destination);
    if (!href || isExternalDestination(href)) {
      continue;
    }
    if (href.startsWith("/")) {
      throw new Error(
        `${file} uses a site-root link instead of a relative source link: ${href}`,
      );
    }

    const hashIndex = href.indexOf("#");
    const pathAndQuery = hashIndex === -1 ? href : href.slice(0, hashIndex);
    const rawFragment = hashIndex === -1 ? "" : href.slice(hashIndex + 1);
    const pathOnly = pathAndQuery.split("?", 1)[0];

    let decoded;
    try {
      decoded = decodeURIComponent(pathOnly);
    } catch {
      throw new Error(
        `${file} contains an invalid encoded local link: ${href}`,
      );
    }

    const target = pathOnly
      ? path.resolve(
          path.dirname(path.join(docsDir, file)),
          decoded.replace(/\\([\\() ])/g, "$1"),
        )
      : path.join(docsDir, file);
    try {
      await fs.access(target);
    } catch {
      throw new Error(`${file} links to missing local target: ${href}`);
    }

    if (rawFragment && /\.mdx?$/i.test(target)) {
      await assertHeadingFragment(file, href, target, rawFragment);
    }
  }
}

/**
 * Require a local Markdown fragment to match the published heading identifier.
 *
 * @param {string} file Relative source path used in validation errors.
 * @param {string} href Original link destination.
 * @param {string} target Absolute target Markdown path.
 * @param {string} rawFragment URL-encoded fragment without the hash marker.
 * @returns {Promise<void>}
 */
async function assertHeadingFragment(file, href, target, rawFragment) {
  let fragment;
  try {
    fragment = decodeURIComponent(rawFragment);
  } catch {
    throw new Error(`${file} contains an invalid encoded local link: ${href}`);
  }

  const markdown = await fs.readFile(target, "utf8");
  if (!collectHeadingAnchors(markdown).has(fragment)) {
    throw new Error(`${file} links to missing heading: ${href}`);
  }
}

/**
 * Collect the GitHub-style identifiers emitted for Markdown headings.
 *
 * Duplicate headings receive the same numeric suffix used by rehype-slug.
 *
 * @param {string} markdown Markdown page source.
 * @returns {Set<string>} Published heading identifiers.
 */
function collectHeadingAnchors(markdown) {
  const source = stripMarkdownCode(markdown, { keepInlineCode: true });
  const anchors = new Set();
  const counts = new Map();

  for (const match of source.matchAll(/^ {0,3}#{1,6}[ \t]+(.+?)\s*#*\s*$/gm)) {
    const base = stripHeadingMarkup(match[1])
      .toLowerCase()
      .replace(/[^\p{L}\p{N}\s_-]/gu, "")
      .replace(/\s+/g, "-");
    if (!base) continue;

    const duplicateIndex = counts.get(base) ?? 0;
    counts.set(base, duplicateIndex + 1);
    anchors.add(duplicateIndex === 0 ? base : `${base}-${duplicateIndex}`);
  }

  return anchors;
}

/**
 * Collect source and poster paths from the supported DocsVideo MDX component.
 *
 * @param {string} markdown Markdown with code regions removed.
 * @returns {string[]} Local or external media destinations.
 */
function collectDocsVideoDestinations(markdown) {
  const destinations = [];
  for (const tag of markdown.matchAll(/<DocsVideo\b[\s\S]*?\/>/g)) {
    for (const attribute of tag[0].matchAll(
      /\b(?:webm|mp4|poster)=(?:"([^"]+)"|'([^']+)')/g,
    )) {
      destinations.push(attribute[1] ?? attribute[2]);
    }
  }
  return destinations;
}

/**
 * Collect inline Markdown destinations while preserving balanced parentheses.
 *
 * @param {string} markdown Markdown with code regions removed.
 * @returns {string[]} Raw content inside each link's parentheses.
 */
function collectInlineLinkDestinations(markdown) {
  const destinations = [];

  for (let start = 0; start < markdown.length; start += 1) {
    if (markdown[start] !== "[" || isEscaped(markdown, start)) continue;

    let bracketDepth = 1;
    let labelEnd = -1;
    for (let cursor = start + 1; cursor < markdown.length; cursor += 1) {
      const character = markdown[cursor];
      if (character === "\n" || character === "\r") break;
      if (character === "\\") {
        cursor += 1;
      } else if (character === "[") {
        bracketDepth += 1;
      } else if (character === "]") {
        bracketDepth -= 1;
        if (bracketDepth === 0) {
          labelEnd = cursor;
          break;
        }
      }
    }

    if (labelEnd === -1 || markdown[labelEnd + 1] !== "(") continue;

    let parenthesisDepth = 1;
    for (let cursor = labelEnd + 2; cursor < markdown.length; cursor += 1) {
      const character = markdown[cursor];
      if (character === "\n" || character === "\r") break;
      if (character === "\\") {
        cursor += 1;
      } else if (character === "(") {
        parenthesisDepth += 1;
      } else if (character === ")") {
        parenthesisDepth -= 1;
        if (parenthesisDepth === 0) {
          destinations.push(markdown.slice(labelEnd + 2, cursor));
          break;
        }
      }
    }
  }

  return destinations;
}

/**
 * Return whether punctuation is preceded by an odd number of backslashes.
 *
 * @param {string} value Source text.
 * @param {number} index Character index.
 * @returns {boolean} Whether the character is escaped.
 */
function isEscaped(value, index) {
  let backslashes = 0;
  for (
    let cursor = index - 1;
    cursor >= 0 && value[cursor] === "\\";
    cursor -= 1
  ) {
    backslashes += 1;
  }
  return backslashes % 2 === 1;
}

/**
 * Remove fenced, indented, and inline code so examples are not treated as
 * live links.
 *
 * @param {string} markdown Page source.
 * @param {{keepInlineCode?: boolean}} [options] Inline-code handling.
 * @returns {string} Markdown with code regions removed.
 */
function stripMarkdownCode(markdown, { keepInlineCode = false } = {}) {
  let fence = null;
  let indentedCodeIndent = null;
  let canStartIndentedCode = true;
  const listContentIndents = [];
  const lines = markdown.split(/\r?\n/).map((line) => {
    const marker = line.match(/^\s*(`{3,}|~{3,})/)?.[1];
    if (marker) {
      if (!fence) {
        fence = marker;
      } else if (marker[0] === fence[0] && marker.length >= fence.length) {
        fence = null;
      }
      canStartIndentedCode = true;
      return "";
    }
    if (fence) return "";

    const blank = /^\s*$/.test(line);
    const indent = leadingIndentWidth(line);
    if (indentedCodeIndent !== null) {
      if (blank || indent >= indentedCodeIndent) {
        canStartIndentedCode = blank;
        return "";
      }
      indentedCodeIndent = null;
    }

    if (blank) {
      canStartIndentedCode = true;
      return line;
    }

    while (
      listContentIndents.length > 0 &&
      indent < listContentIndents.at(-1)
    ) {
      listContentIndents.pop();
    }

    const requiredCodeIndent = (listContentIndents.at(-1) ?? 0) + 4;
    if (canStartIndentedCode && indent >= requiredCodeIndent) {
      indentedCodeIndent = requiredCodeIndent;
      canStartIndentedCode = false;
      return "";
    }

    const listMarker = line.match(/^([ \t]*)(?:[-+*]|\d{1,9}[.)])([ \t]+)/);
    if (listMarker) {
      listContentIndents.push(columnWidth(listMarker[0]));
    }

    canStartIndentedCode =
      /^(?: {0,3}#{1,6}(?:[ \t]+|$)| {0,3}(?:=+|-+)[ \t]*$| {0,3}\[[^\]\n]+\]:)/.test(
        line,
      );
    return line;
  });

  const withoutBlocks = lines.join("\n");
  return keepInlineCode
    ? withoutBlocks
    : withoutBlocks.replace(/`+[^`\n]*`+/g, "");
}

/**
 * Count indentation columns, expanding tabs to four-column stops.
 *
 * @param {string} value Source line or prefix.
 * @returns {number} Leading indentation width in columns.
 */
function leadingIndentWidth(value) {
  return columnWidth(value.match(/^[ \t]*/)[0]);
}

/**
 * Count source columns, expanding tabs to four-column stops.
 *
 * @param {string} value Source text.
 * @returns {number} Width in columns.
 */
function columnWidth(value) {
  let width = 0;
  for (const character of value) {
    if (character === "\t") {
      width += 4 - (width % 4);
    } else {
      width += 1;
    }
  }
  return width;
}

/**
 * Apply CommonMark's case-insensitive, whitespace-collapsing reference label rules.
 *
 * @param {string} label Raw reference label.
 * @returns {string} Normalized reference label.
 */
function normalizeReferenceLabel(label) {
  return label.trim().replace(/\s+/g, " ").toLowerCase();
}

/**
 * Read the destination portion before an optional Markdown link title.
 *
 * @param {string} raw Raw content inside link parentheses.
 * @returns {string} Link destination.
 */
function parseMarkdownDestination(raw) {
  const value = raw.trim();
  if (value.startsWith("<")) {
    const end = value.indexOf(">");
    return end === -1 ? value : value.slice(1, end);
  }
  return value.split(/\s+/, 1)[0];
}

/**
 * Return whether a destination has a URL scheme or protocol-relative host.
 *
 * @param {string} href Link destination.
 * @returns {boolean} Whether the destination is external.
 */
function isExternalDestination(href) {
  return href.startsWith("//") || /^[a-z][a-z\d+.-]*:/i.test(href);
}

/**
 * Return whether a metadata entry is a navigation heading.
 *
 * @param {string} entry Navigation metadata entry.
 * @returns {boolean} Whether the entry is a heading decoration.
 */
function isNavigationDecoration(entry) {
  return /^---.*---$/.test(entry);
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  validatePublicDocs()
    .then(({ pageCount }) =>
      console.log(`Validated ${pageCount} published docs pages.`),
    )
    .catch((error) => {
      console.error(error.message);
      process.exitCode = 1;
    });
}
