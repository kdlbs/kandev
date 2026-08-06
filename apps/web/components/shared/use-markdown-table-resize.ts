"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
  type SetStateAction,
} from "react";
import { resizeAdjacentColumns } from "@/lib/markdown/table-resize";

type TableGeometry = {
  boundaries: number[];
  columnWidths: number[];
  height: number;
  tableWidth: number;
  top: number;
};

type DragState = {
  boundaryIndex: number;
  pointerId: number;
  restoreTableWidth: number | null;
  restoreWidths: number[] | null;
  startWidths: number[];
  startX: number;
};

function readGeometry(table: HTMLTableElement, wrapper: HTMLDivElement): TableGeometry | null {
  const cells = Array.from(table.rows.item(0)?.cells ?? []);
  const tableRect = table.getBoundingClientRect();
  const wrapperRect = wrapper.getBoundingClientRect();
  if (cells.length < 2 || tableRect.width <= 0 || tableRect.height <= 0) return null;

  const cellRects = cells.map((cell) => cell.getBoundingClientRect());
  return {
    boundaries: cellRects
      .slice(0, -1)
      .map((rect) => rect.right - wrapperRect.left + wrapper.scrollLeft),
    columnWidths: cellRects.map((rect) => rect.width),
    height: tableRect.height,
    tableWidth: tableRect.width,
    top: tableRect.top - wrapperRect.top + wrapper.scrollTop,
  };
}

function setDocumentResizeState(active: boolean) {
  document.body.style.cursor = active ? "col-resize" : "";
  document.body.style.userSelect = active ? "none" : "";
}

type TableRefs = {
  tableRef: RefObject<HTMLTableElement | null>;
  wrapperRef: RefObject<HTMLDivElement | null>;
};

function useTableGeometry(
  enabled: boolean,
  refs: TableRefs,
  widthsRef: RefObject<number[] | null>,
  reset: () => void,
) {
  const [geometry, setGeometry] = useState<TableGeometry | null>(null);
  const measure = useCallback(() => {
    const table = refs.tableRef.current;
    const wrapper = refs.wrapperRef.current;
    if (!enabled || !table || !wrapper) {
      if (!enabled && widthsRef.current) reset();
      setGeometry(null);
      return;
    }
    const columnCount = table.rows.item(0)?.cells.length ?? 0;
    if (widthsRef.current && widthsRef.current.length !== columnCount) reset();
    const next = readGeometry(table, wrapper);
    setGeometry(next);
  }, [enabled, refs, reset, widthsRef]);

  useLayoutEffect(() => {
    measure();
    const table = refs.tableRef.current;
    const wrapper = refs.wrapperRef.current;
    if (!enabled || !table || !wrapper) return;

    const resizeObserver = new ResizeObserver(measure);
    resizeObserver.observe(table);
    resizeObserver.observe(wrapper);
    const mutationObserver = new MutationObserver(measure);
    mutationObserver.observe(table, { childList: true, subtree: true });
    window.addEventListener("resize", measure);
    return () => {
      resizeObserver.disconnect();
      mutationObserver.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [enabled, measure, refs]);
  return geometry;
}

type PointerResizeOptions = {
  enabled: boolean;
  fixedTableWidth: number | null;
  refs: TableRefs;
  setColumnWidths: (widths: number[] | null) => void;
  setFixedTableWidth: Dispatch<SetStateAction<number | null>>;
  widthsRef: RefObject<number[] | null>;
};

function usePointerResize({
  enabled,
  fixedTableWidth,
  refs,
  setColumnWidths,
  setFixedTableWidth,
  widthsRef,
}: PointerResizeOptions) {
  const dragRef = useRef<DragState | null>(null);
  const [activeBoundary, setActiveBoundary] = useState<number | null>(null);
  const finishDrag = useCallback(
    (event: PointerEvent<HTMLButtonElement>, cancelled: boolean) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== event.pointerId) return;
      if (cancelled) {
        setColumnWidths(drag.restoreWidths);
        setFixedTableWidth(drag.restoreTableWidth);
      }
      dragRef.current = null;
      setActiveBoundary(null);
      setDocumentResizeState(false);
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
    },
    [setColumnWidths],
  );

  useEffect(
    () => () => {
      if (!dragRef.current) return;
      dragRef.current = null;
      setDocumentResizeState(false);
    },
    [],
  );

  useEffect(() => {
    if (enabled || !dragRef.current) return;
    dragRef.current = null;
    setActiveBoundary(null);
    setDocumentResizeState(false);
  }, [enabled]);

  const startResize = useCallback(
    (boundaryIndex: number, event: PointerEvent<HTMLButtonElement>) => {
      if (event.button !== 0) return;
      const table = refs.tableRef.current;
      const wrapper = refs.wrapperRef.current;
      if (!table || !wrapper) return;
      const measured = readGeometry(table, wrapper);
      if (!measured) return;

      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      dragRef.current = {
        boundaryIndex,
        pointerId: event.pointerId,
        restoreTableWidth: fixedTableWidth,
        restoreWidths: widthsRef.current ? [...widthsRef.current] : null,
        startWidths: measured.columnWidths,
        startX: event.clientX,
      };
      setColumnWidths(measured.columnWidths);
      setFixedTableWidth(measured.tableWidth);
      setActiveBoundary(boundaryIndex);
      setDocumentResizeState(true);
    },
    [fixedTableWidth, refs, setColumnWidths, setFixedTableWidth, widthsRef],
  );

  const moveResize = useCallback(
    (event: PointerEvent<HTMLButtonElement>) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== event.pointerId) return;
      event.preventDefault();
      setColumnWidths(
        resizeAdjacentColumns(drag.startWidths, drag.boundaryIndex, event.clientX - drag.startX),
      );
    },
    [setColumnWidths],
  );

  return { activeBoundary, finishDrag, moveResize, startResize };
}

export function useMarkdownTableResize(enabled: boolean) {
  const wrapperRef = useRef<HTMLDivElement>(null);
  const tableRef = useRef<HTMLTableElement>(null);
  const refs = useMemo(() => ({ tableRef, wrapperRef }), []);
  const widthsRef = useRef<number[] | null>(null);
  const [columnWidths, setColumnWidthsState] = useState<number[] | null>(null);
  const [fixedTableWidth, setFixedTableWidth] = useState<number | null>(null);

  const setColumnWidths = useCallback((widths: number[] | null) => {
    widthsRef.current = widths;
    setColumnWidthsState(widths);
  }, []);
  const reset = useCallback(() => {
    setColumnWidths(null);
    setFixedTableWidth(null);
  }, [setColumnWidths]);
  const geometry = useTableGeometry(enabled, refs, widthsRef, reset);
  const pointerResize = usePointerResize({
    enabled,
    fixedTableWidth,
    refs,
    setColumnWidths,
    setFixedTableWidth,
    widthsRef,
  });

  const resizeWithKeyboard = useCallback(
    (boundaryIndex: number, event: KeyboardEvent<HTMLButtonElement>) => {
      if (event.key === "Enter") {
        event.preventDefault();
        reset();
        return;
      }
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;

      const table = refs.tableRef.current;
      const wrapper = refs.wrapperRef.current;
      const measured = table && wrapper ? readGeometry(table, wrapper) : null;
      const widths = widthsRef.current ?? measured?.columnWidths;
      if (!widths || !measured) return;
      event.preventDefault();
      setFixedTableWidth(fixedTableWidth ?? measured.tableWidth);
      setColumnWidths(
        resizeAdjacentColumns(widths, boundaryIndex, event.key === "ArrowRight" ? 8 : -8),
      );
    },
    [fixedTableWidth, refs, reset, setColumnWidths],
  );

  return {
    ...pointerResize,
    columnWidths,
    fixedTableWidth,
    geometry,
    reset,
    resizeWithKeyboard,
    tableRef,
    wrapperRef,
  };
}
