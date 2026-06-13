/**
 * Pure markdown normalization plus a bounded, value-keyed LRU cache.
 *
 * `normalizeMarkdown` is a pure string transform (moved here from
 * markdown-components so the cache has no React dependency). `normalizeCached`
 * wraps it in a hard-capped LRU keyed by the raw input string, so repeated
 * renders of the same content never re-run the transform. The cache is keyed by
 * content only (never by task/session/message id), so it cannot leak per-task
 * state: entries are anonymous strings that age out via LRU.
 */

const FENCE_OPEN_RE = /^ {0,3}(`{3,})/;
const TRAILING_FENCE_RE = /(`{3,})\s*$/;

function pureCloseLength(line: string, openCount: number): number | null {
  const match = /^ {0,3}(`{3,})\s*$/.exec(line);
  if (!match || match[1].length < openCount) return null;
  return match[1].length;
}

function gluedCloseLength(line: string, openCount: number): number | null {
  const match = TRAILING_FENCE_RE.exec(line);
  if (!match || match[1].length < openCount) return null;
  // Reject pure-fence lines (already handled by pureCloseLength) and lines
  // where everything before the trailing run is whitespace only.
  const head = line.slice(0, line.length - match[0].length).trimEnd();
  if (head.length === 0) return null;
  return match[1].length;
}

/**
 * Pre-process a markdown string to repair fenced code blocks that have their
 * closing fence glued to the last code line (`...}\`\`\`\n`prose`). Without
 * this, CommonMark/GFM treats the glued backticks as code content, so the
 * fence never closes and following prose gets swallowed into one huge code
 * node. We split such lines into `<content>\n<backticks>` only when we're
 * inside an open fence whose opener run length is ≤ the trailing run length.
 *
 * Pure string preprocessing, intentionally not a remark plugin.
 */
export function normalizeMarkdown(input: string): string {
  if (!input || input.length === 0) return input;
  const hadTrailingNewline = input.endsWith("\n");
  const lines = input.split("\n");
  const out: string[] = [];
  let openCount: number | null = null;

  for (const line of lines) {
    if (openCount === null) {
      const opener = FENCE_OPEN_RE.exec(line);
      if (opener) openCount = opener[1].length;
      out.push(line);
      continue;
    }
    if (pureCloseLength(line, openCount) !== null) {
      openCount = null;
      out.push(line);
      continue;
    }
    const glued = gluedCloseLength(line, openCount);
    if (glued !== null) {
      const trailingMatch = TRAILING_FENCE_RE.exec(line)!;
      const head = line.slice(0, line.length - trailingMatch[0].length);
      out.push(head);
      out.push("`".repeat(glued));
      openCount = null;
      continue;
    }
    out.push(line);
  }

  const result = out.join("\n");
  return hadTrailingNewline && !result.endsWith("\n") ? result + "\n" : result;
}

// ── Bounded LRU cache ───────────────────────────────────────────────

const MAX_CACHE_ENTRIES = 500;
const normalizeCache = new Map<string, string>();
let parseCount = 0;

/**
 * Cached variant of {@link normalizeMarkdown}. Keyed by the raw input string;
 * a hit refreshes recency (Map preserves insertion order, so delete+set moves
 * the entry to the end). On overflow the oldest entry is evicted.
 */
export function normalizeCached(input: string): string {
  const cached = normalizeCache.get(input);
  if (cached !== undefined) {
    normalizeCache.delete(input);
    normalizeCache.set(input, cached);
    return cached;
  }
  parseCount += 1;
  const result = normalizeMarkdown(input);
  normalizeCache.set(input, result);
  if (normalizeCache.size > MAX_CACHE_ENTRIES) {
    const oldest = normalizeCache.keys().next().value;
    if (oldest !== undefined) normalizeCache.delete(oldest);
  }
  return result;
}

// ── Test-only instrumentation ───────────────────────────────────────

/** Number of real normalize passes (cache misses) since the last reset. */
export function __markdownParseCount(): number {
  return parseCount;
}

/** Clears the cache and resets the parse counter. Test-only. */
export function __resetMarkdownCounters(): void {
  parseCount = 0;
  normalizeCache.clear();
}

/** Current cache size. Test-only (asserts the LRU stays bounded). */
export function __lruSize(): number {
  return normalizeCache.size;
}

/** Hard cap on cached entries. Test-only. */
export const __MAX_CACHE_ENTRIES = MAX_CACHE_ENTRIES;
