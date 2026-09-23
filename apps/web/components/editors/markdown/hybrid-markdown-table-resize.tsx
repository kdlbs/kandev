"use client";

import {
  Fragment,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type Dispatch,
  type KeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type SetStateAction,
} from "react";
import { MIN_MARKDOWN_COLUMN_WIDTH, resizeAdjacentColumns } from "@/lib/markdown/table-resize";
import {
  getTableContext,
  readTableGeometry,
  sameGeometry,
  type TableElementContext,
  type TableEdgeGeometry,
  type TableEdgePointerMode,
} from "./hybrid-markdown-table-edge-geometry";

type StoredTableWidths = {
  tableWidth: number;
  widths: number[];
};

type PendingColumnInsertion = {
  boundaryIndex: number;
  tableKey: string;
};

type ResizeDrag = {
  boundaryIndex: number;
  captureTarget: HTMLButtonElement;
  pointerId: number;
  startWidths: number[];
  startX: number;
  tableKey: string;
  tableWidth: number;
};

type MutableRef<T> = { current: T };

type ResizeStoreRefs = {
  pendingInsertionRef: MutableRef<PendingColumnInsertion | null>;
  storedWidthsRef: MutableRef<Map<string, StoredTableWidths>>;
};

type GeometryState = {
  geometry: TableEdgeGeometry | null;
  reconcile: () => void;
  setGeometry: Dispatch<SetStateAction<TableEdgeGeometry | null>>;
};

type ApplyWidths = (widths: number[], tableWidth: number) => void;

type PointerResizeEvent = Pick<PointerEvent, "clientX" | "pointerId"> & {
  preventDefault: () => void;
};

type PointerResizeApi = {
  activeBoundary: number | null;
  finishDrag: (event: ReactPointerEvent<HTMLButtonElement>, cancelled: boolean) => void;
  moveResize: (event: ReactPointerEvent<HTMLButtonElement>) => void;
  startResize: (boundaryIndex: number, event: ReactPointerEvent<HTMLButtonElement>) => void;
};

type ResizeApi = {
  activeBoundary: number | null;
  finishDrag: PointerResizeApi["finishDrag"];
  geometry: TableEdgeGeometry | null;
  moveResize: PointerResizeApi["moveResize"];
  prepareColumnInsertion: (columnIndex: number) => void;
  reset: () => void;
  resizeWithKeyboard: (boundaryIndex: number, event: KeyboardEvent<HTMLButtonElement>) => void;
  startResize: PointerResizeApi["startResize"];
};

export function MarkdownTableResizeHandles({
  geometry,
  resize,
  t,
}: {
  geometry: TableEdgeGeometry;
  resize: ResizeApi;
  t: (key: string, options?: Record<string, number>) => string;
}) {
  return geometry.boundaries.map((left, index) => {
    const leftWidth = geometry.columnWidths[index];
    const rightWidth = geometry.columnWidths[index + 1];
    if (
      leftWidth === undefined ||
      rightWidth === undefined ||
      !canResizeHybridBoundary(leftWidth, rightWidth)
    ) {
      return null;
    }
    return (
      <Fragment key={`resize-${index}`}>
        <span
          aria-hidden="true"
          className="kandev-markdown-table-resizer-guide"
          style={{ height: geometry.height, left, top: geometry.tableTop }}
        />
        <button
          type="button"
          role="separator"
          aria-orientation="vertical"
          aria-label={t("common:resizeTableColumns", { left: index + 1, right: index + 2 })}
          aria-valuemin={MIN_MARKDOWN_COLUMN_WIDTH}
          aria-valuemax={Math.max(
            MIN_MARKDOWN_COLUMN_WIDTH,
            leftWidth + rightWidth - MIN_MARKDOWN_COLUMN_WIDTH,
          )}
          aria-valuenow={Math.round(leftWidth)}
          data-active={resize.activeBoundary === index}
          data-testid={`markdown-table-resizer-${index}`}
          className="kandev-markdown-table-resizer"
          style={{ height: geometry.resizeHeight, left, top: geometry.resizeTop }}
          onDoubleClick={resize.reset}
          onKeyDown={(event) => resize.resizeWithKeyboard(index, event)}
          onLostPointerCapture={(event) => resize.finishDrag(event, true)}
          onPointerCancel={(event) => resize.finishDrag(event, true)}
          onPointerDown={(event) => resize.startResize(index, event)}
          onPointerMove={resize.moveResize}
          onPointerUp={(event) => resize.finishDrag(event, false)}
        />
      </Fragment>
    );
  });
}

function canResizeHybridBoundary(leftWidth: number, rightWidth: number): boolean {
  return leftWidth > 0 && rightWidth > 0 && leftWidth + rightWidth >= MIN_MARKDOWN_COLUMN_WIDTH * 2;
}

function listenForNativeResize(
  move: (event: PointerEvent) => void,
  finish: (pointerId: number, cancelled: boolean) => void,
): () => void {
  const onPointerMove = (event: PointerEvent) => move(event);
  const onPointerUp = (event: PointerEvent) => finish(event.pointerId, false);
  const onPointerCancel = (event: PointerEvent) => finish(event.pointerId, true);
  window.addEventListener("pointermove", onPointerMove, true);
  window.addEventListener("pointerup", onPointerUp, true);
  window.addEventListener("pointercancel", onPointerCancel, true);
  return () => {
    window.removeEventListener("pointermove", onPointerMove, true);
    window.removeEventListener("pointerup", onPointerUp, true);
    window.removeEventListener("pointercancel", onPointerCancel, true);
  };
}

function applyStoredWidths(context: TableElementContext, stored: StoredTableWidths): void {
  const { table } = context;
  table.dataset.kandevMarkdownTableWidths = "true";
  table.style.tableLayout = "fixed";
  table.style.width = `${stored.tableWidth}px`;

  let colgroup = table.querySelector<HTMLElement>(":scope > colgroup");
  if (!colgroup) {
    colgroup = document.createElement("colgroup");
    table.prepend(colgroup);
  }
  colgroup.dataset.kandevMarkdownTableWidths = "true";
  while (colgroup.children.length > stored.widths.length) {
    colgroup.lastElementChild?.remove();
  }
  while (colgroup.children.length < stored.widths.length) {
    colgroup.append(document.createElement("col"));
  }
  Array.from(colgroup.children).forEach((column, index) => {
    (column as HTMLElement).style.width = `${stored.widths[index]}px`;
  });
}

function clearStoredWidths(context: TableElementContext): void {
  const { table } = context;
  if (table.dataset.kandevMarkdownTableWidths !== "true") return;
  table.style.tableLayout = "";
  table.style.width = "";
  table.dataset.kandevMarkdownTableWidths = "";
  table
    .querySelector<HTMLElement>(":scope > colgroup[data-kandev-markdown-table-widths]")
    ?.remove();
}

function getInsertedWidth(
  widths: readonly number[],
  boundaryIndex: number,
  tableWidth: number,
): number {
  const adjacent = widths[boundaryIndex] ?? tableWidth / Math.max(widths.length, 1);
  return Math.max(MIN_MARKDOWN_COLUMN_WIDTH, adjacent);
}

function insertWidth(
  widths: readonly number[],
  boundaryIndex: number,
  tableWidth: number,
): number[] {
  const inserted = getInsertedWidth(widths, boundaryIndex, tableWidth);
  return [...widths.slice(0, boundaryIndex + 1), inserted, ...widths.slice(boundaryIndex + 1)];
}

function updateGeometry(
  setGeometry: Dispatch<SetStateAction<TableEdgeGeometry | null>>,
  next: TableEdgeGeometry | null,
  displayedWidths?: readonly number[],
): void {
  setGeometry((current) => {
    if (!next) return current === null ? current : null;
    const adjusted = displayedWidths ? { ...next, columnWidths: [...displayedWidths] } : next;
    return sameGeometry(current, adjusted) ? current : adjusted;
  });
}

function getReconciledStoredWidths(
  tableKey: string | null,
  columnCount: number,
  refs: ResizeStoreRefs,
): StoredTableWidths | undefined {
  if (!tableKey) return undefined;
  const stored = refs.storedWidthsRef.current.get(tableKey);
  const pending = refs.pendingInsertionRef.current;
  if (!stored || stored.widths.length === columnCount) return stored;
  if (pending?.tableKey !== tableKey || columnCount !== stored.widths.length + 1) {
    refs.storedWidthsRef.current.delete(tableKey);
    return undefined;
  }

  const insertedWidth = getInsertedWidth(stored.widths, pending.boundaryIndex, stored.tableWidth);
  const reconciled = {
    tableWidth: stored.tableWidth + insertedWidth,
    widths: insertWidth(stored.widths, pending.boundaryIndex, stored.tableWidth),
  };
  refs.storedWidthsRef.current.set(tableKey, reconciled);
  refs.pendingInsertionRef.current = null;
  return reconciled;
}

function useHybridTableGeometry(
  host: HTMLDivElement | null,
  tableKey: string | null,
  pointerMode: TableEdgePointerMode,
  refs: ResizeStoreRefs,
): GeometryState {
  const [geometry, setGeometry] = useState<TableEdgeGeometry | null>(null);
  const reconcile = useCallback(() => {
    const context = getTableContext(host);
    const measured = context ? readTableGeometry(context, pointerMode) : null;
    if (!measured) {
      updateGeometry(setGeometry, null);
      return;
    }
    const stored = getReconciledStoredWidths(tableKey, measured.columnWidths.length, refs);
    if (stored && context) applyStoredWidths(context, stored);
    updateGeometry(setGeometry, measured, stored?.widths);
  }, [host, pointerMode, refs, tableKey]);

  useLayoutEffect(() => reconcile(), [reconcile]);

  useEffect(() => {
    const context = getTableContext(host);
    if (!context) return;
    const mutationObserver = new MutationObserver(reconcile);
    mutationObserver.observe(context.wrapper, {
      attributeFilter: ["class"],
      attributes: true,
      characterData: true,
      childList: true,
      subtree: true,
    });
    const resizeObserver =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(reconcile);
    resizeObserver?.observe(context.wrapper);
    resizeObserver?.observe(context.table);
    context.wrapper.addEventListener("scroll", reconcile, { passive: true });
    window.addEventListener("resize", reconcile);
    return () => {
      mutationObserver.disconnect();
      resizeObserver?.disconnect();
      context.wrapper.removeEventListener("scroll", reconcile);
      window.removeEventListener("resize", reconcile);
    };
  }, [host, reconcile]);

  return { geometry, reconcile, setGeometry };
}

function useKeyboardTableResize(
  host: HTMLDivElement | null,
  tableKey: string | null,
  storedWidthsRef: MutableRef<Map<string, StoredTableWidths>>,
  applyWidths: ApplyWidths,
  reset: () => void,
) {
  return useCallback(
    (boundaryIndex: number, event: KeyboardEvent<HTMLButtonElement>) => {
      if (event.key === "Enter") {
        event.preventDefault();
        reset();
        return;
      }
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
      const context = getTableContext(host);
      const measured = context ? readTableGeometry(context, "fine") : null;
      if (!measured || !tableKey) return;
      const stored = storedWidthsRef.current.get(tableKey);
      const widths = stored?.widths ?? measured.columnWidths;
      const delta = event.key === "ArrowRight" ? 8 : -8;
      event.preventDefault();
      applyWidths(
        resizeAdjacentColumns(widths, boundaryIndex, delta),
        stored?.tableWidth ?? measured.tableWidth,
      );
    },
    [applyWidths, host, reset, storedWidthsRef, tableKey],
  );
}

function usePointerTableResize(
  host: HTMLDivElement | null,
  tableKey: string | null,
  storedWidthsRef: MutableRef<Map<string, StoredTableWidths>>,
  applyWidths: ApplyWidths,
  setActiveBoundary: (boundaryIndex: number | null) => void,
): PointerResizeApi {
  const dragRef = useRef<ResizeDrag | null>(null);
  const globalListenersRef = useRef<(() => void) | null>(null);
  const clearGlobalListeners = useCallback(() => {
    globalListenersRef.current?.();
    globalListenersRef.current = null;
  }, []);
  const moveNativeResize = useCallback(
    (event: PointerResizeEvent) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== event.pointerId) return;
      event.preventDefault();
      applyWidths(
        resizeAdjacentColumns(drag.startWidths, drag.boundaryIndex, event.clientX - drag.startX),
        drag.tableWidth,
      );
    },
    [applyWidths],
  );
  const finishNativeResize = useCallback(
    (pointerId: number, cancelled: boolean) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== pointerId) return;
      if (cancelled) applyWidths(drag.startWidths, drag.tableWidth);
      dragRef.current = null;
      clearGlobalListeners();
      setActiveBoundary(null);
      if (drag.captureTarget.hasPointerCapture?.(pointerId)) {
        drag.captureTarget.releasePointerCapture(pointerId);
      }
    },
    [applyWidths, clearGlobalListeners, setActiveBoundary],
  );
  const startResize = useCallback(
    (boundaryIndex: number, event: ReactPointerEvent<HTMLButtonElement>) => {
      if (event.button !== 0 && event.pointerType !== "touch") return;
      const context = getTableContext(host);
      const measured = context ? readTableGeometry(context, "coarse") : null;
      if (!measured || !tableKey) return;
      const stored = storedWidthsRef.current.get(tableKey);
      event.preventDefault();
      if (typeof event.currentTarget.setPointerCapture === "function") {
        try {
          event.currentTarget.setPointerCapture(event.pointerId);
        } catch {
          // Pointer capture is unavailable for some synthetic or interrupted gestures.
        }
      }
      dragRef.current = {
        boundaryIndex,
        captureTarget: event.currentTarget,
        pointerId: event.pointerId,
        startWidths: [...(stored?.widths ?? measured.columnWidths)],
        startX: event.clientX,
        tableKey,
        tableWidth: stored?.tableWidth ?? measured.tableWidth,
      };
      globalListenersRef.current = listenForNativeResize(moveNativeResize, finishNativeResize);
      setActiveBoundary(boundaryIndex);
    },
    [
      finishNativeResize,
      globalListenersRef,
      host,
      moveNativeResize,
      setActiveBoundary,
      storedWidthsRef,
      tableKey,
    ],
  );
  const moveResize = useCallback(
    (event: ReactPointerEvent<HTMLButtonElement>) => {
      moveNativeResize(event);
    },
    [moveNativeResize],
  );
  const finishDrag = useCallback(
    (event: ReactPointerEvent<HTMLButtonElement>, cancelled: boolean) => {
      finishNativeResize(event.pointerId, cancelled);
    },
    [finishNativeResize],
  );
  useEffect(
    () => () => {
      clearGlobalListeners();
      dragRef.current = null;
      setActiveBoundary(null);
    },
    [clearGlobalListeners, setActiveBoundary],
  );
  return { activeBoundary: null, finishDrag, moveResize, startResize };
}

export function useHybridTableResize(
  host: HTMLDivElement | null,
  tableKey: string | null,
  pointerMode: TableEdgePointerMode,
): ResizeApi {
  const storedWidthsRef = useRef(new Map<string, StoredTableWidths>());
  const pendingInsertionRef = useRef<PendingColumnInsertion | null>(null);
  const refs = useRef<ResizeStoreRefs>({ pendingInsertionRef, storedWidthsRef }).current;
  const geometryState = useHybridTableGeometry(host, tableKey, pointerMode, refs);
  const [activeBoundary, setActiveBoundary] = useState<number | null>(null);
  const applyWidths = useCallback(
    (widths: number[], tableWidth: number) => {
      if (!tableKey) return;
      const stored = { tableWidth, widths };
      storedWidthsRef.current.set(tableKey, stored);
      const context = getTableContext(host);
      if (!context) return;
      applyStoredWidths(context, stored);
      const measured = readTableGeometry(context, pointerMode);
      updateGeometry(geometryState.setGeometry, measured, widths);
    },
    [geometryState.setGeometry, host, pointerMode, tableKey],
  );
  const reset = useCallback(() => {
    if (tableKey) storedWidthsRef.current.delete(tableKey);
    pendingInsertionRef.current = null;
    const context = getTableContext(host);
    if (context) clearStoredWidths(context);
    geometryState.reconcile();
  }, [geometryState.reconcile, host, tableKey]);
  const pointer = usePointerTableResize(
    host,
    tableKey,
    storedWidthsRef,
    applyWidths,
    setActiveBoundary,
  );
  const resizeWithKeyboard = useKeyboardTableResize(
    host,
    tableKey,
    storedWidthsRef,
    applyWidths,
    reset,
  );
  const prepareColumnInsertion = useCallback(
    (boundaryIndex: number) => {
      if (tableKey) pendingInsertionRef.current = { boundaryIndex, tableKey };
    },
    [tableKey],
  );
  return {
    activeBoundary,
    finishDrag: pointer.finishDrag,
    geometry: geometryState.geometry,
    moveResize: pointer.moveResize,
    prepareColumnInsertion,
    reset,
    resizeWithKeyboard,
    startResize: pointer.startResize,
  };
}
