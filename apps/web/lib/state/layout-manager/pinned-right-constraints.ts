import type { DockviewApi, DockviewGroupPanel } from "dockview-react";
import { LAYOUT_PINNED_MIN_PX } from "./caps";

type OwnedBounds = {
  minimumWidth: number;
  maximumWidth: number;
  appliedMaximumWidth: number;
};

const ownedBounds = new WeakMap<DockviewApi, Map<DockviewGroupPanel, OwnedBounds>>();

function releaseBounds(group: DockviewGroupPanel, previous: OwnedBounds): void {
  const bounds: { minimumWidth?: number; maximumWidth?: number } = {};
  // Component updates after our last write are not host-owned constraints.
  if (group.minimumWidth === LAYOUT_PINNED_MIN_PX) bounds.minimumWidth = previous.minimumWidth;
  if (group.maximumWidth === previous.appliedMaximumWidth)
    bounds.maximumWidth = previous.maximumWidth;
  if (bounds.minimumWidth !== undefined || bounds.maximumWidth !== undefined)
    group.api.setConstraints(bounds);
}

function rememberComponentBounds(
  group: DockviewGroupPanel,
  bounds: OwnedBounds,
  maximumWidth: number,
): void {
  if (group.minimumWidth !== LAYOUT_PINNED_MIN_PX) bounds.minimumWidth = group.minimumWidth;
  if (group.maximumWidth !== bounds.appliedMaximumWidth) bounds.maximumWidth = group.maximumWidth;
  bounds.appliedMaximumWidth = maximumWidth;
}

/** Refresh every owned group and release only host bounds left on former right roots. */
export function refreshPinnedRightConstraints(
  api: DockviewApi,
  next: { groups: DockviewGroupPanel[]; maximumWidth: number } | null,
): void {
  let previous = ownedBounds.get(api);
  if (!previous) {
    if (!next?.groups.length) return;
    previous = new Map();
    ownedBounds.set(api, previous);
  }
  const liveGroups = api.groups;
  for (const [group, bounds] of previous) {
    if (next?.groups.includes(group)) continue;
    if (liveGroups.includes(group)) releaseBounds(group, bounds);
    previous.delete(group);
  }
  if (!next) return;
  for (const group of next.groups) {
    const bounds = previous.get(group);
    if (bounds) rememberComponentBounds(group, bounds, next.maximumWidth);
    else
      previous.set(group, {
        minimumWidth: group.minimumWidth,
        maximumWidth: group.maximumWidth,
        appliedMaximumWidth: next.maximumWidth,
      });
    if (group.minimumWidth !== LAYOUT_PINNED_MIN_PX || group.maximumWidth !== next.maximumWidth)
      group.api.setConstraints({
        minimumWidth: LAYOUT_PINNED_MIN_PX,
        maximumWidth: next.maximumWidth,
      });
  }
}
