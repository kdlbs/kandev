import type { DockviewApi, Splitview } from "dockview-react";

export type RootSplitview = Pick<Splitview, "length" | "getViewSize" | "resizeView"> &
  Partial<Pick<Splitview, "orientation">> & { sashes?: readonly { container?: HTMLElement }[] };

type ReadableSplitview = Pick<RootSplitview, "getViewSize"> & Partial<RootSplitview>;

/** Dockview exposes no public recursive grid reader; validate this compatibility seam. */
export function readDockviewGrid(api: DockviewApi) {
  const root = (
    api as unknown as {
      component?: {
        gridview?: { root?: { splitview?: unknown; children?: unknown; orientation?: unknown } };
      };
    }
  ).component?.gridview?.root;
  const candidate = root?.splitview as Partial<RootSplitview> | undefined;
  const splitview: ReadableSplitview | null =
    candidate && typeof candidate.getViewSize === "function"
      ? (candidate as ReadableSplitview)
      : null;
  return {
    splitview,
    orientation: root?.orientation,
    children: Array.isArray(root?.children) ? root.children : undefined,
  };
}

export function readResizableRootSplitview(api: DockviewApi): RootSplitview | null {
  const { splitview } = readDockviewGrid(api);
  return splitview &&
    typeof splitview.length === "number" &&
    typeof splitview.resizeView === "function"
    ? (splitview as RootSplitview)
    : null;
}
