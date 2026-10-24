import type { DockviewApi, DockviewGroupPanel } from "dockview-react";
import { getRootOrientation } from "./applier";
import { fromDockviewApi } from "./serializer";

export type PinnedRightColumn = { index: number; groups: DockviewGroupPanel[] };

/** Resolve the live horizontal pinned right root child, independent of reusable ratio ownership. */
export function getPinnedRightColumn(api: DockviewApi): PinnedRightColumn | null {
  if (getRootOrientation(api) === "VERTICAL") return null;
  const columns = fromDockviewApi(api).columns;
  const index = columns.findIndex((column) => column.id === "right" && column.pinned);
  if (index < 0) return null;
  const groupIds = new Set(columns[index].groups.map((group) => group.id));
  return { index, groups: api.groups.filter((group) => groupIds.has(group.id)) };
}
