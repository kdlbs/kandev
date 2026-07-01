import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { EditorView } from "@codemirror/view";
import { useAppStore } from "@/components/state-provider";
import { useTaskWalkthrough } from "@/hooks/domains/session/use-task-walkthrough";
import {
  clearWalkthroughEditorAnchor,
  isWalkthroughAnchorTargetVisible,
  setWalkthroughEditorAnchor,
  type WalkthroughViewportRect,
} from "@/lib/walkthrough-editor-anchor";
import {
  getWalkthroughEditorRange,
  type WalkthroughEditorRange,
  type WalkthroughRangeBox,
} from "@/lib/walkthrough-editor-range";
import { useIsWalkthroughOpenForTask } from "@/lib/walkthrough-open-state";

type UseCodeMirrorWalkthroughRangeOpts = {
  view: EditorView | null;
  editorAreaRef: RefObject<HTMLElement | null>;
  path: string;
  repo?: string;
};

function rectFromViewport(left: number, top: number, width: number, height: number) {
  return { left, top, width, height, right: left + width, bottom: top + height };
}

function clampDocLine(view: EditorView, line: number): number {
  return Math.min(Math.max(line, 1), view.state.doc.lines);
}

function linePosition(view: EditorView, line: number) {
  return view.state.doc.line(clampDocLine(view, line));
}

function measureRangeBox(
  view: EditorView,
  area: HTMLElement,
  range: WalkthroughEditorRange,
): { box: WalkthroughRangeBox; viewportRect: WalkthroughViewportRect } | null {
  const startLine = linePosition(view, range.startLine);
  const endLine = linePosition(view, range.endLine);
  const start = view.coordsAtPos(startLine.from);
  const end = view.coordsAtPos(endLine.to) ?? start;
  if (!start || !end) return null;

  const areaRect = area.getBoundingClientRect();
  const scrollerRect = view.scrollDOM.getBoundingClientRect();
  const left = Math.max(38, scrollerRect.left - areaRect.left + 36);
  const top = start.top - areaRect.top;
  const height = Math.max(18, end.bottom - start.top);
  const width = Math.max(120, areaRect.right - (areaRect.left + left) - 12);

  return {
    box: { ...range, top, left, width, height },
    viewportRect: rectFromViewport(areaRect.left + left, start.top, width, height),
  };
}

function useActiveWalkthroughStep() {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const isOpen = useIsWalkthroughOpenForTask(taskId);
  const stepIndex = useAppStore((s) =>
    taskId ? (s.walkthroughs.activeStepByTaskId[taskId] ?? 0) : 0,
  );
  const walkthrough = useTaskWalkthrough(taskId).data ?? null;
  const step = walkthrough?.steps[stepIndex] ?? null;
  return { taskId, stepIndex, step: isOpen ? step : null };
}

export function useCodeMirrorWalkthroughRange({
  view,
  editorAreaRef,
  path,
  repo,
}: UseCodeMirrorWalkthroughRangeOpts): WalkthroughRangeBox | null {
  const [box, setBox] = useState<WalkthroughRangeBox | null>(null);
  const { taskId, stepIndex, step } = useActiveWalkthroughStep();
  const range = useMemo(
    () => getWalkthroughEditorRange({ path, repository_name: repo }, step),
    [path, repo, step],
  );
  const anchorKey = taskId ? `${taskId}:${stepIndex}:${repo ?? ""}:${path}:cm` : "";
  const didRevealRef = useRef("");

  useEffect(() => {
    const area = editorAreaRef.current;
    if (!view || !area || !range || !taskId || !step) {
      didRevealRef.current = "";
      setBox(null);
      if (anchorKey) clearWalkthroughEditorAnchor(anchorKey);
      return;
    }

    const revealKey = `${anchorKey}:${range.startLine}-${range.endLine}`;
    if (didRevealRef.current !== revealKey) {
      didRevealRef.current = revealKey;
      const startLine = linePosition(view, range.startLine);
      view.dispatch({
        effects: EditorView.scrollIntoView(startLine.from, { y: "center" }),
      });
    }

    let frame: number | null = null;
    const update = () => {
      if (frame !== null) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const measured = measureRangeBox(view, area, range);
        if (!measured) {
          setBox(null);
          clearWalkthroughEditorAnchor(anchorKey);
          return;
        }
        if (!isWalkthroughAnchorTargetVisible(view.dom, measured.viewportRect)) {
          setBox(null);
          clearWalkthroughEditorAnchor(anchorKey);
          return;
        }
        setBox(measured.box);
        setWalkthroughEditorAnchor({
          key: anchorKey,
          taskId,
          stepIndex,
          file: step.file,
          repo: step.repo,
          line: range.startLine,
          lineEnd: range.endLine,
          rect: measured.viewportRect,
          container: view.dom,
        });
      });
    };

    update();
    const resizeObserver = new ResizeObserver(update);
    resizeObserver.observe(area);
    const visibilityInterval = window.setInterval(update, 150);
    view.scrollDOM.addEventListener("scroll", update);
    window.addEventListener("resize", update);
    return () => {
      if (frame !== null) cancelAnimationFrame(frame);
      resizeObserver.disconnect();
      window.clearInterval(visibilityInterval);
      view.scrollDOM.removeEventListener("scroll", update);
      window.removeEventListener("resize", update);
      clearWalkthroughEditorAnchor(anchorKey);
    };
  }, [anchorKey, editorAreaRef, path, range, step, stepIndex, taskId, view]);

  return box;
}
