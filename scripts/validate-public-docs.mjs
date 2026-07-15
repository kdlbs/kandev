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
    if (path.posix.basename(file).toLowerCase() === "readme.md") continue;

    const markdown = await fs.readFile(path.join(docsDir, file), "utf8");
    assertFrontmatter(file, markdown);

    const slug = file.replace(/\.mdx?$/, "").replace(/\/index$/, "") || "index";
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
