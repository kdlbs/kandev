import type { AppStatusBarOrderState } from "@/lib/state/slices/settings/types";

export const APP_STATUS_CONNECTION_ID = "builtin:connection";
export const APP_STATUS_METRICS_ID = "builtin:metrics";

export type AppStatusBarSide = "left" | "right";

export type AppStatusItemDescriptor = {
  id: string;
  defaultSide: AppStatusBarSide;
};

export type ProjectedStatusItems<T extends AppStatusItemDescriptor> = {
  left: T[];
  right: T[];
};

export type AppStatusItemGeometry = {
  id: string;
  side: AppStatusBarSide;
  left: number;
  right: number;
};

export type AppStatusDropTarget = {
  side: AppStatusBarSide;
  activeIndex: number;
};

export function reconcileAppStatusBarOrder(
  saved: AppStatusBarOrderState,
  activeItems: AppStatusItemDescriptor[],
): AppStatusBarOrderState {
  const seen = new Set<string>();
  const leftItemIds = uniqueItemIds(saved.leftItemIds, seen);
  const rightItemIds = uniqueItemIds(saved.rightItemIds, seen);

  for (const item of activeItems) {
    if (!item.id || seen.has(item.id)) continue;
    seen.add(item.id);
    (item.defaultSide === "left" ? leftItemIds : rightItemIds).push(item.id);
  }
  return { leftItemIds, rightItemIds };
}

export function projectActiveStatusItems<T extends AppStatusItemDescriptor>(
  order: AppStatusBarOrderState,
  activeItems: T[],
): ProjectedStatusItems<T> {
  const activeById = new Map<string, T>();
  for (const item of activeItems) {
    if (item.id && !activeById.has(item.id)) activeById.set(item.id, item);
  }
  return {
    left: projectSide(order.leftItemIds, activeById),
    right: projectSide(order.rightItemIds, activeById),
  };
}

export function moveAppStatusItem(
  order: AppStatusBarOrderState,
  itemId: string,
  targetSide: AppStatusBarSide,
  targetActiveIndex: number,
  activeItems: AppStatusItemDescriptor[],
): AppStatusBarOrderState {
  const leftItemIds = order.leftItemIds.filter((id) => id !== itemId);
  const rightItemIds = order.rightItemIds.filter((id) => id !== itemId);
  const targetIds = targetSide === "left" ? leftItemIds : rightItemIds;
  const activeIds = new Set(activeItems.map((item) => item.id));
  const targetActiveIds = targetIds.filter((id) => activeIds.has(id));
  const activeIndex = Math.max(0, Math.min(targetActiveIndex, targetActiveIds.length));
  const insertionIndex =
    activeIndex === 0 ? 0 : targetIds.indexOf(targetActiveIds[activeIndex - 1]) + 1;

  targetIds.splice(insertionIndex, 0, itemId);
  return { leftItemIds, rightItemIds };
}

export function resolveAppStatusDropTarget(
  clientX: number,
  items: AppStatusItemGeometry[],
  spacer: { left: number; right: number },
): AppStatusDropTarget {
  const side: AppStatusBarSide = clientX < (spacer.left + spacer.right) / 2 ? "left" : "right";
  const sideItems = items.filter((item) => item.side === side);
  const activeIndex = sideItems.findIndex((item) => clientX < (item.left + item.right) / 2);
  return { side, activeIndex: activeIndex === -1 ? sideItems.length : activeIndex };
}

function uniqueItemIds(ids: string[], seen: Set<string>): string[] {
  const result: string[] = [];
  for (const id of ids) {
    if (!id || seen.has(id)) continue;
    seen.add(id);
    result.push(id);
  }
  return result;
}

function projectSide<T>(ids: string[], activeById: Map<string, T>): T[] {
  return ids.flatMap((id) => {
    const item = activeById.get(id);
    return item ? [item] : [];
  });
}
