"use client";

import { defaultRangeExtractor, useVirtualizer, type Range } from "@tanstack/react-virtual";
import {
  useCallback,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type FocusEvent,
  type ReactNode,
} from "react";
import { measureFileTreeElement } from "./file-tree-measurement";
import {
  captureChangesTimelineAnchor,
  observeChangesTimelinePresentationChanges,
  resolveChangesTimelineAnchor,
  scrollTopForChangesTimelineAnchor,
  type ChangesTimelineAnchor,
} from "./changes-timeline-measurement";
import {
  changesTimelineNextIndex,
  isChangesTimelineTextEntry,
  type ChangesTimelineNavigationKey,
} from "./changes-timeline-interaction";
import {
  buildVisibleGroupTree,
  resolveRetainedFocusIndex,
  visibleNodeKey,
  type ChangesTimelineGroup,
  type ChangesTimelineRowIdentity,
  type ChangesTimelineVisibleNode,
} from "./changes-timeline-viewport-groups";

export type {
  ChangesTimelineGroup,
  ChangesTimelineRowIdentity,
} from "./changes-timeline-viewport-groups";

export type ChangesTimelineFocusRequest = {
  rowKey: string;
  token: number;
};

type ChangesTimelineViewportProps<Row extends ChangesTimelineRowIdentity> = {
  rows: Row[];
  scrollElement: HTMLDivElement | null;
  estimateSize: (row: Row, index: number) => number;
  renderRow: (row: Row, index: number) => ReactNode;
  getGroups?: (row: Row) => ChangesTimelineGroup[];
  getGroup?: (row: Row) => ChangesTimelineGroup | undefined;
  getNestedGroup?: (row: Row) => ChangesTimelineGroup | undefined;
  scrollMargin?: number;
  bottomPadding?: number;
  contextKey?: string;
  focusRequest?: ChangesTimelineFocusRequest;
};

/** Render the bounded window for a timeline that uses its panel's scroll owner. */
export function ChangesTimelineViewport<Row extends ChangesTimelineRowIdentity>({
  rows,
  scrollElement,
  estimateSize,
  renderRow,
  getGroups,
  getGroup,
  getNestedGroup,
  scrollMargin = 0,
  bottomPadding = 0,
  contextKey = "",
  focusRequest,
}: ChangesTimelineViewportProps<Row>) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const focusState = useTimelineViewportFocusState<Row>(rows, contextKey, focusRequest);
  const { focusedRowKey } = focusState;
  const indexByKey = useMemo(() => new Map(rows.map((row, index) => [row.key, index])), [rows]);
  const getRowGroups = useCallback(
    (row: Row) => resolveRowGroups(row, getGroups, getGroup, getNestedGroup),
    [getGroup, getGroups, getNestedGroup],
  );
  const rangeExtractor = useCallback(
    (range: Range) => {
      const visible = defaultRangeExtractor(range);
      const retainedIndex = focusedRowKey === null ? undefined : indexByKey.get(focusedRowKey);
      if (retainedIndex === undefined || visible.includes(retainedIndex)) return visible;
      return [...new Set([...visible, retainedIndex])].sort((left, right) => left - right);
    },
    [focusedRowKey, indexByKey],
  );
  const virtualizer = useVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: rows.length,
    getScrollElement: () => scrollElement,
    estimateSize: (index) => estimateSize(rows[index], index),
    getItemKey: (index) => rows[index]?.key ?? index,
    initialRect: { width: 800, height: 600 },
    measureElement: measureFileTreeElement,
    overscan: 5,
    scrollMargin,
    rangeExtractor,
  });
  const viewportHeight = Math.max(0, virtualizer.getTotalSize() - scrollMargin);
  const visibleItems = virtualizer.getVirtualItems();
  const groupTree = buildVisibleGroupTree(visibleItems, rows, getRowGroups);
  useTimelineViewportLifecycle({
    rows,
    scrollElement,
    contextKey,
    focusRequest,
    indexByKey,
    virtualizer,
    viewportRef,
    state: focusState,
    getRowGroups,
  });
  const handlers = useTimelineViewportHandlers({
    rows,
    indexByKey,
    virtualizer,
    viewportRef,
    state: focusState,
    getRowGroups,
  });

  return (
    <div
      ref={viewportRef}
      data-testid="changes-timeline-viewport"
      tabIndex={-1}
      onFocusCapture={handlers.handleFocusCapture}
      onBlurCapture={handlers.handleBlurCapture}
      onKeyDownCapture={handlers.handleKeyDownCapture}
      className="relative w-full min-w-0 max-w-full"
      style={{ height: `${viewportHeight + bottomPadding}px` }}
    >
      {groupTree.map((node) => (
        <VisibleTimelineNode
          key={visibleNodeKey(node)}
          node={node}
          rows={rows}
          renderRow={renderRow}
          virtualizer={virtualizer}
          parentStart={scrollMargin}
        />
      ))}
    </div>
  );
}

type ViewportFocusState<Row extends ChangesTimelineRowIdentity> = {
  anchorRef: React.MutableRefObject<ReturnType<typeof captureChangesTimelineAnchor<Row>> | null>;
  previousRowsRef: React.MutableRefObject<Row[]>;
  previousContextKeyRef: React.MutableRefObject<string>;
  previousFocusRequestRef: React.MutableRefObject<ChangesTimelineFocusRequest | undefined>;
  focusRequestFramesRef: React.MutableRefObject<number[]>;
  lastFocusedIndexRef: React.MutableRefObject<number>;
  focusedRowKey: string | null;
  setFocusedRowKey: React.Dispatch<React.SetStateAction<string | null>>;
  pendingFocusKey: string | null;
  setPendingFocusKey: React.Dispatch<React.SetStateAction<string | null>>;
  lastFocusedGroupKeysRef: React.MutableRefObject<string[]>;
  pendingPresentationAnchorRef: React.MutableRefObject<ChangesTimelineAnchor | null>;
};

function useTimelineViewportFocusState<Row extends ChangesTimelineRowIdentity>(
  rows: Row[],
  contextKey: string,
  focusRequest: ChangesTimelineFocusRequest | undefined,
): ViewportFocusState<Row> {
  const [focusedRowKey, setFocusedRowKey] = useState<string | null>(null);
  const [pendingFocusKey, setPendingFocusKey] = useState<string | null>(null);
  return {
    anchorRef: useRef<ReturnType<typeof captureChangesTimelineAnchor<Row>>>(null),
    previousRowsRef: useRef(rows),
    previousContextKeyRef: useRef(contextKey),
    previousFocusRequestRef: useRef(focusRequest),
    focusRequestFramesRef: useRef<number[]>([]),
    lastFocusedIndexRef: useRef(0),
    lastFocusedGroupKeysRef: useRef([]),
    pendingPresentationAnchorRef: useRef(null),
    focusedRowKey,
    setFocusedRowKey,
    pendingFocusKey,
    setPendingFocusKey,
  };
}

function resolveRowGroups<Row extends ChangesTimelineRowIdentity>(
  row: Row,
  getGroups: ChangesTimelineViewportProps<Row>["getGroups"],
  getGroup: ChangesTimelineViewportProps<Row>["getGroup"],
  getNestedGroup: ChangesTimelineViewportProps<Row>["getNestedGroup"],
): ChangesTimelineGroup[] {
  if (getGroups) return getGroups(row);
  return [getGroup?.(row), getNestedGroup?.(row)].filter(
    (group): group is ChangesTimelineGroup => group !== undefined,
  );
}

function useTimelineViewportLifecycle<Row extends ChangesTimelineRowIdentity>({
  rows,
  scrollElement,
  contextKey,
  focusRequest,
  indexByKey,
  virtualizer,
  viewportRef,
  state,
  getRowGroups,
}: {
  rows: Row[];
  scrollElement: HTMLDivElement | null;
  contextKey: string;
  focusRequest: ChangesTimelineFocusRequest | undefined;
  indexByKey: Map<string, number>;
  virtualizer: ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>;
  viewportRef: React.RefObject<HTMLDivElement | null>;
  state: ViewportFocusState<Row>;
  getRowGroups: (row: Row) => ChangesTimelineGroup[];
}) {
  const latestPresentationStateRef = useRef({ rows, scrollElement, virtualizer, state });
  latestPresentationStateRef.current = { rows, scrollElement, virtualizer, state };
  const cancelFocusRequestFrames = useCallback(() => {
    state.focusRequestFramesRef.current.forEach((frame) => cancelAnimationFrame(frame));
    state.focusRequestFramesRef.current = [];
  }, [state.focusRequestFramesRef]);

  useLayoutEffect(() => {
    if (!scrollElement) return;
    return observeChangesTimelinePresentationChanges(scrollElement, () => {
      const current = latestPresentationStateRef.current;
      const anchor = captureChangesTimelineAnchor(
        current.rows,
        current.virtualizer.getVirtualItems(),
        current.scrollElement?.scrollTop ?? 0,
      );
      current.state.pendingPresentationAnchorRef.current = anchor;
      current.virtualizer.measure();
    });
  }, [scrollElement, virtualizer]);

  useLayoutEffect(() => cancelFocusRequestFrames, [cancelFocusRequestFrames]);
  useLayoutEffect(() => {
    const contextChanged = resetTimelineContext({
      contextKey,
      focusRequest,
      scrollElement,
      virtualizer,
      viewportRef,
      state,
      cancelFocusRequestFrames,
    });
    if (!contextChanged) {
      restoreTimelineAnchor(rows, indexByKey, virtualizer, state);
      restorePresentationAnchor(rows, indexByKey, virtualizer, state);
      scheduleRequestedFocus(
        focusRequest,
        indexByKey,
        virtualizer,
        state,
        cancelFocusRequestFrames,
      );
      state.previousFocusRequestRef.current = focusRequest;
    }
    state.previousRowsRef.current = rows;
    state.previousContextKeyRef.current = contextKey;
    reconcileRetainedFocus({
      rows,
      getRowGroups,
      indexByKey,
      virtualizer,
      viewportRef,
      state,
    });
    focusPendingRow(viewportRef, state);
    recordTimelineAnchor(rows, scrollElement, virtualizer, state);
  });
}

function resetTimelineContext<Row extends ChangesTimelineRowIdentity>({
  contextKey,
  focusRequest,
  scrollElement,
  virtualizer,
  viewportRef,
  state,
  cancelFocusRequestFrames,
}: {
  contextKey: string;
  focusRequest: ChangesTimelineFocusRequest | undefined;
  scrollElement: HTMLDivElement | null;
  virtualizer: ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>;
  viewportRef: React.RefObject<HTMLDivElement | null>;
  state: ViewportFocusState<Row>;
  cancelFocusRequestFrames: () => void;
}): boolean {
  if (state.previousContextKeyRef.current === contextKey) return false;
  cancelFocusRequestFrames();
  state.previousFocusRequestRef.current = focusRequest;
  const activeElement = document.activeElement;
  if (activeElement instanceof HTMLElement && viewportRef.current?.contains(activeElement)) {
    activeElement.blur();
  }
  state.setFocusedRowKey(null);
  state.setPendingFocusKey(null);
  state.lastFocusedGroupKeysRef.current = [];
  state.pendingPresentationAnchorRef.current = null;
  state.anchorRef.current = null;
  if (scrollElement) scrollElement.scrollTop = 0;
  virtualizer.scrollToOffset(0);
  return true;
}

function restoreTimelineAnchor<Row extends ChangesTimelineRowIdentity>(
  rows: Row[],
  indexByKey: Map<string, number>,
  virtualizer: ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>,
  state: ViewportFocusState<Row>,
) {
  const anchor = state.anchorRef.current;
  if (state.previousRowsRef.current === rows || !anchor) return;
  const target = resolveChangesTimelineAnchor(rows, anchor, indexByKey);
  const offset = target ? virtualizer.getOffsetForIndex(target.index, "start")?.[0] : undefined;
  if (target && offset !== undefined) {
    virtualizer.scrollToOffset(scrollTopForChangesTimelineAnchor(offset, anchor.viewportOffset));
  }
}

function restorePresentationAnchor<Row extends ChangesTimelineRowIdentity>(
  rows: Row[],
  indexByKey: Map<string, number>,
  virtualizer: ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>,
  state: ViewportFocusState<Row>,
) {
  const anchor = state.pendingPresentationAnchorRef.current;
  if (!anchor) return;
  state.pendingPresentationAnchorRef.current = null;
  const target = resolveChangesTimelineAnchor(rows, anchor, indexByKey);
  const offset = target ? virtualizer.getOffsetForIndex(target.index, "start")?.[0] : undefined;
  if (target && offset !== undefined) {
    virtualizer.scrollToOffset(scrollTopForChangesTimelineAnchor(offset, anchor.viewportOffset));
  }
}

function scheduleRequestedFocus<Row extends ChangesTimelineRowIdentity>(
  focusRequest: ChangesTimelineFocusRequest | undefined,
  indexByKey: Map<string, number>,
  virtualizer: ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>,
  state: ViewportFocusState<Row>,
  cancelFocusRequestFrames: () => void,
) {
  if (!isNewFocusRequest(focusRequest, state.previousFocusRequestRef.current)) return;
  cancelFocusRequestFrames();
  const request = focusRequest;
  const firstFrame = requestAnimationFrame(() => {
    const secondFrame = requestAnimationFrame(() => {
      state.focusRequestFramesRef.current = [];
      const focusIndex = indexByKey.get(request.rowKey);
      if (focusIndex === undefined) return;
      state.lastFocusedIndexRef.current = focusIndex;
      state.setFocusedRowKey(request.rowKey);
      state.setPendingFocusKey(request.rowKey);
      virtualizer.scrollToIndex(focusIndex, { align: "auto" });
    });
    state.focusRequestFramesRef.current = [secondFrame];
  });
  state.focusRequestFramesRef.current = [firstFrame];
}

function reconcileRetainedFocus<Row extends ChangesTimelineRowIdentity>({
  rows,
  getRowGroups,
  indexByKey,
  virtualizer,
  viewportRef,
  state,
}: {
  rows: Row[];
  getRowGroups: (row: Row) => ChangesTimelineGroup[];
  indexByKey: Map<string, number>;
  virtualizer: ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>;
  viewportRef: React.RefObject<HTMLDivElement | null>;
  state: ViewportFocusState<Row>;
}) {
  const { focusedRowKey } = state;
  if (!focusedRowKey || indexByKey.has(focusedRowKey)) return;
  const fallbackIndex = resolveRetainedFocusIndex(
    rows,
    getRowGroups,
    state.lastFocusedGroupKeysRef.current,
    state.lastFocusedIndexRef.current,
  );
  const fallbackRow = rows[fallbackIndex];
  state.setFocusedRowKey(fallbackRow?.key ?? null);
  state.setPendingFocusKey(fallbackRow?.key ?? null);
  if (fallbackRow) virtualizer.scrollToIndex(fallbackIndex, { align: "auto" });
  else viewportRef.current?.focus({ preventScroll: true });
}

function focusPendingRow<Row extends ChangesTimelineRowIdentity>(
  viewportRef: React.RefObject<HTMLDivElement | null>,
  state: ViewportFocusState<Row>,
) {
  if (!state.pendingFocusKey) return;
  const rowElement = findRowElement(viewportRef.current, state.pendingFocusKey);
  const focusTarget =
    rowElement?.querySelector<HTMLElement>("[data-changes-row-focus]") ?? rowElement;
  if (!focusTarget) return;
  focusTarget.focus({ preventScroll: true });
  state.setPendingFocusKey(null);
}

function recordTimelineAnchor<Row extends ChangesTimelineRowIdentity>(
  rows: Row[],
  scrollElement: HTMLDivElement | null,
  virtualizer: ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>,
  state: ViewportFocusState<Row>,
) {
  if (!scrollElement) return;
  state.anchorRef.current = captureChangesTimelineAnchor(
    rows,
    virtualizer.getVirtualItems(),
    scrollElement.scrollTop,
  );
}

function useTimelineViewportHandlers<Row extends ChangesTimelineRowIdentity>({
  rows,
  indexByKey,
  virtualizer,
  viewportRef,
  state,
  getRowGroups,
}: {
  rows: Row[];
  indexByKey: Map<string, number>;
  virtualizer: ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>;
  viewportRef: React.RefObject<HTMLDivElement | null>;
  state: ViewportFocusState<Row>;
  getRowGroups: (row: Row) => ChangesTimelineGroup[];
}) {
  const handleFocusCapture = useCallback(
    (event: FocusEvent<HTMLDivElement>) => {
      const rowElement = (event.target as Element).closest<HTMLElement>("[data-changes-row-key]");
      const key = rowElement?.dataset.changesRowKey;
      if (!key) return;
      const index = indexByKey.get(key);
      if (index !== undefined) {
        const row = rows[index];
        state.lastFocusedIndexRef.current = index;
        state.lastFocusedGroupKeysRef.current = row
          ? getRowGroups(row).map((group) => group.key)
          : [];
      }
      state.setFocusedRowKey(key);
    },
    [getRowGroups, indexByKey, rows, state],
  );
  const handleBlurCapture = useCallback(
    (event: FocusEvent<HTMLDivElement>) => {
      const nextTarget = event.relatedTarget;
      if (shouldRetainTimelineFocus(nextTarget, viewportRef.current)) return;
      state.setFocusedRowKey(null);
    },
    [state, viewportRef],
  );
  const handleKeyDownCapture = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      const key = event.key as ChangesTimelineNavigationKey;
      if (!isTimelineNavigationEvent(event, key)) return;
      const currentIndex = indexForFocusedTarget(event.target, indexByKey);
      if (currentIndex === undefined) return;
      const nextIndex = changesTimelineNextIndex(key, currentIndex, rows.length);
      const row = rows[nextIndex];
      if (!row || nextIndex === currentIndex) return;
      event.preventDefault();
      state.lastFocusedIndexRef.current = nextIndex;
      state.lastFocusedGroupKeysRef.current = getRowGroups(row).map((group) => group.key);
      state.setFocusedRowKey(row.key);
      state.setPendingFocusKey(row.key);
      virtualizer.scrollToIndex(nextIndex, { align: "auto" });
    },
    [getRowGroups, indexByKey, rows, state, virtualizer],
  );
  return { handleFocusCapture, handleBlurCapture, handleKeyDownCapture };
}

function shouldRetainTimelineFocus(
  target: EventTarget | null,
  viewport: HTMLDivElement | null,
): boolean {
  if (!(target instanceof Element)) return false;
  if (viewport?.contains(target)) return true;
  return Boolean(
    target.closest(
      '[role="menu"], [role="dialog"], [role="listbox"], [data-radix-popper-content-wrapper]',
    ),
  );
}

function isTimelineNavigationEvent(
  event: KeyboardEvent<HTMLDivElement>,
  key: ChangesTimelineNavigationKey,
): boolean {
  return (
    ["ArrowDown", "ArrowUp", "Home", "End"].includes(key) &&
    !event.defaultPrevented &&
    !event.altKey &&
    !event.ctrlKey &&
    !event.metaKey &&
    !event.shiftKey &&
    !isChangesTimelineTextEntry(event.target)
  );
}

function indexForFocusedTarget(target: EventTarget, indexByKey: Map<string, number>) {
  const rowElement = (target as Element).closest<HTMLElement>("[data-changes-row-key]");
  const key = rowElement?.dataset.changesRowKey;
  return key === undefined ? undefined : indexByKey.get(key);
}

type TimelineVirtualizer = ReturnType<typeof useVirtualizer<HTMLDivElement, HTMLDivElement>>;

function VisibleTimelineNode<Row extends ChangesTimelineRowIdentity>({
  node,
  rows,
  renderRow,
  virtualizer,
  parentStart,
}: {
  node: ChangesTimelineVisibleNode<Row>;
  rows: Row[];
  renderRow: (row: Row, index: number) => ReactNode;
  virtualizer: TimelineVirtualizer;
  parentStart: number;
}) {
  if (node.kind === "row") {
    const row = rows[node.item.index];
    if (!row) return null;
    return (
      <div
        key={node.item.key}
        ref={virtualizer.measureElement}
        data-index={node.item.index}
        data-changes-timeline-row
        data-changes-row-key={row.key}
        tabIndex={-1}
        className="absolute left-0 top-0 w-full"
        style={{ transform: `translateY(${node.item.start - parentStart}px)` }}
      >
        {renderRow(row, node.item.index)}
      </div>
    );
  }
  const first = node.items[0];
  const last = node.items.at(-1);
  if (!first || !last) return null;
  return (
    <div
      key={visibleNodeKey(node)}
      data-testid={node.group.testId}
      {...node.group.attributes}
      className="absolute left-0 top-0 w-full"
      style={{
        transform: `translateY(${first.start - parentStart}px)`,
        height: `${last.end - first.start}px`,
      }}
    >
      {node.children.map((child) => (
        <VisibleTimelineNode
          key={visibleNodeKey(child)}
          node={child}
          rows={rows}
          renderRow={renderRow}
          virtualizer={virtualizer}
          parentStart={first.start}
        />
      ))}
    </div>
  );
}

function isNewFocusRequest(
  request: ChangesTimelineFocusRequest | undefined,
  previous: ChangesTimelineFocusRequest | undefined,
): request is ChangesTimelineFocusRequest {
  return Boolean(
    request && (request.rowKey !== previous?.rowKey || request.token !== previous?.token),
  );
}

function findRowElement(viewport: HTMLDivElement | null, key: string): HTMLElement | null {
  if (!viewport) return null;
  for (const row of viewport.querySelectorAll<HTMLElement>("[data-changes-row-key]")) {
    if (row.dataset.changesRowKey === key) return row;
  }
  return null;
}
