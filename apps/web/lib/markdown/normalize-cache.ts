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
const MARKDOWN_WRAPPER_OPEN_RE = /^( {0,3})(`{3,})(\s*(?:markdown|mdx?)(?:[ \t].*)?\s*)$/i;
const FENCE_OPENER_LINE_RE = /^ {0,3}(`{3,})(?!`)(?:[ \t]+\S|\S)/;
const PURE_FENCE_LINE_RE = /^( {0,3})(`{3,})\s*$/;
const TRAILING_FENCE_RE = /(`{3,})\s*$/;

function pureCloseLength(line: string, openCount: number): number | null {
  const match = PURE_FENCE_LINE_RE.exec(line);
  if (!match || match[2].length < openCount) return null;
  return match[2].length;
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

function lastNonBlankLineIndex(lines: string[]): number {
  for (let index = lines.length - 1; index >= 0; index--) {
    if (lines[index].trim() !== "") return index;
  }
  return -1;
}

function firstNonBlankLineIndex(lines: string[]): number {
  for (let index = 0; index < lines.length; index++) {
    if (lines[index].trim() !== "") return index;
  }
  return -1;
}

type InnerFenceScan = {
  maxInnerPureFence: number;
  nestedFenceCount: number;
};

type NestedFenceStep = {
  failed: boolean;
  nextIndex: number;
};

type NestedFenceContext = {
  closeIndex: number;
  lines: string[];
  scan: InnerFenceScan;
  wrapperOpenCount: number;
};

function matchingCloseLength(line: string, openCount: number): number | null {
  return pureCloseLength(line, openCount) ?? gluedCloseLength(line, openCount);
}

function findTaggedNestedClose(
  lines: string[],
  startIndex: number,
  closeIndex: number,
  openCount: number,
) {
  for (let index = startIndex + 1; index < closeIndex; index++) {
    const closeCount = matchingCloseLength(lines[index], openCount);
    if (closeCount !== null) return { closeCount, closeIndex: index };
  }
  return null;
}

function findBareNestedClose(
  lines: string[],
  startIndex: number,
  closeIndex: number,
  openCount: number,
  wrapperOpenCount: number,
) {
  for (let index = startIndex + 1; index < closeIndex; index++) {
    const closeCount = matchingCloseLength(lines[index], openCount);
    if (closeCount !== null) {
      if (
        openCount === wrapperOpenCount &&
        isSameLengthBareMarkdownSampleClose(lines, startIndex, index, openCount)
      ) {
        return null;
      }
      return { closeCount, closeIndex: index };
    }
  }
  return null;
}

function nearestNonBlankBefore(lines: string[], index: number): string {
  for (let cursor = index - 1; cursor >= 0; cursor--) {
    const line = lines[cursor].trim();
    if (line !== "") return line;
  }
  return "";
}

function hasTaggedLookingBareContent(
  lines: string[],
  startIndex: number,
  closeIndex: number,
  openCount: number,
): boolean {
  for (let index = startIndex + 1; index < closeIndex; index++) {
    const taggedOpener = FENCE_OPENER_LINE_RE.exec(lines[index]);
    if ((taggedOpener?.[1]?.length ?? 0) >= openCount) return true;
  }
  return false;
}

function isSameLengthBareMarkdownSampleClose(
  lines: string[],
  startIndex: number,
  closeIndex: number,
  openCount: number,
): boolean {
  if (hasTaggedLookingBareContent(lines, startIndex, closeIndex, openCount)) return false;
  if (lines[closeIndex + 1]?.trim() === "") return false;
  const previousContent = nearestNonBlankBefore(lines, startIndex);
  return previousContent.startsWith("#") || !previousContent.endsWith(":");
}

function noteNestedFence(state: InnerFenceScan, openCount: number, closeCount: number): void {
  state.nestedFenceCount += 1;
  state.maxInnerPureFence = Math.max(state.maxInnerPureFence, openCount, closeCount);
}

function taggedNestedFenceClose(
  lines: string[],
  startIndex: number,
  closeIndex: number,
  taggedOpenCount: number,
  wrapperOpenCount: number,
) {
  const taggedClose = findTaggedNestedClose(lines, startIndex, closeIndex, taggedOpenCount);
  if (taggedOpenCount < wrapperOpenCount && (taggedClose?.closeCount ?? 0) < wrapperOpenCount) {
    return null;
  }
  return taggedClose;
}

function bareNestedFenceClose(
  lines: string[],
  startIndex: number,
  closeIndex: number,
  bareOpenCount: number,
  wrapperOpenCount: number,
) {
  const bareClose = findBareNestedClose(
    lines,
    startIndex,
    closeIndex,
    bareOpenCount,
    wrapperOpenCount,
  );
  if (bareOpenCount < wrapperOpenCount && (bareClose?.closeCount ?? 0) < wrapperOpenCount) {
    return null;
  }
  return bareClose;
}

function scanTaggedNestedFence(
  context: NestedFenceContext,
  index: number,
  taggedOpenCount: number,
): NestedFenceStep {
  const taggedClose = taggedNestedFenceClose(
    context.lines,
    index,
    context.closeIndex,
    taggedOpenCount,
    context.wrapperOpenCount,
  );
  if (!taggedClose) {
    return {
      failed: taggedOpenCount >= context.wrapperOpenCount && context.scan.nestedFenceCount === 0,
      nextIndex: index,
    };
  }
  noteNestedFence(context.scan, taggedOpenCount, taggedClose.closeCount);
  return { failed: false, nextIndex: taggedClose.closeIndex };
}

function scanBareNestedFence(
  context: NestedFenceContext,
  index: number,
  bareOpenCount: number,
): NestedFenceStep {
  const bareClose = bareNestedFenceClose(
    context.lines,
    index,
    context.closeIndex,
    bareOpenCount,
    context.wrapperOpenCount,
  );
  if (!bareClose) return { failed: bareOpenCount >= context.wrapperOpenCount, nextIndex: index };
  noteNestedFence(context.scan, bareOpenCount, bareClose.closeCount);
  return { failed: false, nextIndex: bareClose.closeIndex };
}

function markdownWrapperInnerFenceInfo(
  lines: string[],
  openIndex: number,
  closeIndex: number,
  openCount: number,
) {
  const scan: InnerFenceScan = {
    maxInnerPureFence: 0,
    nestedFenceCount: 0,
  };
  const context: NestedFenceContext = {
    closeIndex,
    lines,
    scan,
    wrapperOpenCount: openCount,
  };

  for (let index = openIndex + 1; index < closeIndex; index++) {
    const taggedOpener = FENCE_OPENER_LINE_RE.exec(lines[index]);
    const taggedOpenCount = taggedOpener?.[1]?.length;
    if (taggedOpenCount) {
      const step = scanTaggedNestedFence(context, index, taggedOpenCount);
      if (step.failed) return null;
      index = step.nextIndex;
      continue;
    }

    const bareOpener = PURE_FENCE_LINE_RE.exec(lines[index]);
    const bareOpenCount = bareOpener?.[2]?.length;
    if (bareOpenCount) {
      const step = scanBareNestedFence(context, index, bareOpenCount);
      if (step.failed) return null;
      index = step.nextIndex;
    }
  }

  if (scan.nestedFenceCount === 0 || scan.maxInnerPureFence === 0) return null;
  return { maxInnerPureFence: scan.maxInnerPureFence };
}

function wrapperCloseLength(lines: string[], closeIndex: number, openCount: number): number | null {
  return (
    pureCloseLength(lines[closeIndex], openCount) ?? gluedCloseLength(lines[closeIndex], openCount)
  );
}

function writeStrengthenedWrapperClose(
  lines: string[],
  closeIndex: number,
  targetCount: number,
): void {
  const closer = PURE_FENCE_LINE_RE.exec(lines[closeIndex]);
  if (closer) {
    lines[closeIndex] = `${closer[1]}${"`".repeat(targetCount)}`;
    return;
  }

  const closeLine = lines[closeIndex];
  const trailingMatch = TRAILING_FENCE_RE.exec(closeLine)!;
  const head = closeLine.slice(0, closeLine.length - trailingMatch[0].length);
  lines.splice(closeIndex, 1, head, "`".repeat(targetCount));
}

function strengthenMarkdownWrapperFence(lines: string[]): string[] {
  const openIndex = firstNonBlankLineIndex(lines);
  if (openIndex < 0) return lines;

  const opener = MARKDOWN_WRAPPER_OPEN_RE.exec(lines[openIndex] ?? "");
  if (!opener || lines.length < 3) return lines;

  const closeIndex = lastNonBlankLineIndex(lines);
  if (closeIndex <= openIndex + 1) return lines;

  const openCount = opener[2].length;
  const closeCount = wrapperCloseLength(lines, closeIndex, openCount);
  if (closeCount === null) return lines;
  if (closeCount < openCount) return lines;

  const innerInfo = markdownWrapperInnerFenceInfo(lines, openIndex, closeIndex, openCount);
  if (!innerInfo) return lines;

  const targetCount = Math.max(openCount + 1, closeCount, innerInfo.maxInnerPureFence + 1);
  const strengthened = lines.slice();
  strengthened[openIndex] = `${opener[1]}${"`".repeat(targetCount)}${opener[3]}`;
  writeStrengthenedWrapperClose(strengthened, closeIndex, targetCount);
  return strengthened;
}

/**
 * Pre-process a markdown string to repair common malformed LLM fence output:
 * - whole-message `markdown` wrappers that contain nested fenced blocks using
 *   the same backtick run length;
 * - closing fences glued to the last code line (`...}\`\`\`\n`prose`).
 *
 * For glued fences, CommonMark/GFM treats the glued backticks as code content,
 * so the fence never closes and following prose gets swallowed into one huge
 * code node. We split such lines into `<content>\n<backticks>` only when we're
 * inside an open fence whose opener run length is ≤ the trailing run length.
 *
 * Pure string preprocessing, intentionally not a remark plugin.
 */
export function normalizeMarkdown(input: string): string {
  if (!input || input.length === 0) return input;
  const hadTrailingNewline = input.endsWith("\n");
  const lines = strengthenMarkdownWrapperFence(input.split("\n"));
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
  // Evict before insert so the map never exceeds the cap. A miss means the key
  // isn't present, so the cache holds at most MAX_CACHE_ENTRIES - 1 entries here.
  if (normalizeCache.size >= MAX_CACHE_ENTRIES) {
    const oldest = normalizeCache.keys().next().value;
    if (oldest !== undefined) normalizeCache.delete(oldest);
  }
  normalizeCache.set(input, result);
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
