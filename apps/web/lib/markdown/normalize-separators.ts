type FenceState = {
  character: "`" | "~";
  length: number;
  owner: ContainerKind | null;
};

type ContainerKind = "blockquote" | "list";

type ContainerState = {
  kind: ContainerKind;
  afterBlank: boolean;
};

export type MarkdownLine = {
  text: string;
  ending: string;
};

type HtmlBlockState = {
  kind: "basic" | "comment" | "cdata" | "raw-tag" | "processing-instruction" | "declaration";
  tag?: string;
};

const SEPARATOR_LINE_RE = /^ {0,3}-{3,}[ \t]*$/;
const FENCE_OPEN_LINE_RE = /^ {0,3}((?:`{3,}|~{3,}))/;
const FENCE_CLOSE_LINE_RE = /^ {0,3}((?:`{3,}|~{3,}))[ \t]*$/;
const ATX_HEADING_RE = /^ {0,3}#{1,6}(?:[ \t]|$)/;
const BLOCKQUOTE_RE = /^ {0,3}>/;
const LIST_ITEM_RE = /^ {0,3}(?:[-+*](?:[ \t]+|$)|\d{1,9}[.)](?:[ \t]+|$))/;
const LIST_FENCE_PREFIX_RE = /^ {0,3}(?:[-+*]|\d{1,9}[.)])[ \t]+(.+)$/;
const BLOCKQUOTE_FENCE_PREFIX_RE = /^ {0,3}>[ \t]?(.*)$/;
const HTML_TAG_RE = /^ {0,3}<\/?([A-Za-z][\w:-]*)(?:[ \t]+[^<>]*)?[ \t]*\/?>/;
const HTML_PROCESSING_INSTRUCTION_RE = /^ {0,3}<\?/;
const HTML_DECLARATION_RE = /^ {0,3}<![A-Z]/;
const HTML_RAW_TAGS = new Set(["pre", "script", "style", "textarea"]);

function withoutLineEnding(line: string): string {
  return line.endsWith("\r") ? line.slice(0, -1) : line;
}

function isBlank(line: string): boolean {
  return withoutLineEnding(line).trim() === "";
}

function isSeparatorLine(line: string): boolean {
  return SEPARATOR_LINE_RE.test(withoutLineEnding(line));
}

function isFenceOpen(line: string): FenceState | null {
  const match = FENCE_OPEN_LINE_RE.exec(withoutLineEnding(line));
  if (!match) return null;
  return { character: match[1][0] as "`" | "~", length: match[1].length, owner: null };
}

function isContainerFenceOpen(line: string): FenceState | null {
  const text = withoutLineEnding(line);
  const listMatch = LIST_FENCE_PREFIX_RE.exec(text);
  const listFence = listMatch ? isFenceOpen(listMatch[1]) : null;
  if (listFence) return { ...listFence, owner: "list" };

  const blockquoteMatch = BLOCKQUOTE_FENCE_PREFIX_RE.exec(text);
  const blockquoteFence = blockquoteMatch ? isFenceOpen(blockquoteMatch[1]) : null;
  return blockquoteFence ? { ...blockquoteFence, owner: "blockquote" } : null;
}

function isFenceClose(line: string, fence: FenceState): boolean {
  const match = FENCE_CLOSE_LINE_RE.exec(withoutLineEnding(line));
  return Boolean(match && match[1][0] === fence.character && match[1].length >= fence.length);
}

function isRuleLine(line: string): boolean {
  const text = withoutLineEnding(line).trim();
  if (SEPARATOR_LINE_RE.test(withoutLineEnding(line))) return true;
  for (const character of ["*", "_", "-"]) {
    const compact = text.replaceAll(" ", "").replaceAll("\t", "");
    if (compact.length >= 3 && compact.split("").every((item) => item === character)) return true;
  }
  return false;
}

function hasUnescapedPipe(line: string): boolean {
  const text = withoutLineEnding(line);
  for (let index = 0; index < text.length; index++) {
    if (text[index] === "|" && text[index - 1] !== "\\") return true;
  }
  return false;
}

function isBlockquoteOrListMarker(line: string): boolean {
  const text = withoutLineEnding(line);
  return BLOCKQUOTE_RE.test(text) || LIST_ITEM_RE.test(text);
}

function isStructuralLine(line: string): boolean {
  const text = withoutLineEnding(line);
  const trimmed = text.trim();
  return (
    /^\t/.test(text) ||
    /^ {4}/.test(text) ||
    ATX_HEADING_RE.test(text) ||
    isBlockquoteOrListMarker(text) ||
    FENCE_OPEN_LINE_RE.test(text) ||
    isRuleLine(text) ||
    hasUnescapedPipe(text) ||
    /^ {0,3}<\/?[A-Za-z!][^>]*>/.test(text) ||
    trimmed === ""
  );
}

function sentenceTerminator(line: string): boolean {
  const text = withoutLineEnding(line).trim();
  const withoutClosingMarkers = text.replace(/[\\`*_)\]}>'"]+$/u, "");
  return /[.!?]$/u.test(withoutClosingMarkers);
}

function isEligibleProse(line: string, container: ContainerState | null): boolean {
  const text = withoutLineEnding(line);
  const trimmed = text.trim();
  if (container || trimmed === "" || isStructuralLine(text)) return false;
  if (!/\s/u.test(trimmed)) return false;

  const codePointLength = Array.from(trimmed).length;
  return codePointLength >= 70 || (codePointLength >= 40 && sentenceTerminator(text));
}

function containerMarker(line: string): ContainerKind | null {
  const text = withoutLineEnding(line);
  if (BLOCKQUOTE_RE.test(text)) return "blockquote";
  if (LIST_ITEM_RE.test(text)) return "list";
  return null;
}

function updateContainer(
  current: ContainerState | null,
  line: string,
  eligible: boolean,
): ContainerState | null {
  if (isBlank(line)) {
    return current?.kind === "list" ? { ...current, afterBlank: true } : null;
  }
  if (current?.kind === "list" && isIndentedListContinuation(line)) {
    return { ...current, afterBlank: false };
  }
  if (current?.afterBlank) return null;
  if (isRuleLine(line)) return null;

  const marker = containerMarker(line);
  if (marker) return { kind: marker, afterBlank: false };
  if (eligible) return null;
  return current;
}

function isContainerBoundary(
  line: string,
  fenceStart: FenceState | null,
  htmlStart: HtmlBlockState | null,
  container: ContainerState | null,
): boolean {
  const text = withoutLineEnding(line);
  const isIndentedContinuation = container?.kind === "list" && isIndentedListContinuation(text);
  const listEndedAfterBlank =
    container?.kind === "list" && container.afterBlank && !isIndentedContinuation;
  return (
    listEndedAfterBlank ||
    (!isIndentedContinuation &&
      (ATX_HEADING_RE.test(text) ||
        isRuleLine(text) ||
        Boolean(htmlStart) ||
        (Boolean(fenceStart) && fenceStart?.owner === null)))
  );
}

function isIndentedListContinuation(line: string): boolean {
  return /^(?: {2,}|\t)\S/u.test(withoutLineEnding(line));
}

function htmlBlockStart(line: string): HtmlBlockState | null {
  const source = withoutLineEnding(line);
  const text = source.replace(/^ {0,3}/u, "");
  if (text.startsWith("<!--") && !text.includes("-->")) return { kind: "comment" };
  if (text.startsWith("<![CDATA[") && !text.includes("]]>")) {
    return { kind: "cdata" };
  }

  if (HTML_PROCESSING_INSTRUCTION_RE.test(source) && !text.includes("?>")) {
    return { kind: "processing-instruction" };
  }
  if (HTML_DECLARATION_RE.test(source) && !text.includes(">")) {
    return { kind: "declaration" };
  }

  const match = HTML_TAG_RE.exec(source);
  if (!match) return null;
  const tag = match[1].toLowerCase();
  const isClosing = /^ {0,3}<\//u.test(source);
  if (!isClosing && HTML_RAW_TAGS.has(tag)) return { kind: "raw-tag", tag };
  return { kind: "basic" };
}

function htmlBlockEnd(state: HtmlBlockState, line: string): boolean {
  const text = withoutLineEnding(line);
  if (state.kind === "comment") return text.includes("-->");
  if (state.kind === "cdata") return text.includes("]]>");
  if (state.kind === "processing-instruction") return text.includes("?>");
  if (state.kind === "declaration") return text.includes(">");
  if (state.kind === "raw-tag") {
    return Boolean(state.tag && new RegExp(`</${state.tag}\\s*>`, "i").test(text));
  }
  return isBlank(line);
}

function isFrontMatterBoundary(line: string): boolean {
  const text = withoutLineEnding(line).trim();
  return text === "---" || text === "...";
}

function isFrontMatterStart(line: string): boolean {
  const text = withoutLineEnding(line);
  return text === "---";
}

function firstNonBlankIndex(lines: MarkdownLine[]): number {
  return lines.findIndex((line) => !isBlank(line.text));
}

type SeparatorScanState = {
  container: ContainerState | null;
  fence: FenceState | null;
  frontMatter: boolean;
  frontMatterStart: number;
  htmlBlock: HtmlBlockState | null;
  output: MarkdownLine[];
  previousEligible: boolean;
};

function handleFrontMatterLine(
  state: SeparatorScanState,
  line: MarkdownLine,
  index: number,
): boolean {
  if (!state.frontMatter) return false;
  state.output.push(line);
  state.previousEligible = false;
  if (index > state.frontMatterStart && isFrontMatterBoundary(line.text)) state.frontMatter = false;
  return true;
}

function fenceCloseLine(line: string, fence: FenceState): string {
  if (fence.owner !== "blockquote") return line;
  return BLOCKQUOTE_FENCE_PREFIX_RE.exec(withoutLineEnding(line))?.[1] ?? line;
}

function handleFenceLine(state: SeparatorScanState, line: MarkdownLine): boolean {
  if (!state.fence) return false;
  state.output.push(line);
  state.previousEligible = false;
  if (isFenceClose(fenceCloseLine(line.text, state.fence), state.fence)) {
    const owner = state.fence.owner;
    state.fence = null;
    state.container = owner === "list" ? { kind: "list", afterBlank: false } : null;
  }
  return true;
}

function handleHtmlLine(state: SeparatorScanState, line: MarkdownLine): boolean {
  if (!state.htmlBlock) return false;
  state.output.push(line);
  state.previousEligible = false;
  if (htmlBlockEnd(state.htmlBlock, line.text)) state.htmlBlock = null;
  return true;
}

function handleSeparatorLine(
  state: SeparatorScanState,
  lines: MarkdownLine[],
  index: number,
): boolean {
  const line = lines[index];
  if (!line || !isSeparatorLine(line.text) || !state.previousEligible) return false;
  const previous = lines[index - 1];
  state.output.push({ text: "", ending: previous?.ending ?? line.ending });
  state.output.push(line);
  state.previousEligible = false;
  return true;
}

function handleOrdinaryLine(state: SeparatorScanState, line: MarkdownLine): void {
  state.output.push(line);
  const containerFence =
    isContainerFenceOpen(line.text) ??
    (state.container?.kind === "list" && isIndentedListContinuation(line.text)
      ? isFenceOpen(line.text.trimStart())
      : null);
  const fenceStart = containerFence
    ? { ...containerFence, owner: containerFence.owner ?? state.container?.kind ?? null }
    : isFenceOpen(line.text);
  const htmlStart = htmlBlockStart(line.text);
  if (isContainerBoundary(line.text, fenceStart, htmlStart, state.container)) {
    state.container = null;
  }

  if (fenceStart) {
    state.fence = fenceStart;
    state.container = fenceStart.owner ? { kind: fenceStart.owner, afterBlank: false } : null;
    state.previousEligible = false;
    return;
  }

  if (htmlStart) {
    state.htmlBlock = htmlBlockEnd(htmlStart, line.text) ? null : htmlStart;
    state.previousEligible = false;
    return;
  }

  const eligible = isEligibleProse(line.text, state.container);
  state.previousEligible = eligible;
  state.container = updateContainer(state.container, line.text, eligible);
}

/**
 * Inserts one blank line before a long prose line's hyphen separator. The
 * caller supplies source lines so existing line endings and fence repairs stay
 * byte-for-byte unchanged except for the inserted separator line.
 */
export function normalizeProseSeparators(lines: MarkdownLine[]): MarkdownLine[] {
  const frontMatterStart = firstNonBlankIndex(lines);
  const state: SeparatorScanState = {
    container: null,
    fence: null,
    frontMatter: frontMatterStart >= 0 && isFrontMatterStart(lines[frontMatterStart].text),
    frontMatterStart,
    htmlBlock: null,
    output: [],
    previousEligible: false,
  };

  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    if (!line) continue;

    if (handleFrontMatterLine(state, line, index)) continue;
    if (handleFenceLine(state, line)) continue;
    if (handleHtmlLine(state, line)) continue;
    if (handleSeparatorLine(state, lines, index)) continue;
    handleOrdinaryLine(state, line);
  }

  return state.output;
}
