import { useEffect, useRef, type RefObject } from "react";
import {
  findNearestSourceElement,
  readSourceLineRange,
  SOURCE_START_ATTR,
} from "@/lib/markdown/source-line-ranges";
import type { SourcePosition, SourcePositionProps } from "@/components/editors/source-position";
import { clampSourcePosition } from "./markdown-source-position";

type PreviewSourcePositionOptions = SourcePositionProps & {
  content: string;
  identity: string;
  rootRef: RefObject<HTMLDivElement | null>;
  scrollRef: RefObject<HTMLDivElement | null>;
};

function sourceBlocks(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(`[${SOURCE_START_ATTR}]`));
}

function nearestReadingLine(root: HTMLElement, scroll: HTMLElement): number {
  const selection = window.getSelection();
  if (selection && !selection.isCollapsed) {
    const selected = findNearestSourceElement(root, selection.focusNode);
    const range = selected && readSourceLineRange(selected);
    if (range) return range.startLine;
  }
  const viewport = scroll.getBoundingClientRect();
  const visible = sourceBlocks(root).filter((element) => {
    const rect = element.getBoundingClientRect();
    return rect.bottom > viewport.top && rect.top < viewport.bottom;
  });
  visible.sort(
    (a, b) =>
      Math.abs(a.getBoundingClientRect().top - viewport.top) -
      Math.abs(b.getBoundingClientRect().top - viewport.top),
  );
  return (visible[0] && readSourceLineRange(visible[0])?.startLine) ?? 1;
}

function nearestBlock(root: HTMLElement, line: number): HTMLElement | undefined {
  const distance = (element: HTMLElement) => {
    const range = readSourceLineRange(element)!;
    if (line >= range.startLine && line <= range.endLine)
      return (range.endLine - range.startLine) / 100000;
    return Math.min(Math.abs(line - range.startLine), Math.abs(line - range.endLine)) + 1;
  };
  return sourceBlocks(root).sort((a, b) => distance(a) - distance(b))[0];
}

export function useMarkdownPreviewSourcePosition({
  content,
  identity,
  rootRef,
  scrollRef,
  isActive = true,
  onSourcePositionHandle,
}: PreviewSourcePositionOptions) {
  const contentRef = useRef(content);
  contentRef.current = content;
  useEffect(() => {
    const root = rootRef.current;
    const scroll = scrollRef.current;
    if (!root || !scroll || !isActive || !onSourcePositionHandle) return;
    let live = true;
    let selectionIntent: SourcePosition["selection"];
    onSourcePositionHandle({
      capture: () =>
        live
          ? clampSourcePosition(contentRef.current, {
              selection: selectionIntent,
              nearestLine: nearestReadingLine(root, scroll),
            })
          : null,
      restore: (position) => {
        if (!live) return;
        const target = clampSourcePosition(contentRef.current, position);
        selectionIntent = target.selection;
        const block = nearestBlock(root, target.nearestLine);
        if (block)
          scroll.scrollTop +=
            block.getBoundingClientRect().top - scroll.getBoundingClientRect().top;
      },
    });
    return () => {
      live = false;
      onSourcePositionHandle(null);
    };
  }, [identity, rootRef, scrollRef, isActive, onSourcePositionHandle]);
}
