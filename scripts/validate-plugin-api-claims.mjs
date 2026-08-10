import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const defaultTypesPath = path.join(
  repoRoot,
  "apps/web/lib/plugins/types.ts",
);

const SKIP_DIRS = new Set([
  "node_modules",
  ".git",
  "dist",
  "build",
  "coverage",
  ".next",
  "target",
  "vendor",
  ".claude",
]);

const CLAIM_PATTERNS = [
  /\bdoes not expose\b/i,
  /\bnot exposed\b/i,
  /\babsent from\b/i,
  /\b(?:is|are) not available\b/i,
  /\bunavailable\b/i,
  /\bdoes not (?:yet )?(?:exist|support|provide)\b/i,
  /\bno\s+(?:[\w'-]+\s+){0,2}hook\b/i,
  /\bdo not invent hooks such as\b/i,
];

/**
 * Extract the `{ ... }` body of a top-level `export interface <name>` block.
 *
 * @param {string} source TypeScript source.
 * @param {string} interfaceName Interface identifier to locate.
 * @returns {string | null} The interface body, or null if not found.
 */
function extractInterfaceBody(source, interfaceName) {
  const marker = `export interface ${interfaceName}`;
  const markerIndex = source.indexOf(marker);
  if (markerIndex === -1) return null;
  const braceStart = source.indexOf("{", markerIndex + marker.length);
  if (braceStart === -1) return null;

  let depth = 1;
  let i = braceStart + 1;
  while (i < source.length && depth > 0) {
    if (source[i] === "{") depth++;
    else if (source[i] === "}") depth--;
    i++;
  }
  return source.slice(braceStart + 1, i - 1);
}

/**
 * Extract `register*` method names declared in a `PluginRegistry`-shaped body.
 *
 * @param {string} body Interface body text.
 * @returns {Set<string>} Registered method names.
 */
function extractRegisterMethods(body) {
  const names = new Set();
  const re = /\b(register[A-Z]\w*)\s*\(/g;
  let match;
  while ((match = re.exec(body))) names.add(match[1]);
  return names;
}

/**
 * Extract top-level property names from an interface body, skipping
 * comments and properties nested inside inline object types.
 *
 * @param {string} body Interface body text.
 * @returns {Set<string>} Top-level property names.
 */
function extractTopLevelProperties(body) {
  const names = new Set();
  let depth = 0;
  let inBlockComment = false;

  for (const rawLine of body.split("\n")) {
    const line = rawLine.trim();
    if (!line) continue;

    if (inBlockComment) {
      if (line.includes("*/")) inBlockComment = false;
      continue;
    }
    if (line.startsWith("/*")) {
      if (!line.includes("*/")) inBlockComment = true;
      continue;
    }
    if (line.startsWith("*") || line.startsWith("//")) continue;

    if (depth === 0) {
      const match = /^(\w+)\??\s*:/.exec(line);
      if (match) names.add(match[1]);
    }

    for (const ch of line) {
      if (ch === "{") depth++;
      else if (ch === "}") depth = Math.max(0, depth - 1);
    }
  }

  return names;
}

/**
 * Extract PascalCase component names listed in the JSDoc comment
 * immediately (contiguously, with no other declaration in between) above a
 * given property declaration.
 *
 * @param {string} body Interface body text.
 * @param {string} propertyName Property whose leading comment to read.
 * @returns {Set<string>} Component names mentioned in the comment.
 */
function extractCommentComponentNames(body, propertyName) {
  const names = new Set();
  const propertyRe = new RegExp(`^${propertyName}\\??\\s*:`);
  let pendingComment = [];
  let inBlockComment = false;

  for (const rawLine of body.split("\n")) {
    const line = rawLine.trim();

    if (inBlockComment) {
      pendingComment.push(line);
      if (line.includes("*/")) inBlockComment = false;
      continue;
    }
    if (line.startsWith("/*")) {
      pendingComment = [line];
      inBlockComment = !line.includes("*/");
      continue;
    }
    if (line.startsWith("//")) {
      pendingComment = [];
      continue;
    }
    if (line === "") continue;

    if (propertyRe.test(line)) {
      const commentText = pendingComment
        .map((commentLine) => commentLine.replace(/^\/?\*+\/?/, " "))
        .join(" ");
      const groups = commentText.match(/\(([^)]*)\)/g) || [];
      for (const group of groups) {
        for (const token of group.slice(1, -1).split(",")) {
          const trimmed = token.trim();
          if (/^[A-Z][A-Za-z0-9]*$/.test(trimmed)) names.add(trimmed);
        }
      }
    }
    pendingComment = [];
  }

  return names;
}

/**
 * Extract quoted named-slot string literals from the JSDoc comment above a
 * given type alias.
 *
 * @param {string} source Full TypeScript source.
 * @param {string} typeName Type alias whose leading comment to read.
 * @returns {Set<string>} Slot name literals.
 */
function extractSlotNames(source, typeName) {
  const names = new Set();
  const declarationRe = new RegExp(`^export type ${typeName}\\b`);
  let pendingComment = [];
  let inBlockComment = false;

  for (const rawLine of source.split("\n")) {
    const line = rawLine.trim();

    if (inBlockComment) {
      pendingComment.push(line);
      if (line.includes("*/")) inBlockComment = false;
      continue;
    }
    if (line.startsWith("/*")) {
      pendingComment = [line];
      inBlockComment = !line.includes("*/");
      continue;
    }
    if (line.startsWith("//")) {
      pendingComment = [];
      continue;
    }
    if (line === "") continue;

    if (declarationRe.test(line)) {
      const commentText = pendingComment.join(" ");
      const re = /"([a-z][a-z0-9-]*)"/g;
      let m;
      while ((m = re.exec(commentText))) names.add(m[1]);
      return names;
    }
    pendingComment = [];
  }

  return names;
}

/**
 * Mechanically derive the set of live plugin API names from the frontend
 * contract file — no curated list, so nothing here can go stale.
 *
 * @param {string} typesSource Contents of `lib/plugins/types.ts`.
 * @returns {Set<string>} Live API, property, component, and slot names.
 */
export function extractLiveNames(typesSource) {
  const names = new Set();

  const registryBody = extractInterfaceBody(typesSource, "PluginRegistry");
  if (registryBody) {
    for (const n of extractRegisterMethods(registryBody)) names.add(n);
  }

  const hostApiBody = extractInterfaceBody(typesSource, "PluginHostApi");
  if (hostApiBody) {
    for (const n of extractTopLevelProperties(hostApiBody)) names.add(n);
    for (const n of extractCommentComponentNames(hostApiBody, "ui")) {
      names.add(n);
    }
  }

  for (const n of extractSlotNames(typesSource, "PluginSlotName")) {
    names.add(n);
  }

  return names;
}

/**
 * Replace fenced code blocks with blank lines, preserving line numbers.
 *
 * @param {string} content Markdown content.
 * @returns {string} Content with fenced code block bodies blanked out.
 */
function stripFencedCodeBlocks(content) {
  const lines = content.split("\n");
  let inFence = false;
  return lines
    .map((line) => {
      if (/^\s*```/.test(line)) {
        inFence = !inFence;
        return "";
      }
      return inFence ? "" : line;
    })
    .join("\n");
}

/** Matches a line that always starts a fresh markdown block (list item, blockquote, heading, or table row). */
const BLOCK_START_RE = /^(#{1,6}\s|[-*+]\s|\d+[.)]\s|>|\|.*\|$)/;

/**
 * Split content into paragraphs, each carrying the 1-indexed line number it
 * starts on. A blank line always ends a paragraph. A list item, blockquote,
 * heading, or table row always starts a fresh paragraph — it never merges
 * with a sibling item — while its own wrapped continuation lines (plain text
 * with no marker) merge forward into it, so a claim wrapped across lines
 * within one block is still matched as one unit.
 *
 * @param {string} content Markdown content (fenced code already stripped).
 * @returns {Array<{line: number, text: string}>} Paragraphs.
 */
function splitParagraphs(content) {
  const lines = content.split("\n");
  const paragraphs = [];
  let current = [];
  let startLine = null;

  function flush() {
    if (current.length) {
      paragraphs.push({ line: startLine, text: current.join(" ") });
    }
    current = [];
    startLine = null;
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trim();
    if (trimmed === "") {
      flush();
      continue;
    }
    if (BLOCK_START_RE.test(trimmed)) flush();
    if (startLine === null) startLine = i + 1;
    current.push(trimmed);
  }
  flush();

  return paragraphs;
}

/**
 * Find paragraphs asserting that some API is unavailable.
 *
 * @param {string} content Markdown/file content.
 * @returns {Array<{line: number, text: string}>} Paragraphs matching an
 *   absence-claim pattern.
 */
export function findAbsenceClaims(content) {
  const paragraphs = splitParagraphs(stripFencedCodeBlocks(content));
  return paragraphs.filter((p) =>
    CLAIM_PATTERNS.some((pattern) => pattern.test(p.text)),
  );
}

/**
 * Resolve the fixed set of harness files this guard scans, de-duplicated by
 * physical (symlink-resolved) path.
 *
 * @param {string} root Repository root.
 * @returns {Promise<string[]>} Absolute, de-duplicated file paths.
 */
async function resolveScanFiles(root) {
  const found = [];

  async function walkForAgentsMd(dir) {
    let entries;
    try {
      entries = await fs.readdir(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      if (entry.isSymbolicLink()) continue;
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (SKIP_DIRS.has(entry.name)) continue;
        await walkForAgentsMd(full);
      } else if (entry.isFile() && entry.name === "AGENTS.md") {
        found.push(full);
      }
    }
  }

  async function walkMarkdown(dir) {
    let entries;
    try {
      entries = await fs.readdir(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory() && !entry.isSymbolicLink()) {
        await walkMarkdown(full);
      } else if (entry.isFile() && entry.name.endsWith(".md")) {
        found.push(full);
      }
    }
  }

  async function listMarkdown(dir) {
    let entries;
    try {
      entries = await fs.readdir(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      if (entry.isFile() && entry.name.endsWith(".md")) {
        found.push(path.join(dir, entry.name));
      }
    }
  }

  await walkForAgentsMd(root);
  await walkMarkdown(path.join(root, ".agents/skills"));
  await walkMarkdown(path.join(root, ".agents/agents"));
  await listMarkdown(path.join(root, "docs/plans/plugins"));

  const seen = new Map();
  for (const file of found) {
    let real;
    try {
      real = await fs.realpath(file);
    } catch {
      continue;
    }
    if (!seen.has(real)) seen.set(real, file);
  }
  return [...seen.keys()];
}

/**
 * Validate that no harness file claims a live plugin API is unavailable.
 *
 * @param {object} [options] Validation options.
 * @param {string} [options.repoRoot] Repository root to scan.
 * @param {string} [options.typesPath] Path to `lib/plugins/types.ts`.
 * @param {string[]} [options.scanFiles] Explicit file list, overriding the
 *   default harness scan set (used by tests).
 * @returns {Promise<{findings: Array<{file: string, line: number, api: string}>}>}
 */
export async function validatePluginApiClaims({
  repoRoot: root = repoRoot,
  typesPath = defaultTypesPath,
  scanFiles,
} = {}) {
  const typesSource = await fs.readFile(typesPath, "utf8");
  const liveNames = extractLiveNames(typesSource);
  const files = scanFiles ?? (await resolveScanFiles(root));

  const findings = [];
  for (const file of files) {
    const content = await fs.readFile(file, "utf8");
    const claims = findAbsenceClaims(content);
    if (!claims.length) continue;

    const relFile = path.relative(root, file);
    for (const claim of claims) {
      for (const name of liveNames) {
        const boundary = new RegExp(
          `\\b${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`,
        );
        if (boundary.test(claim.text)) {
          findings.push({ file: relFile, line: claim.line, api: name });
        }
      }
    }
  }

  return { findings };
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  validatePluginApiClaims()
    .then(({ findings }) => {
      if (!findings.length) {
        console.log("No stale plugin API absence claims found.");
        return;
      }
      for (const finding of findings) {
        console.error(
          `${finding.file}:${finding.line} — claims "${finding.api}" is unavailable, but lib/plugins/types.ts declares it`,
        );
      }
      process.exitCode = 1;
    })
    .catch((error) => {
      console.error(error.message);
      process.exitCode = 1;
    });
}
