import { useEffect, useRef, type Dispatch, type RefObject, type SetStateAction } from "react";
import type { EditorView } from "@codemirror/view";
import type { ReactCodeMirrorRef } from "@uiw/react-codemirror";

export type CodeMirrorCurrentSelection = {
  text: string;
  startLine: number;
  endLine: number;
} | null;

type FloatingButtonPosition = { x: number; y: number } | null;

export function useCodeMirrorTouchSelection({
  enabled,
  wrapperRef,
  editorRef,
  readCurrentSelection,
  setCurrentSelection,
  setFloatingButtonPos,
}: {
  enabled: boolean;
  wrapperRef: RefObject<HTMLDivElement | null>;
  editorRef: RefObject<ReactCodeMirrorRef | null>;
  readCurrentSelection: (view: EditorView) => CodeMirrorCurrentSelection;
  setCurrentSelection: Dispatch<SetStateAction<CodeMirrorCurrentSelection>>;
  setFloatingButtonPos: Dispatch<SetStateAction<FloatingButtonPosition>>;
}) {
  const touchSelectionActiveRef = useRef(false);
  const touchSelectionTimerRef = useRef<number | null>(null);

  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper || !enabled) return;
    const ownerDocument = wrapper.ownerDocument;
    const handleTouchStart = (event: TouchEvent) => {
      if (
        event.target instanceof Element &&
        event.target.closest(".floating-comment-btn, [data-selection-comment-dock-action]")
      ) {
        return;
      }
      touchSelectionActiveRef.current = true;
      setCurrentSelection(null);
      setFloatingButtonPos(null);
    };
    const handleTouchEnd = () => {
      if (touchSelectionTimerRef.current !== null) {
        window.clearTimeout(touchSelectionTimerRef.current);
      }
      touchSelectionTimerRef.current = window.setTimeout(() => {
        touchSelectionTimerRef.current = null;
        if (!touchSelectionActiveRef.current) return;
        const view = editorRef.current?.view;
        if (!view) return;
        setCurrentSelection(readCurrentSelection(view));
        setFloatingButtonPos(null);
      }, 10);
    };
    const handleOutsidePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (event.pointerType === "mouse" || event.pointerType === "pen") {
        if (target instanceof Node && wrapper.contains(target)) {
          touchSelectionActiveRef.current = false;
          if (touchSelectionTimerRef.current !== null) {
            window.clearTimeout(touchSelectionTimerRef.current);
            touchSelectionTimerRef.current = null;
          }
          return;
        }
      }
      if (
        target instanceof Element &&
        target.closest("[data-selection-comment-dock-action], .floating-comment-btn")
      ) {
        return;
      }
      if (!(target instanceof Node) || !wrapper.contains(target)) {
        touchSelectionActiveRef.current = false;
        if (touchSelectionTimerRef.current !== null) {
          window.clearTimeout(touchSelectionTimerRef.current);
          touchSelectionTimerRef.current = null;
        }
        setCurrentSelection(null);
      }
    };
    wrapper.addEventListener("touchstart", handleTouchStart, { passive: true });
    wrapper.addEventListener("touchend", handleTouchEnd, { passive: true });
    ownerDocument.addEventListener("pointerdown", handleOutsidePointerDown, true);
    return () => {
      wrapper.removeEventListener("touchstart", handleTouchStart);
      wrapper.removeEventListener("touchend", handleTouchEnd);
      ownerDocument.removeEventListener("pointerdown", handleOutsidePointerDown, true);
      if (touchSelectionTimerRef.current !== null) {
        window.clearTimeout(touchSelectionTimerRef.current);
        touchSelectionTimerRef.current = null;
      }
      touchSelectionActiveRef.current = false;
    };
  }, [
    enabled,
    editorRef,
    readCurrentSelection,
    setCurrentSelection,
    setFloatingButtonPos,
    wrapperRef,
  ]);

  return { touchSelectionActiveRef, touchSelectionTimerRef };
}
