import { useEffect, useRef, type RefObject } from "react";
import { OffsetRange, Selection, type EditorModel, type EditorView } from "@vscode/markdown-editor";
import {
  clampSourcePosition,
  sourceLineAtOffset,
  sourceOffsetAtLine,
} from "@/components/task/markdown-source-position";
import type { SourcePosition, SourcePositionHandle, SourcePositionProps } from "../source-position";

export type HybridSourcePositionLifecycle = {
  active: boolean;
  model: EditorModel;
  view: EditorView;
  sourcePositionReveal?: { dispose: () => void };
};

export function cancelHybridSourcePositionReveal(
  lifecycle: HybridSourcePositionLifecycle | null,
): void {
  if (!lifecycle) return;
  const pending = lifecycle.sourcePositionReveal;
  lifecycle.sourcePositionReveal = undefined;
  pending?.dispose();
}

function nearestVisibleOffset(lifecycle: HybridSourcePositionLifecycle, root: HTMLElement): number {
  const map = lifecycle.view.measuredLayout.visualLineMap.get();
  if (map.isEmpty) return lifecycle.model.selection.get()?.active ?? 0;
  const rect = root.getBoundingClientRect();
  const point = lifecycle.view.coordinateSpace
    .capture()
    .toLocalPoint({ x: rect.left, y: rect.top + 1 });
  return map.offsetAtPoint(point);
}

function createHandle(
  lifecycle: HybridSourcePositionLifecycle,
  root: HTMLElement,
  isLive: () => boolean,
): SourcePositionHandle {
  let pendingPosition: SourcePosition | undefined;
  let pendingSource: string | undefined;
  return {
    capture: () => {
      if (!isLive()) return null;
      const source = lifecycle.model.sourceText.get().value;
      const selection = lifecycle.model.selection.get();
      if (
        pendingPosition &&
        pendingSource === source &&
        pendingPosition.selection?.anchor === selection?.anchor &&
        pendingPosition.selection?.active === selection?.active
      )
        return pendingPosition;
      const nearest =
        selection && !selection.isCollapsed
          ? selection.active
          : nearestVisibleOffset(lifecycle, root);
      return clampSourcePosition(source, {
        selection: selection ? { anchor: selection.anchor, active: selection.active } : undefined,
        nearestLine: sourceLineAtOffset(source, nearest),
      });
    },
    restore: (position) => {
      if (!isLive()) return;
      cancelHybridSourcePositionReveal(lifecycle);
      const source = lifecycle.model.sourceText.get().value;
      const target = clampSourcePosition(source, position);
      const anchor = target.selection?.anchor ?? sourceOffsetAtLine(source, target.nearestLine);
      const active = target.selection?.active ?? anchor;
      lifecycle.model.selection.set(new Selection(anchor, active), undefined, undefined);
      const start =
        anchor === active
          ? sourceOffsetAtLine(source, target.nearestLine)
          : Math.min(anchor, active);
      const end = anchor === active ? start : Math.max(anchor, active);
      lifecycle.view.focus();
      const reveal = lifecycle.view.revealRangeInCenterIfOutsideViewport(
        new OffsetRange(start, end),
        "instant",
      );
      const requestedPosition = { ...target, selection: { anchor, active } };
      pendingPosition = requestedPosition;
      pendingSource = source;
      const clearPending = () => {
        if (pendingPosition === requestedPosition) {
          pendingPosition = undefined;
          pendingSource = undefined;
        }
        root.removeEventListener("scroll", onScroll);
      };
      const onScroll = () => {
        cancelAnimationFrame(frame);
        reveal.dispose();
        clearPending();
      };
      root.addEventListener("scroll", onScroll, { once: true });
      // The engine registers its reveal frame before this completion frame.
      const frame = requestAnimationFrame(clearPending);
      lifecycle.sourcePositionReveal = {
        dispose: () => {
          cancelAnimationFrame(frame);
          clearPending();
          reveal.dispose();
        },
      };
    },
  };
}

export function useHybridSourcePosition({
  lifecycle,
  rootRef,
  content,
  isActive = true,
  onSourcePositionHandle,
  onError,
}: SourcePositionProps & {
  lifecycle: HybridSourcePositionLifecycle | null;
  rootRef: RefObject<HTMLDivElement | null>;
  content: string;
  onError: (error: unknown) => void;
}) {
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;
  useEffect(() => () => cancelHybridSourcePositionReveal(lifecycle), [content, lifecycle]);
  useEffect(() => {
    const root = rootRef.current;
    if (!lifecycle?.active || !root || !isActive || !onSourcePositionHandle) return;
    let live = true;
    const handle = createHandle(lifecycle, root, () => live && lifecycle.active);
    onSourcePositionHandle({
      capture: handle.capture,
      restore: (position) => {
        try {
          handle.restore(position);
        } catch (error) {
          cancelHybridSourcePositionReveal(lifecycle);
          onErrorRef.current(error);
        }
      },
    });
    return () => {
      live = false;
      cancelHybridSourcePositionReveal(lifecycle);
      onSourcePositionHandle(null);
    };
  }, [lifecycle, rootRef, isActive, onSourcePositionHandle]);
}
