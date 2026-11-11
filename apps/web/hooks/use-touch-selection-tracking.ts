import { useEffect, useRef, type RefObject } from "react";

/** Marks a selection action dock; pressing it must not drop the captured selection. */
export const SELECTION_ACTION_DOCK_ATTRIBUTE = "data-selection-action-dock";
const DOCK_SELECTOR = `[${SELECTION_ACTION_DOCK_ATTRIBUTE}]`;

function selectionWithin(root: HTMLElement, selection: Selection | null): Selection | null {
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) return null;
  const { anchorNode, focusNode } = selection;
  if (!anchorNode || !focusNode) return null;
  if (!root.contains(anchorNode) || !root.contains(focusNode)) return null;
  return selection.toString().trim() ? selection : null;
}

/**
 * Reports the live document selection inside `rootRef` on touch devices.
 * Native selection handles change the selection without dispatching touch
 * events to the page, so this follows `selectionchange`, coalesced to one
 * callback per animation frame. A selection that collapses because the user
 * is pressing a selection action dock is not reported.
 */
export function useTouchSelectionTracking(
  rootRef: RefObject<HTMLElement | null>,
  enabled: boolean,
  onChange: (selection: Selection | null) => void,
) {
  const onChangeRef = useRef(onChange);
  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useEffect(() => {
    const root = rootRef.current;
    if (!enabled || !root) return;
    const ownerDocument = root.ownerDocument;
    let frame: number | null = null;
    let pressingDock = false;

    const flush = () => {
      frame = null;
      const selection = selectionWithin(root, ownerDocument.getSelection());
      if (!selection && pressingDock) return;
      onChangeRef.current(selection);
    };
    const onSelectionChange = () => {
      if (frame === null) frame = requestAnimationFrame(flush);
    };
    const onPointerDown = (event: Event) => {
      pressingDock =
        event.target instanceof Element && Boolean(event.target.closest(DOCK_SELECTOR));
    };

    ownerDocument.addEventListener("selectionchange", onSelectionChange);
    ownerDocument.addEventListener("pointerdown", onPointerDown, true);
    return () => {
      ownerDocument.removeEventListener("selectionchange", onSelectionChange);
      ownerDocument.removeEventListener("pointerdown", onPointerDown, true);
      if (frame !== null) cancelAnimationFrame(frame);
    };
  }, [enabled, rootRef]);
}
