import { useEffect, useRef, type RefObject } from "react";

/** Marks a selection action dock; pressing it must not drop the captured selection. */
export const SELECTION_ACTION_DOCK_ATTRIBUTE = "data-selection-action-dock";
const DOCK_SELECTOR = `[${SELECTION_ACTION_DOCK_ATTRIBUTE}]`;

type SelectionListener = {
  root: HTMLElement;
  onChange: RefObject<(selection: Selection | null) => void>;
};

type SelectionTracker = {
  listeners: Set<SelectionListener>;
  frame: number | null;
  pressingDock: boolean;
  onSelectionChange: () => void;
  onPointerDown: (event: Event) => void;
};

const trackers = new WeakMap<Document, SelectionTracker>();

/** The selection when it is non-empty text; otherwise null. */
function textSelection(selection: Selection | null): Selection | null {
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) return null;
  if (!selection.anchorNode || !selection.focusNode) return null;
  return selection.toString().trim() ? selection : null;
}

function selectionWithin(root: HTMLElement, selection: Selection | null): Selection | null {
  if (!selection) return null;
  return root.contains(selection.anchorNode) && root.contains(selection.focusNode)
    ? selection
    : null;
}

function flushTracker(ownerDocument: Document, tracker: SelectionTracker) {
  tracker.frame = null;
  const selection = textSelection(ownerDocument.getSelection());
  for (const listener of tracker.listeners) {
    const within = selectionWithin(listener.root, selection);
    if (!within && tracker.pressingDock) continue;
    listener.onChange.current?.(within);
  }
}

function createTracker(ownerDocument: Document): SelectionTracker {
  const tracker: SelectionTracker = {
    listeners: new Set(),
    frame: null,
    pressingDock: false,
    onSelectionChange: () => {
      if (tracker.frame === null) {
        tracker.frame = requestAnimationFrame(() => flushTracker(ownerDocument, tracker));
      }
    },
    onPointerDown: (event) => {
      tracker.pressingDock =
        event.target instanceof Element && Boolean(event.target.closest(DOCK_SELECTOR));
    },
  };
  ownerDocument.addEventListener("selectionchange", tracker.onSelectionChange);
  ownerDocument.addEventListener("pointerdown", tracker.onPointerDown, true);
  return tracker;
}

function registerSelectionListener(listener: SelectionListener) {
  const ownerDocument = listener.root.ownerDocument;
  let tracker = trackers.get(ownerDocument);
  if (!tracker) {
    tracker = createTracker(ownerDocument);
    trackers.set(ownerDocument, tracker);
  }
  tracker.listeners.add(listener);
  return () => {
    const current = trackers.get(ownerDocument);
    if (!current) return;
    current.listeners.delete(listener);
    if (current.listeners.size > 0) return;
    ownerDocument.removeEventListener("selectionchange", current.onSelectionChange);
    ownerDocument.removeEventListener("pointerdown", current.onPointerDown, true);
    if (current.frame !== null) cancelAnimationFrame(current.frame);
    trackers.delete(ownerDocument);
  };
}

/**
 * Reports the live document selection inside `rootRef` on touch devices.
 * Native selection handles change the selection without dispatching touch
 * events to the page, so this follows `selectionchange`. All tracked surfaces
 * in a document share one listener and one animation-frame callback. A
 * selection that collapses because the user is pressing a selection action
 * dock is not reported.
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
    return registerSelectionListener({ root, onChange: onChangeRef });
  }, [enabled, rootRef]);
}
