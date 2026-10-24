import type { DockviewApi, DockviewGroupPanel } from "dockview-react";
import { getRootOrientation } from "./applier";
import { getReusableRightWidth } from "./pinned-targets";
import { fromDockviewApi } from "./serializer";

export type PinnedRightColumn = { index: number; groups: DockviewGroupPanel[] };

/** Resolve the live root child owned by the reusable right-width source, including generated groups. */
export function getOwnedRightColumn(
  api: DockviewApi,
  envId: string | null | undefined,
): PinnedRightColumn | null {
  if (getRootOrientation(api) === "VERTICAL" || getReusableRightWidth(envId, api.width) === null) {
    return null;
  }
  const columns = fromDockviewApi(api).columns;
  const index = columns.findIndex((column) => column.id === "right" && column.pinned);
  if (index < 0) return null;
  const groupIds = new Set(columns[index].groups.map((group) => group.id));
  return { index, groups: api.groups.filter((group) => groupIds.has(group.id)) };
}
