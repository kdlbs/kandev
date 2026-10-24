import type { DockviewApi } from "dockview-react";
import { getRootSplitview } from "./applier";
import { fromDockviewApi } from "./serializer";
import { getReusableRightWidth, resolveReusableRootSizes } from "./pinned-targets";

/** Restore owned automatic root weights without rebuilding panels or overriding manual pixels. */
export function restoreReusableRootGeometry(
  api: DockviewApi,
  envId: string | null | undefined,
  width: number | undefined,
  rightTarget: number,
  manualWidth: number | null,
): boolean {
  if (manualWidth !== null || width === undefined || getReusableRightWidth(envId, width) === null)
    return false;
  const live = fromDockviewApi(api);
  const sizes = resolveReusableRootSizes(envId, live, width, rightTarget);
  const sv = getRootSplitview(api);
  if (!sizes || !sv || sizes.length !== sv.length) return false;
  try {
    // Release the old right extent before fixing the prefix; changed caps need not clamp it eagerly.
    const rightIndex = live.columns.findIndex((column) => column.id === "right" && column.pinned);
    if (rightIndex >= 0 && Math.abs(sv.getViewSize(rightIndex) - rightTarget) >= 1) {
      sv.resizeView(rightIndex, rightTarget);
    }
    // With live extents in bounds, each fixed prefix leaves the suffix its remaining space.
    for (let index = 0; index < sizes.length - 1; index++) {
      if (Math.abs(sv.getViewSize(index) - sizes[index]) >= 1) sv.resizeView(index, sizes[index]);
    }
  } catch {
    // Component bounds may make the source allocation unreachable on this container.
    return false;
  }
  return true;
}
