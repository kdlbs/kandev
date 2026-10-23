import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import type { EditorModel, EditorView } from "@vscode/markdown-editor";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { EditorCommentPopover } from "@/components/task/editor-comment-popover";
import type { MarkdownCommentSubmission } from "./hybrid-markdown-editor";

type Disposable = { dispose: () => void };
export type CommentSelectionLifecycle = {
  active: boolean;
  model: EditorModel;
  view: EditorView;
  selectionSubscription?: Disposable;
};
type CommentSelection = {
  owner: CommentSelectionLifecycle;
  source: string;
  revision: number;
  start: number;
  endExclusive: number;
  selectedText: string;
  lineRange: { start: number; end: number };
  position: { x: number; y: number };
};
type SelectionObserver = {
  publish: (selection: CommentSelection | null) => void;
  invalidate: () => void;
  revision: { current: number };
};

function observeSelection(
  lifecycle: CommentSelectionLifecycle,
  state: SelectionObserver,
): Disposable {
  let active = true;
  let queued = false;
  const update = () => {
    queued = false;
    if (!active || !lifecycle.active) return;
    const { model, view } = lifecycle;
    const source = model.sourceText.get().value;
    const selection = model.selection.get();
    const rect = view.caretRect.get();
    if (
      !selection ||
      selection.isCollapsed ||
      !rect ||
      model.readonlyMode.get() ||
      model.isSelecting.get() ||
      model.selectionSource.get() !== "user"
    ) {
      state.publish(null);
      return;
    }
    const { start, endExclusive } = selection.range;
    const clientRect = view.coordinateSpace.capture().toClientRect(rect);
    state.publish({
      owner: lifecycle,
      source,
      revision: state.revision.current,
      start,
      endExclusive,
      selectedText: source.slice(start, endExclusive),
      lineRange: {
        start: source.slice(0, start).split("\n").length,
        end: source.slice(0, Math.max(start, endExclusive - 1)).split("\n").length,
      },
      position: { x: clientRect.left, y: clientRect.bottom + 8 },
    });
  };
  const schedule = () => {
    if (queued || !active) return;
    queued = true;
    queueMicrotask(update);
  };
  const observer = {
    beginUpdate() {},
    endUpdate: schedule,
    handlePossibleChange: schedule,
    handleChange(observable: unknown) {
      if (!active) return;
      if (observable === lifecycle.model.sourceText || observable === lifecycle.model.readonlyMode)
        state.invalidate();
      schedule();
    },
  };
  const observables = [
    lifecycle.model.selection,
    lifecycle.model.selectionSource,
    lifecycle.model.isSelecting,
    lifecycle.model.sourceText,
    lifecycle.model.readonlyMode,
    lifecycle.view.caretRect,
  ];
  for (const observable of observables) observable.addObserver(observer);
  update();
  return {
    dispose: () => {
      if (!active) return;
      active = false;
      for (const observable of observables) observable.removeObserver(observer);
    },
  };
}

function isCurrentSelection(
  selection: CommentSelection | null,
  lifecycle: CommentSelectionLifecycle | null,
  enabled: boolean,
  revision: number,
): selection is CommentSelection {
  return (
    !!selection &&
    enabled &&
    !!lifecycle?.active &&
    selection.owner === lifecycle &&
    !lifecycle.model.readonlyMode.get() &&
    selection.revision === revision &&
    lifecycle.model.sourceText.get().value === selection.source
  );
}

function useTriggerPosition(
  selection: CommentSelection | null,
  triggerRef: RefObject<HTMLButtonElement | null>,
) {
  const [position, setPosition] = useState({ left: 0, top: 0 });
  useLayoutEffect(() => {
    const trigger = triggerRef.current;
    if (!selection || !trigger) return;
    const viewport = window.visualViewport;
    const update = () => {
      const bounds = trigger.getBoundingClientRect();
      const left = (viewport?.offsetLeft ?? 0) + 8;
      const top = (viewport?.offsetTop ?? 0) + 8;
      const right = left + (viewport?.width ?? window.innerWidth) - 16;
      const bottom = top + (viewport?.height ?? window.innerHeight) - 16;
      const view = selection.owner.view;
      const rect = view.caretRect.get();
      if (!selection.owner.active || !rect) return;
      const caret = view.coordinateSpace.capture().toClientRect(rect);
      const x = Math.max(left, Math.min(caret.left, right - bounds.width));
      const preferredTop = caret.bottom + 8;
      const y =
        preferredTop + bounds.height <= bottom ? preferredTop : caret.top - bounds.height - 8;
      const next = { left: x, top: Math.max(top, Math.min(y, bottom - bounds.height)) };
      setPosition((current) =>
        current.left === next.left && current.top === next.top ? current : next,
      );
    };
    update();
    window.addEventListener("resize", update);
    window.addEventListener("scroll", update, true);
    viewport?.addEventListener("resize", update);
    viewport?.addEventListener("scroll", update);
    const observer = new ResizeObserver(update);
    observer.observe(trigger);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", update);
      window.removeEventListener("scroll", update, true);
      viewport?.removeEventListener("resize", update);
      viewport?.removeEventListener("scroll", update);
    };
  }, [selection, triggerRef]);
  return position;
}

export function HybridMarkdownComments({
  lifecycle,
  enabled,
  onComment,
}: {
  lifecycle: CommentSelectionLifecycle | null;
  enabled: boolean;
  onComment?: (comment: MarkdownCommentSubmission) => void;
}) {
  const { t } = useTranslation();
  const revision = useRef(0);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [selection, setSelection] = useState<CommentSelection | null>(null);
  const [captured, setCaptured] = useState<CommentSelection | null>(null);
  useEffect(() => {
    const invalidate = () => {
      revision.current += 1;
      setSelection(null);
      setCaptured(null);
    };
    invalidate();
    if (!enabled || !lifecycle?.active) return;
    const subscription = observeSelection(lifecycle, {
      publish: setSelection,
      invalidate,
      revision,
    });
    lifecycle.selectionSubscription = subscription;
    return () => subscription.dispose();
  }, [enabled, lifecycle]);
  const validCapture = isCurrentSelection(captured, lifecycle, enabled, revision.current)
    ? captured
    : null;
  const validSelection = isCurrentSelection(selection, lifecycle, enabled, revision.current)
    ? selection
    : null;
  const position = useTriggerPosition(validCapture ? null : validSelection, triggerRef);
  if (validCapture)
    return (
      <EditorCommentPopover
        selectedText={validCapture.selectedText}
        lineRange={validCapture.lineRange}
        position={validCapture.position}
        onClose={() => setCaptured(null)}
        onSubmit={(text) => {
          if (!onComment || !isCurrentSelection(validCapture, lifecycle, enabled, revision.current))
            return;
          onComment({ text, start: validCapture.start, endExclusive: validCapture.endExclusive });
          setCaptured(null);
          setSelection(null);
          if (lifecycle?.active) lifecycle.view.focus();
        }}
      />
    );
  if (!validSelection) return null;
  return (
    <Button
      ref={triggerRef}
      type="button"
      variant="secondary"
      size="sm"
      className="fixed z-40 h-7 max-w-[calc(100vw-1rem)] px-2 text-xs max-md:h-11 max-md:min-w-11 pointer-coarse:h-11 pointer-coarse:min-w-11"
      style={position}
      data-testid="hybrid-markdown-comment-trigger"
      onMouseDown={(event) => event.preventDefault()}
      onClick={() => {
        if (isCurrentSelection(validSelection, lifecycle, enabled, revision.current))
          setCaptured({ ...validSelection, position: { x: position.left, y: position.top } });
      }}
    >
      {t("task:comment")}
    </Button>
  );
}
