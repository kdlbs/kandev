#!/usr/bin/env node
/**
 * Fail when user-visible UI copy contains a Unicode em dash.
 *
 * The check covers locale values, rendered web source strings, backend mock-agent
 * response strings, backend shared page catalogs, and published Markdown. Historical
 * changelog content is intentionally excluded because the changelog is generated
 * release history and must remain immutable.
 * Comments are ignored in source files so this remains a copy check rather than
 * a style check for developer prose.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const EM_DASH = "\u2014";

const WEB_ROOT = path.resolve(import.meta.dirname, "..");
const REPO_ROOT = path.resolve(WEB_ROOT, "..", "..");
const SOURCE_EXTENSIONS = new Set([".js", ".jsx", ".ts", ".tsx"]);
const BACKEND_SOURCE_EXTENSIONS = new Set([".go"]);
const PUBLIC_DOC_EXTENSIONS = new Set([".md", ".mdx"]);
const IGNORED_DIRECTORIES = new Set(["dist", "e2e", "node_modules"]);
const CODE_STATE = "code";
const LINE_COMMENT_STATE = "line-comment";
const BLOCK_COMMENT_STATE = "block-comment";
const STRING_STATE = "string";
const REGEX_START_CHARACTERS = new Set("=([{,:;!?&|+-*%^~<>".split(""));
const REGEX_KEYWORDS = new Set([
  "await",
  "case",
  "delete",
  "do",
  "else",
  "in",
  "instanceof",
  "new",
  "of",
  "return",
  "throw",
  "typeof",
  "void",
  "while",
  "yield",
]);

export function containsEmDash(value) {
  return value.includes(EM_DASH) || /\\u2014/i.test(value);
}

function relativePath(file, root) {
  return path.relative(root, file).split(path.sep).join("/");
}

function commentCharacter(character, state) {
  return character === "\n"
    ? { output: character, nextState: state === LINE_COMMENT_STATE ? CODE_STATE : state }
    : { output: " ", nextState: state };
}

function consumeLineComment(source, index) {
  return { ...commentCharacter(source[index], LINE_COMMENT_STATE), nextIndex: index + 1 };
}

function consumeBlockComment(source, index) {
  const closesComment = source[index] === "*" && source[index + 1] === "/";
  if (closesComment) return { output: "  ", nextIndex: index + 2, nextState: CODE_STATE };
  return { ...commentCharacter(source[index], BLOCK_COMMENT_STATE), nextIndex: index + 1 };
}

function consumeString(source, index, quote, escaped) {
  const character = source[index];
  const closesString = !escaped && character === quote;
  return {
    output: character,
    nextIndex: index + 1,
    nextState: closesString ? CODE_STATE : STRING_STATE,
    nextQuote: closesString ? "" : quote,
    nextEscaped: closesString ? false : !escaped && character === "\\",
  };
}

function previousToken(source, index) {
  let cursor = index - 1;
  while (cursor >= 0 && /\s/.test(source[cursor])) cursor -= 1;
  const end = cursor;
  while (cursor >= 0 && /[\w$]/.test(source[cursor])) cursor -= 1;
  return source.slice(cursor + 1, end + 1);
}

function canStartRegex(source, index) {
  if (source[index - 1] === "<") return false;
  let cursor = index - 1;
  while (cursor >= 0 && /\s/.test(source[cursor])) cursor -= 1;
  if (cursor < 0) return true;
  if (REGEX_START_CHARACTERS.has(source[cursor])) return true;
  return REGEX_KEYWORDS.has(previousToken(source, index));
}

function consumeRegex(source, index) {
  let inCharacterClass = false;
  let escaped = false;

  for (let cursor = index + 1; cursor < source.length; cursor += 1) {
    const character = source[cursor];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (character === "\\") {
      escaped = true;
      continue;
    }
    if (character === "[") {
      inCharacterClass = true;
      continue;
    }
    if (character === "]") {
      inCharacterClass = false;
      continue;
    }
    if (character !== "/" || inCharacterClass) continue;

    let end = cursor + 1;
    while (/[a-z]/i.test(source[end] ?? "")) end += 1;
    return {
      output: source.slice(index, end).replace(/[^\n]/g, " "),
      nextIndex: end,
      nextState: CODE_STATE,
    };
  }

  return { output: source[index], nextIndex: index + 1, nextState: CODE_STATE };
}

function consumeCode(source, index) {
  const character = source[index];
  const next = source[index + 1];
  if (character === "/" && next === "/") {
    return { output: "  ", nextIndex: index + 2, nextState: LINE_COMMENT_STATE };
  }
  if (character === "/" && next === "*") {
    return { output: "  ", nextIndex: index + 2, nextState: BLOCK_COMMENT_STATE };
  }
  if (character === "/" && canStartRegex(source, index)) return consumeRegex(source, index);
  if (["'", '"', "`"].includes(character)) {
    return {
      output: character,
      nextIndex: index + 1,
      nextState: STRING_STATE,
      nextQuote: character,
      nextEscaped: false,
    };
  }
  return { output: character, nextIndex: index + 1, nextState: CODE_STATE };
}

const STATE_CONSUMERS = {
  [LINE_COMMENT_STATE]: consumeLineComment,
  [BLOCK_COMMENT_STATE]: consumeBlockComment,
};

function consumeState(source, index, state, quote, escaped) {
  if (state === STRING_STATE) return consumeString(source, index, quote, escaped);
  return (STATE_CONSUMERS[state] ?? consumeCode)(source, index);
}

/** Remove comments while retaining strings and source line positions. */
export function stripCommentsPreservingStrings(source) {
  let output = "";
  let state = CODE_STATE;
  let quote = "";
  let escaped = false;

  for (let index = 0; index < source.length; ) {
    const consumed = consumeState(source, index, state, quote, escaped);
    output += consumed.output;
    index = consumed.nextIndex;
    state = consumed.nextState;
    quote = consumed.nextQuote ?? "";
    escaped = consumed.nextEscaped ?? false;
  }

  return output;
}

export function findSourceViolations(source, file, root) {
  const cleanedSource = stripCommentsPreservingStrings(source);
  return cleanedSource
    .split("\n")
    .flatMap((line, index) =>
      containsEmDash(line)
        ? [{ kind: "source", file: relativePath(file, root), line: index + 1 }]
        : [],
    );
}

export function findPublicDocViolations(source, file, root) {
  return source
    .split("\n")
    .flatMap((line, index) =>
      containsEmDash(line)
        ? [{ kind: "public-doc", file: relativePath(file, root), line: index + 1 }]
        : [],
    );
}

function listFiles(directory, predicate, files = []) {
  if (!fs.existsSync(directory)) return files;

  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    if (entry.isSymbolicLink()) continue;
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      if (!IGNORED_DIRECTORIES.has(entry.name)) listFiles(file, predicate, files);
    } else if (predicate(file)) {
      files.push(file);
    }
  }

  return files;
}

function collectCatalogStrings(value, keyPath, file, root, violations) {
  if (typeof value === "string") {
    if (containsEmDash(value)) {
      violations.push({ kind: "catalog", file: relativePath(file, root), key: keyPath });
    }
    return;
  }

  if (Array.isArray(value)) {
    value.forEach((item, index) =>
      collectCatalogStrings(item, `${keyPath}[${index}]`, file, root, violations),
    );
    return;
  }

  if (value && typeof value === "object") {
    for (const [key, child] of Object.entries(value)) {
      const childPath = keyPath ? `${keyPath}.${key}` : key;
      collectCatalogStrings(child, childPath, file, root, violations);
    }
  }
}

function scanCatalogDirectory(directory, root, violations) {
  for (const file of listFiles(directory, (candidate) => candidate.endsWith(".json"))) {
    let catalog;
    try {
      catalog = JSON.parse(fs.readFileSync(file, "utf8"));
    } catch (error) {
      throw new Error(`Unable to parse locale catalog ${file}: ${error.message}`);
    }
    collectCatalogStrings(catalog, "", file, root, violations);
  }
}

export function scanPublicDocsEmDashViolations({
  docsRoot = path.join(REPO_ROOT, "docs", "public"),
  root = REPO_ROOT,
} = {}) {
  const violations = [];
  for (const file of listFiles(docsRoot, (candidate) =>
    PUBLIC_DOC_EXTENSIONS.has(path.extname(candidate).toLowerCase()),
  )) {
    violations.push(...findPublicDocViolations(fs.readFileSync(file, "utf8"), file, root));
  }
  return violations;
}

export function scanUiEmDashViolations({ repoRoot = REPO_ROOT, webRoot = WEB_ROOT } = {}) {
  const violations = [];
  const sourceRoots = ["components", "app", "src", "hooks", "lib"].map((directory) =>
    path.join(webRoot, directory),
  );
  const catalogRoots = [
    path.join(webRoot, "src", "locales"),
    path.join(repoRoot, "apps", "backend", "internal", "i18n", "locales"),
  ];

  for (const directory of catalogRoots) scanCatalogDirectory(directory, repoRoot, violations);

  for (const directory of sourceRoots) {
    for (const file of listFiles(directory, (candidate) => {
      const extension = path.extname(candidate);
      return SOURCE_EXTENSIONS.has(extension) && !/\.(test|spec)\.[^.]+$/.test(candidate);
    })) {
      violations.push(...findSourceViolations(fs.readFileSync(file, "utf8"), file, repoRoot));
    }
  }

  const backendRenderedSourceRoots = [path.join(repoRoot, "apps", "backend", "cmd", "mock-agent")];
  for (const directory of backendRenderedSourceRoots) {
    for (const file of listFiles(directory, (candidate) => {
      return (
        BACKEND_SOURCE_EXTENSIONS.has(path.extname(candidate)) && !candidate.endsWith("_test.go")
      );
    })) {
      violations.push(...findSourceViolations(fs.readFileSync(file, "utf8"), file, repoRoot));
    }
  }

  violations.push(
    ...scanPublicDocsEmDashViolations({
      docsRoot: path.join(repoRoot, "docs", "public"),
      root: repoRoot,
    }),
  );

  return violations;
}

function formatViolation(violation) {
  const location = violation.key
    ? `${violation.file}:${violation.key}`
    : `${violation.file}:${violation.line}`;
  return `  ${location} (${violation.kind})`;
}

function main() {
  const violations = scanUiEmDashViolations();
  if (violations.length > 0) {
    console.error(`✗ ${violations.length} public-copy em dash violation(s):\n`);
    for (const violation of violations) console.error(formatViolation(violation));
    console.error("\nUse a period, colon, comma, semicolon, or parentheses in user-facing copy.");
    process.exitCode = 1;
    return;
  }

  console.log("✓ no em dashes in public copy or locale values.");
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
