import { useMemo, type RefObject } from "react";
import type { WalkthroughEditorRange } from "@/lib/walkthrough-editor-range";
import type { MarkdownWalkthroughTarget } from "@/lib/markdown/walkthrough-range";
import { readSourceLineRange, SOURCE_START_ATTR } from "@/lib/markdown/source-line-ranges";
import {
  useRenderedWalkthroughRange,
  type RenderedWalkthroughAdapter,
} from "@/components/editors/use-rendered-walkthrough-range";

export function markdownPreviewRangeElements(
  root: HTMLElement,
  range: WalkthroughEditorRange,
): HTMLElement[] {
  const blocks = Array.from(root.querySelectorAll<HTMLElement>(`[${SOURCE_START_ATTR}]`)).filter(
    (element) => readSourceLineRange(element),
  );
  const leaves = blocks.filter(
    (element) =>
      hasOwnMappedText(element) ||
      !blocks.some((child) => child !== element && element.contains(child)),
  );
  const matching = leaves.filter((element) => {
    const source = readSourceLineRange(element)!;
    return source.startLine <= range.endLine && source.endLine >= range.startLine;
  });
  if (matching.length)
    return matching.filter((element) => {
      const own = readSourceLineRange(element)!;
      let coveredUntil = Math.max(own.startLine, range.startLine) - 1;
      const end = Math.min(own.endLine, range.endLine);
      const children = matching
        .filter((child) => child !== element && element.contains(child))
        .map((child) => readSourceLineRange(child)!)
        .sort((a, b) => a.startLine - b.startLine);
      for (const child of children) {
        if (child.startLine > coveredUntil + 1) break;
        coveredUntil = Math.max(coveredUntil, child.endLine);
      }
      return coveredUntil < end;
    });
  const distance = (element: HTMLElement) => {
    const source = readSourceLineRange(element)!;
    return Math.min(
      Math.abs(source.startLine - range.startLine),
      Math.abs(source.endLine - range.startLine),
    );
  };
  return leaves.sort((a, b) => distance(a) - distance(b)).slice(0, 1);
}

function hasOwnMappedText(element: Element): boolean {
  return Array.from(element.childNodes).some((node) => {
    if (node.nodeType === Node.TEXT_NODE) return !!node.textContent?.trim();
    return (
      node instanceof Element && !node.hasAttribute(SOURCE_START_ATTR) && hasOwnMappedText(node)
    );
  });
}

type Options = {
  target?: MarkdownWalkthroughTarget | null;
  isActive?: boolean;
  content: string;
  rootRef: RefObject<HTMLDivElement | null>;
  scrollRef: RefObject<HTMLDivElement | null>;
};

export function useMarkdownPreviewWalkthroughRange({
  target,
  isActive = true,
  content,
  rootRef,
  scrollRef,
}: Options): void {
  const adapter = useMemo(
    () => (): RenderedWalkthroughAdapter | null => {
      const root = rootRef.current;
      const scroll = scrollRef.current;
      if (!target || !root || !scroll) return null;
      const measure = () => {
        const rects = markdownPreviewRangeElements(root, target)
          .map((element) => element.getBoundingClientRect())
          .filter((rect) => rect.height > 0 && rect.width > 0);
        if (!rects.length) return null;
        const viewport = scroll.getBoundingClientRect();
        const left = viewport.left + 8;
        const top = Math.min(...rects.map((rect) => rect.top));
        const bottom = Math.max(...rects.map((rect) => rect.bottom));
        const width = Math.max(0, viewport.width - 16);
        const rect = { left, top, bottom, right: left + width, width, height: bottom - top };
        return {
          rect,
          box: { left: 8, top: top - viewport.top + scroll.scrollTop, width, height: rect.height },
        };
      };
      return {
        container: scroll,
        overlay: scroll,
        measure,
        canReveal: () => !!measure(),
        reveal: () => {
          const geometry = measure();
          const viewport = scroll.getBoundingClientRect();
          if (
            geometry &&
            (geometry.rect.top < viewport.top || geometry.rect.bottom > viewport.bottom)
          )
            scroll.scrollTop +=
              geometry.rect.top -
              viewport.top -
              Math.max(0, (viewport.height - geometry.rect.height) / 2);
          return undefined;
        },
        observe: (update) => {
          const observer = new ResizeObserver(update);
          observer.observe(root);
          return () => observer.disconnect();
        },
      };
    },
    [rootRef, scrollRef, target, content],
  );
  useRenderedWalkthroughRange({ target, active: isActive, content, adapter });
}
