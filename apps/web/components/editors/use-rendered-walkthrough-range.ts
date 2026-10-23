import { useEffect, useId, useRef } from "react";
import type { MarkdownWalkthroughTarget } from "@/lib/markdown/walkthrough-range";
import { WALKTHROUGH_RANGE_CLASS } from "@/lib/markdown/walkthrough-range";
import { pendingWalkthroughAcknowledgement } from "@/hooks/file-editor-cursor";
import {
  clearWalkthroughEditorAnchor,
  isWalkthroughAnchorTargetVisible,
  setWalkthroughEditorAnchor,
  type WalkthroughViewportRect,
} from "@/lib/walkthrough-editor-anchor";

type Box = { left: number; top: number; width: number; height: number };
export type RenderedWalkthroughGeometry = { box: Box; rect: WalkthroughViewportRect };
export type RenderedWalkthroughAdapter = {
  container: HTMLElement;
  overlay: HTMLElement;
  measure: () => RenderedWalkthroughGeometry | null;
  canReveal?: () => boolean;
  reveal: () => { dispose: () => void } | undefined;
  observe?: (update: () => void) => () => void;
};

type Options = {
  target?: MarkdownWalkthroughTarget | null;
  active: boolean;
  content: string;
  adapter: RenderedWalkthroughAdapter | (() => RenderedWalkthroughAdapter | null) | null;
};

export function useRenderedWalkthroughRange({ target, active, content, adapter }: Options): void {
  const instance = useId();
  const revealed = useRef("");
  useEffect(() => {
    if (!target || !active || !adapter) {
      revealed.current = "";
      return;
    }
    const current = typeof adapter === "function" ? adapter() : adapter;
    if (!current) return;
    return mountRenderedRange(current, target, instance, revealed);
  }, [target, active, content, adapter, instance]);
}

// eslint-disable-next-line max-lines-per-function -- owns the overlay, layout observers, and pending reveal as one disposable unit.
function mountRenderedRange(
  adapter: RenderedWalkthroughAdapter,
  target: MarkdownWalkthroughTarget,
  instance: string,
  revealed: { current: string },
): () => void {
  const { container, overlay } = adapter;
  const key = `${target.key}:${instance}`;
  const indicator = document.createElement("div");
  indicator.className = WALKTHROUGH_RANGE_CLASS;
  indicator.setAttribute("aria-hidden", "true");
  indicator.dataset.testid = "walkthrough-editor-range";
  indicator.dataset.lineRange = `${target.startLine}-${target.endLine}`;
  indicator.hidden = true;
  overlay.append(indicator);
  let live = true;
  let frame: number | undefined;
  let reveal: { dispose: () => void } | undefined;
  const update = () => {
    frame = undefined;
    if (!live) return;
    const bounds = container.getBoundingClientRect();
    if (bounds.width <= 0 || bounds.height <= 0 || !container.isConnected) {
      indicator.hidden = true;
      clearWalkthroughEditorAnchor(key);
      return;
    }
    if (revealed.current !== key && (adapter.canReveal?.() ?? true)) {
      const acknowledge = pendingWalkthroughAcknowledgement(
        target.path,
        target.repositoryName,
        target.sessionId,
        target.navigationLine ?? target.startLine,
      );
      reveal = adapter.reveal();
      revealed.current = key;
      acknowledge();
    }
    const geometry = adapter.measure();
    indicator.hidden = !geometry;
    if (!geometry) {
      clearWalkthroughEditorAnchor(key);
      return;
    }
    Object.assign(indicator.style, {
      left: `${geometry.box.left}px`,
      top: `${geometry.box.top}px`,
      width: `${geometry.box.width}px`,
      height: `${geometry.box.height}px`,
    });
    if (!isWalkthroughAnchorTargetVisible(container, geometry.rect)) {
      clearWalkthroughEditorAnchor(key);
      return;
    }
    setWalkthroughEditorAnchor({
      key,
      taskId: target.taskId,
      stepIndex: target.stepIndex,
      file: target.file,
      repo: target.repo,
      line: target.startLine,
      lineEnd: target.endLine,
      rect: geometry.rect,
      container,
    });
  };
  const schedule = () => {
    if (!live || frame !== undefined) return;
    frame = requestAnimationFrame(update);
  };
  const observer = new ResizeObserver(schedule);
  observer.observe(container);
  if (overlay !== container) observer.observe(overlay);
  const unobserve = adapter.observe?.(schedule);
  container.addEventListener("scroll", schedule, true);
  container.addEventListener("load", schedule, true);
  window.addEventListener("resize", schedule);
  const interval = window.setInterval(schedule, 150);
  schedule();
  return () => {
    live = false;
    if (frame !== undefined) cancelAnimationFrame(frame);
    reveal?.dispose();
    unobserve?.();
    observer.disconnect();
    window.clearInterval(interval);
    container.removeEventListener("scroll", schedule, true);
    container.removeEventListener("load", schedule, true);
    window.removeEventListener("resize", schedule);
    indicator.remove();
    clearWalkthroughEditorAnchor(key);
  };
}
