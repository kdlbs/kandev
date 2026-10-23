import { useMemo, type RefObject } from "react";
import {
  OffsetRange,
  Rect2D,
  type EditorModel,
  type EditorView,
  type BlockMeasurement,
  type SelectionRect,
} from "@vscode/markdown-editor";
import { sourceOffsetAtLine } from "@/components/task/markdown-source-position";
import type { MarkdownWalkthroughTarget } from "@/lib/markdown/walkthrough-range";
import {
  useRenderedWalkthroughRange,
  type RenderedWalkthroughAdapter,
} from "../use-rendered-walkthrough-range";

type Lifecycle = { active: boolean; model: EditorModel; view: EditorView };
type Options = {
  lifecycle: Lifecycle | null;
  rootRef: RefObject<HTMLDivElement | null>;
  target?: MarkdownWalkthroughTarget | null;
  content: string;
  isActive: boolean;
};

export function hybridWalkthroughOffsets(
  content: string,
  startLine: number,
  endLine: number,
): OffsetRange {
  const start = sourceOffsetAtLine(content, startLine);
  const end = sourceOffsetAtLine(content, endLine, Number.MAX_SAFE_INTEGER);
  return new OffsetRange(start, Math.max(start, end));
}

export function useHybridWalkthroughRange({
  lifecycle,
  rootRef,
  target,
  content,
  isActive,
}: Options): void {
  const adapter = useMemo<RenderedWalkthroughAdapter | null>(() => {
    const root = rootRef.current;
    if (!lifecycle?.active || !target || !root) return null;
    const { view } = lifecycle;
    const range = hybridWalkthroughOffsets(content, target.startLine, target.endLine);
    const rects = view.rangeRects(range);
    return {
      container: root,
      overlay: view.overlayContainer,
      reveal: () => view.revealRangeInCenterIfOutsideViewport(range, "instant"),
      measure: () => {
        if (!lifecycle.active) return null;
        const measured = hybridSectionRects(
          rects.get(),
          view.measuredLayout.measurements.get(),
          range,
        );
        if (!measured.length) return null;
        const top = Math.min(...measured.map((rect) => rect.y));
        const bottom = Math.max(...measured.map((rect) => rect.y + rect.height));
        const bounds = root.getBoundingClientRect();
        const local = view.coordinateSpace
          .capture()
          .toLocalPoint({ x: bounds.left + 8, y: bounds.top });
        const width = Math.max(0, bounds.width - 16);
        const rect = view.coordinateSpace
          .capture()
          .toClientRect(Rect2D.fromPointSize(local.x, top, width, bottom - top));
        return { box: { left: local.x, top, width, height: bottom - top }, rect };
      },
      observe: (update) => {
        const observer = {
          beginUpdate() {},
          endUpdate: update,
          handlePossibleChange: update,
          handleChange: update,
        };
        rects.addObserver(observer);
        return () => rects.removeObserver(observer);
      },
    };
  }, [lifecycle, rootRef, target, content]);
  useRenderedWalkthroughRange({
    target,
    active: isActive && !!lifecycle?.active,
    content,
    adapter,
  });
}

function hybridSectionRects(
  selection: readonly SelectionRect[],
  measurements: readonly BlockMeasurement[],
  range: OffsetRange,
): readonly SelectionRect[] {
  const blocks = measurements.filter(
    (block) => block.rect && block.rect.height > 0 && block.block.length > 0,
  );
  const selected = blocks.filter((block) => {
    const end = block.absoluteStart + block.block.length;
    return range.isEmpty
      ? block.absoluteStart <= range.start && range.start < end
      : block.absoluteStart < range.endExclusive && end > range.start;
  });
  const measured = selection.flatMap((rect) =>
    selected.flatMap((block) => {
      const top = Math.max(rect.y, block.rect!.top);
      const bottom = Math.min(rect.y + rect.height, block.rect!.bottom);
      return bottom > top ? [{ ...rect, y: top, height: bottom - top }] : [];
    }),
  );
  const tables = selected.filter((block) => block.block.kind === "table");
  if (measured.length) return [...measured, ...tables.map((block) => block.rect!)];
  const distance = (block: BlockMeasurement) =>
    Math.max(
      block.absoluteStart - range.endExclusive,
      range.start - (block.absoluteStart + block.block.length),
      0,
    );
  const nearest = selected[0] ?? blocks.sort((a, b) => distance(a) - distance(b))[0];
  return nearest?.rect ? [nearest.rect] : [];
}
