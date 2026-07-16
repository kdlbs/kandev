import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

/**
 * Validate that every published Markdown page has frontmatter and appears
 * exactly once in the public navigation metadata.
 *
 * @param {string} [docsDir] Directory containing published docs and meta.json.
 * @returns {Promise<{pageCount: number}>} Number of validated published pages.
 */
export async function validatePublicDocs(
  docsDir = path.join(repoRoot, "docs/public"),
) {
  const meta = await readMeta(docsDir);
  const files = await collectMarkdownFiles(docsDir);
  const pagesBySlug = new Map();

  for (const file of files) {
    const markdown = await fs.readFile(path.join(docsDir, file), "utf8");
    await assertLocalLinks(docsDir, file, markdown);

    if (path.posix.basename(file).toLowerCase() === "readme.md") continue;

    assertFrontmatter(file, markdown);

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

  return { pageCount: pagesBySlug.size };
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
 * Require every relative Markdown link or image to resolve on disk.
 *
 * @param {string} docsDir Published docs root.
 * @param {string} file Relative source path used in validation errors.
 * @param {string} markdown Page source.
 * @returns {Promise<void>}
 */
async function assertLocalLinks(docsDir, file, markdown) {
  const source = stripMarkdownCode(markdown);
  const linkPattern = /!?\[[^\]\n]*\]\(((?:[^)\n\\]|\\.)+)\)/g;
  const definitionPattern = /^\s{0,3}\[([^\]\n]+)\]:\s*(\S.*)$/gm;
  const referencePattern = /!?\[([^\]\n]+)\]\[([^\]\n]*)\]/g;
  const shortcutReferencePattern = /(?<![!\\\[\]])\[([^\]\n]+)\](?![\[(:])/g;
  const destinations = [...source.matchAll(linkPattern)].map((match) => match[1]);
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
    if (!href || href.startsWith("#") || isExternalDestination(href)) {
      continue;
    }
    if (href.startsWith("/")) {
      throw new Error(
        `${file} uses a site-root link instead of a relative source link: ${href}`,
      );
    }

    const pathOnly = href.split(/[?#]/, 1)[0];
    if (!pathOnly) continue;

    let decoded;
    try {
      decoded = decodeURIComponent(pathOnly);
    } catch {
      throw new Error(`${file} contains an invalid encoded local link: ${href}`);
    }

    const target = path.resolve(
      path.dirname(path.join(docsDir, file)),
      decoded.replace(/\\([\\() ])/g, "$1"),
    );
    try {
      await fs.access(target);
    } catch {
      throw new Error(`${file} links to missing local target: ${href}`);
    }
  }
}

/**
 * Remove fenced and inline code so examples are not treated as live links.
 * Note: 4-space indented code blocks are not stripped; use fenced blocks in
 * published pages.
 *
 * @param {string} markdown Page source.
 * @returns {string} Markdown with code regions removed.
 */
function stripMarkdownCode(markdown) {
  let fence = null;
  const lines = markdown.split(/\r?\n/).map((line) => {
    const marker = line.match(/^\s*(`{3,}|~{3,})/)?.[1];
    if (marker) {
      if (!fence) {
        fence = marker;
      } else if (marker[0] === fence[0] && marker.length >= fence.length) {
        fence = null;
      }
      return "";
    }
    return fence ? "" : line;
  });

  return lines.join("\n").replace(/`+[^`\n]*`+/g, "");
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
