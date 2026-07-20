import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type RefObject,
} from "react";
import type { editor as monacoEditor } from "monaco-editor";
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

export { getWalkthroughEditorRange } from "@/lib/walkthrough-editor-range";

type UseMonacoWalkthroughRangeOpts = {
  editor: monacoEditor.IStandaloneCodeEditor | null;
  editorAreaRef: RefObject<HTMLDivElement | null>;
  path: string;
  repo?: string;
};

export function buildWalkthroughRangeDecorations(
  range: WalkthroughEditorRange | null,
): monacoEditor.IModelDeltaDecoration[] {
  if (!range) return [];
  const decorations: monacoEditor.IModelDeltaDecoration[] = [];
  for (let line = range.startLine; line <= range.endLine; line++) {
    decorations.push({
      range: { startLineNumber: line, startColumn: 1, endLineNumber: line, endColumn: 1 },
      options: {
        isWholeLine: true,
        className: "monaco-walkthrough-line",
        lineNumberClassName: "monaco-walkthrough-line-number",
        linesDecorationsClassName:
          line === range.startLine ? "monaco-walkthrough-range-start" : "monaco-walkthrough-range",
      },
    });
  }
  return decorations;
}

export function clampWalkthroughRangeToLineCount(
  range: WalkthroughEditorRange,
  lineCount: number | null | undefined,
): WalkthroughEditorRange {
  if (!lineCount || lineCount < 1) return range;
  const startLine = Math.min(Math.max(range.startLine, 1), lineCount);
  const endLine = Math.min(Math.max(range.endLine, startLine), lineCount);
  return { startLine, endLine };
}

function rectFromViewport(left: number, top: number, width: number, height: number) {
  return { left, top, width, height, right: left + width, bottom: top + height };
}

function measureRangeBox(
  editor: monacoEditor.IStandaloneCodeEditor,
  area: HTMLElement,
  range: WalkthroughEditorRange,
): { box: WalkthroughRangeBox; viewportRect: WalkthroughViewportRect } | null {
  const editorDom = editor.getDomNode();
  if (!editorDom) return null;
  const start = editor.getScrolledVisiblePosition({ lineNumber: range.startLine, column: 1 });
  if (!start) return null;
  const end = editor.getScrolledVisiblePosition({ lineNumber: range.endLine, column: 1 }) ?? start;
  const editorRect = editorDom.getBoundingClientRect();
  const areaRect = area.getBoundingClientRect();
  const lineHeight = start.height || end.height || 18;
  const top = editorRect.top - areaRect.top + start.top;
  const height = Math.max(lineHeight, end.top - start.top + (end.height || lineHeight));
  const left = Math.max(48, start.left - 8);
  const width = Math.max(160, editorRect.width - left - 16);
  return {
    box: { ...range, top, left, width, height },
    viewportRect: rectFromViewport(
      editorRect.left + left,
      editorRect.top + start.top,
      width,
      height,
    ),
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

function useMonacoModelSnapshot(editor: monacoEditor.IStandaloneCodeEditor | null): string {
  const subscribe = useCallback(
    (onStoreChange: () => void) => {
      if (!editor) return () => {};
      const modelSubscription = editor.onDidChangeModel(onStoreChange);
      const contentSubscription = editor.onDidChangeModelContent(onStoreChange);
      return () => {
        modelSubscription.dispose();
        contentSubscription.dispose();
      };
    },
    [editor],
  );
  const getSnapshot = useCallback(() => {
    const model = editor?.getModel();
    return model ? `${model.id}:${model.getLineCount()}` : "";
  }, [editor]);
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

function useMonacoWalkthroughDecorations(
  editor: monacoEditor.IStandaloneCodeEditor | null,
  range: WalkthroughEditorRange | null,
  anchorKey: string,
  modelSnapshot: string,
) {
  const decorationsRef = useRef<monacoEditor.IEditorDecorationsCollection | null>(null);
  const revealedRef = useRef<{
    editor: monacoEditor.IStandaloneCodeEditor;
    model: monacoEditor.ITextModel | null;
    anchorKey: string;
    startLine: number;
    endLine: number;
  } | null>(null);

  useEffect(() => {
    if (!editor) return;
    decorationsRef.current = editor.createDecorationsCollection([]);
    return () => {
      decorationsRef.current?.set([]);
      decorationsRef.current = null;
    };
  }, [editor]);

  useEffect(() => {
    decorationsRef.current?.set(buildWalkthroughRangeDecorations(range));
    if (!editor || !range) {
      revealedRef.current = null;
      return;
    }

    const next = { editor, model: editor.getModel(), anchorKey, ...range };
    const previous = revealedRef.current;
    if (
      previous?.editor === next.editor &&
      previous.model === next.model &&
      previous.anchorKey === next.anchorKey &&
      previous.startLine === next.startLine &&
      previous.endLine === next.endLine
    ) {
      return;
    }
    revealedRef.current = next;
    editor.revealLinesInCenter(range.startLine, range.endLine);
  }, [anchorKey, editor, modelSnapshot, range]);
}

export function useMonacoWalkthroughRange({
  editor,
  editorAreaRef,
  path,
  repo,
}: UseMonacoWalkthroughRangeOpts): WalkthroughRangeBox | null {
  const [box, setBox] = useState<WalkthroughRangeBox | null>(null);
  const { taskId, stepIndex, step } = useActiveWalkthroughStep();
  const modelSnapshot = useMonacoModelSnapshot(editor);
  const range = useMemo(
    () => getWalkthroughEditorRange({ path, repository_name: repo }, step),
    [path, repo, step],
  );
  const clampedRange = useMemo(
    () =>
      range ? clampWalkthroughRangeToLineCount(range, editor?.getModel()?.getLineCount()) : null,
    [editor, modelSnapshot, range],
  );
  const anchorKey = taskId ? `${taskId}:${stepIndex}:${repo ?? ""}:${path}` : "";
  useMonacoWalkthroughDecorations(editor, clampedRange, anchorKey, modelSnapshot);

  useEffect(() => {
    const area = editorAreaRef.current;
    if (!editor || !area || !clampedRange || !taskId || !step) {
      setBox(null);
      if (anchorKey) clearWalkthroughEditorAnchor(anchorKey);
      return;
    }

    let frame: number | null = null;
    const update = () => {
      if (frame !== null) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const measured = measureRangeBox(editor, area, clampedRange);
        if (!measured) {
          setBox(null);
          clearWalkthroughEditorAnchor(anchorKey);
          return;
        }
        const editorDom = editor.getDomNode();
        if (!isWalkthroughAnchorTargetVisible(editorDom, measured.viewportRect)) {
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
          line: clampedRange.startLine,
          lineEnd: clampedRange.endLine,
          rect: measured.viewportRect,
          container: editorDom ?? undefined,
        });
      });
    };

    update();
    const scrollSub = editor.onDidScrollChange(update);
    const layoutSub = editor.onDidLayoutChange(update);
    const resizeObserver = new ResizeObserver(update);
    resizeObserver.observe(area);
    const visibilityInterval = window.setInterval(update, 150);
    window.addEventListener("resize", update);
    return () => {
      if (frame !== null) cancelAnimationFrame(frame);
      scrollSub.dispose();
      layoutSub.dispose();
      resizeObserver.disconnect();
      window.clearInterval(visibilityInterval);
      window.removeEventListener("resize", update);
      clearWalkthroughEditorAnchor(anchorKey);
    };
  }, [anchorKey, clampedRange, editor, editorAreaRef, path, step, stepIndex, taskId]);

  return box;
}
