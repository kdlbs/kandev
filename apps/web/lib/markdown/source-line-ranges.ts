export type SourceLineRange = {
  startLine: number;
  endLine: number;
};

export type MarkdownPreviewSelection = SourceLineRange & {
  selectedText: string;
  position: { x: number; y: number };
};

export const SOURCE_START_ATTR = "data-md-source-start";
export const SOURCE_END_ATTR = "data-md-source-end";

function readPositiveInt(value: string | null): number | null {
  if (!value) return null;
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

export function readSourceLineRange(element: Element): SourceLineRange | null {
  const start = readPositiveInt(element.getAttribute(SOURCE_START_ATTR));
  const end = readPositiveInt(element.getAttribute(SOURCE_END_ATTR)) ?? start;
  if (!start || !end) return null;
  return {
    startLine: Math.min(start, end),
    endLine: Math.max(start, end),
  };
}

export function findNearestSourceElement(root: HTMLElement, node: Node | null): HTMLElement | null {
  if (!node || !root.contains(node)) return null;
  let element =
    node.nodeType === Node.ELEMENT_NODE
      ? (node as HTMLElement)
      : ((node.parentElement ?? null) as HTMLElement | null);

  while (element && root.contains(element)) {
    if (readSourceLineRange(element)) return element;
    if (element === root) break;
    element = element.parentElement;
  }
  return null;
}

function normalizeSearchText(text: string): string {
  return text.replace(/\s+/g, " ").trim().toLowerCase();
}

function normalizeMarkdownSourceWindow(text: string): string {
  return normalizeSearchText(
    text
      .replace(/^#{1,6}\s+/gm, "")
      .replace(/^\s*[-*+]\s+/gm, "")
      .replace(/^\s*\d+[.)]\s+/gm, "")
      .replace(/^>\s?/gm, "")
      .replace(/[`*_~[\]()]/g, ""),
  );
}

export function findLineRangeForSelectedText(
  content: string,
  selectedText: string,
): SourceLineRange | null {
  const needle = normalizeSearchText(selectedText);
  if (!needle) return null;

  const lines = content.replace(/\r\n?/g, "\n").split("\n");
  for (let span = 1; span <= lines.length; span++) {
    for (let start = 0; start + span <= lines.length; start++) {
      const end = start + span - 1;
      const haystack = normalizeMarkdownSourceWindow(lines.slice(start, end + 1).join(" "));
      if (!haystack) continue;
      if (haystack === needle || haystack.includes(needle)) {
        return { startLine: start + 1, endLine: end + 1 };
      }
    }
  }
  return null;
}

function selectionTouchesRoot(root: HTMLElement, range: Range): boolean {
  return (
    root.contains(range.commonAncestorContainer) ||
    root.contains(range.startContainer) ||
    root.contains(range.endContainer)
  );
}

function selectionPosition(
  range: Range,
  fallbackElement: HTMLElement | null,
): { x: number; y: number } {
  const rect = range.getBoundingClientRect?.();
  if (rect && (rect.left || rect.top || rect.right || rect.bottom)) {
    return { x: rect.right, y: rect.bottom };
  }
  const fallbackRect = fallbackElement?.getBoundingClientRect();
  if (fallbackRect) return { x: fallbackRect.right, y: fallbackRect.bottom };
  return { x: 0, y: 0 };
}

function mergeSourceRanges(
  startRange: SourceLineRange | null,
  endRange: SourceLineRange | null,
): SourceLineRange | null {
  if (!startRange && !endRange) return null;
  const startLine = Math.min(
    startRange?.startLine ?? endRange!.startLine,
    endRange?.startLine ?? startRange!.startLine,
  );
  const endLine = Math.max(
    startRange?.endLine ?? endRange!.endLine,
    endRange?.endLine ?? startRange!.endLine,
  );
  return { startLine, endLine };
}

function sourceRangeForSelection(
  root: HTMLElement,
  content: string,
  selectedText: string,
  range: Range,
): {
  lineRange: SourceLineRange | null;
  fallbackElement: HTMLElement | null;
} {
  const startElement = findNearestSourceElement(root, range.startContainer);
  const endElement = findNearestSourceElement(root, range.endContainer);
  const startRange = startElement ? readSourceLineRange(startElement) : null;
  const endRange = endElement ? readSourceLineRange(endElement) : null;
  return {
    lineRange:
      mergeSourceRanges(startRange, endRange) ??
      findLineRangeForSelectedText(content, selectedText),
    fallbackElement: endElement ?? startElement,
  };
}

export function resolveMarkdownDomSelection(
  root: HTMLElement,
  content: string,
  selection: Selection,
): MarkdownPreviewSelection | null {
  if (selection.rangeCount === 0) return null;
  const selectedText = selection.toString().trim();
  if (!selectedText) return null;

  const range = selection.getRangeAt(0);
  if (!selectionTouchesRoot(root, range)) return null;

  const { lineRange, fallbackElement } = sourceRangeForSelection(
    root,
    content,
    selectedText,
    range,
  );
  if (!lineRange) return null;

  return {
    ...lineRange,
    selectedText,
    position: selectionPosition(range, fallbackElement),
  };
}
