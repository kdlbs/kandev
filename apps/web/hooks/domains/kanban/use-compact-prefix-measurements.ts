import { useLayoutEffect, useReducer, useRef, useState, type RefObject } from "react";
import type { KanbanPresentation, Task } from "@/components/kanban-card";
import type { KanbanExternalLinkAvailability } from "@/components/kanban-external-link-availability";

export const COMPACT_PREFIX_ROW_COUNT = 6;

export type CompactRowMeasurement = {
  index: number;
  key: string | number | bigint;
  size: number;
};

type CompactPrefixVirtualizer = {
  elementsCache: Map<string | number | bigint, HTMLDivElement>;
  getVirtualItems: () => readonly CompactRowMeasurement[];
  measureElement: (node: HTMLDivElement | null) => void;
  measurementsCache: readonly CompactRowMeasurement[];
  resizeItem: (index: number, size: number) => void;
};

export function getCompactTaskPrefixHeight(
  taskIds: string[],
  measurements: readonly CompactRowMeasurement[],
  estimateSize: (index: number) => number,
  invalidatedKeys?: ReadonlySet<string>,
): number {
  const sizeByKey = new Map<string | number | bigint, number>();
  for (const item of measurements) {
    if (item) sizeByKey.set(item.key, item.size);
  }
  return taskIds
    .slice(0, COMPACT_PREFIX_ROW_COUNT)
    .reduce(
      (total, taskId, index) =>
        total +
        (invalidatedKeys?.has(taskId)
          ? estimateSize(index)
          : (sizeByKey.get(taskId) ?? estimateSize(index))),
      0,
    );
}

function useScrollViewportWidth(scrollRef: RefObject<HTMLDivElement | null>): number {
  const [width, setWidth] = useState(0);

  useLayoutEffect(() => {
    const element = scrollRef.current;
    if (!element) return undefined;

    const update = () => {
      setWidth((previous) => {
        const next = element.clientWidth;
        return previous === next ? previous : next;
      });
    };
    update();

    if (typeof ResizeObserver === "undefined") return undefined;
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [scrollRef]);

  return width;
}

type PrefixRevisionSnapshot = {
  archivingTaskId?: string | null;
  deletingTaskId?: string | null;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  presentation: KanbanPresentation;
  prefixTasks: Task[];
  queuedCount: number;
  queuedStartIndex: number;
  revision: number;
  showMaximizeButton?: boolean;
  viewportWidth: number;
};

function samePrefixTasks(previous: Task[], next: Task[]): boolean {
  return previous.length === next.length && previous.every((task, index) => task === next[index]);
}

function sameExternalLinkAvailability(
  previous: KanbanExternalLinkAvailability,
  next: KanbanExternalLinkAvailability,
): boolean {
  return (
    previous.gitlab === next.gitlab &&
    previous.jira === next.jira &&
    previous.linear === next.linear &&
    previous.sentry === next.sentry
  );
}

function prefixRevisionChanged(
  previous: PrefixRevisionSnapshot | null,
  next: Omit<PrefixRevisionSnapshot, "revision">,
): boolean {
  if (!previous) return true;
  return (
    !samePrefixTasks(previous.prefixTasks, next.prefixTasks) ||
    previous.queuedStartIndex !== next.queuedStartIndex ||
    previous.queuedCount !== next.queuedCount ||
    previous.presentation !== next.presentation ||
    previous.viewportWidth !== next.viewportWidth ||
    previous.showMaximizeButton !== next.showMaximizeButton ||
    previous.deletingTaskId !== next.deletingTaskId ||
    previous.archivingTaskId !== next.archivingTaskId ||
    !sameExternalLinkAvailability(previous.externalLinkAvailability, next.externalLinkAvailability)
  );
}

function useCompactPrefixRevision({
  orderedTasks,
  queuedStartIndex,
  queuedCount,
  presentation,
  viewportWidth,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  externalLinkAvailability,
}: {
  orderedTasks: Task[];
  queuedStartIndex: number;
  queuedCount: number;
  presentation: KanbanPresentation;
  viewportWidth: number;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  externalLinkAvailability: KanbanExternalLinkAvailability;
}): number {
  const previousRef = useRef<PrefixRevisionSnapshot | null>(null);
  const nextSnapshot = {
    prefixTasks: orderedTasks.slice(0, COMPACT_PREFIX_ROW_COUNT),
    queuedStartIndex,
    queuedCount,
    presentation,
    viewportWidth,
    showMaximizeButton,
    deletingTaskId,
    archivingTaskId,
    externalLinkAvailability,
  };
  const previous = previousRef.current;
  if (prefixRevisionChanged(previous, nextSnapshot)) {
    previousRef.current = {
      ...nextSnapshot,
      revision: (previous?.revision ?? 0) + 1,
    };
  }
  return previousRef.current!.revision;
}

export function useCompactPrefixMeasurements({
  scrollRef,
  orderedTasks,
  taskIds,
  queuedStartIndex,
  queuedCount,
  presentation,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  externalLinkAvailability,
  estimateSize,
  virtualizer,
}: {
  scrollRef: RefObject<HTMLDivElement | null>;
  orderedTasks: Task[];
  taskIds: string[];
  queuedStartIndex: number;
  queuedCount: number;
  presentation: KanbanPresentation;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  estimateSize: (index: number) => number;
  virtualizer: CompactPrefixVirtualizer;
}): number {
  const viewportWidth = useScrollViewportWidth(scrollRef);
  const prefixRevision = useCompactPrefixRevision({
    orderedTasks,
    queuedStartIndex,
    queuedCount,
    presentation,
    viewportWidth,
    showMaximizeButton,
    deletingTaskId,
    archivingTaskId,
    externalLinkAvailability,
  });
  const stalePrefixKeysRef = useRef<Set<string>>(new Set());
  const previousPrefixRevisionRef = useRef(prefixRevision);
  const [, forcePrefixRefresh] = useReducer((value: number) => value + 1, 0);
  if (previousPrefixRevisionRef.current !== prefixRevision) {
    stalePrefixKeysRef.current = new Set(taskIds.slice(0, COMPACT_PREFIX_ROW_COUNT));
    previousPrefixRevisionRef.current = prefixRevision;
  }

  const virtualItems = virtualizer.getVirtualItems();
  const measurements = virtualizer.measurementsCache?.length
    ? virtualizer.measurementsCache
    : virtualItems;
  const compactHeight = getCompactTaskPrefixHeight(
    taskIds,
    measurements,
    estimateSize,
    stalePrefixKeysRef.current,
  );

  useLayoutEffect(() => {
    const staleKeys = stalePrefixKeysRef.current;
    if (staleKeys.size === 0) return;

    let remeasured = false;
    for (const [index, taskId] of taskIds.slice(0, COMPACT_PREFIX_ROW_COUNT).entries()) {
      if (!staleKeys.has(taskId)) continue;
      const element = virtualizer.elementsCache.get(taskId);
      if (element) {
        virtualizer.measureElement(element);
        staleKeys.delete(taskId);
        remeasured = true;
      } else if (virtualizer.measurementsCache[index]) {
        virtualizer.resizeItem(index, estimateSize(index));
      }
    }
    if (remeasured) forcePrefixRefresh();
  }, [estimateSize, forcePrefixRefresh, prefixRevision, taskIds, virtualizer, virtualItems]);

  return compactHeight;
}
