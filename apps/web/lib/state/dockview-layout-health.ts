/**
 * Layout-shape health validation. A persisted dockview layout can become
 * corrupted (zero/negative `size` on a node, missing views, zero container
 * dimensions) — applying it via api.fromJSON() yields collapsed groups the
 * user can't recover from without clearing sessionStorage.
 *
 * Both the initial-mount restore path and the session-switch path consult
 * this validator before handing data to dockview, so any code that loads a
 * layout from storage can guard against persistent corruption.
 */

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function hasValidSizes(node: any): boolean {
  if (!node || typeof node !== "object") return false;
  if (node.size !== undefined && !(typeof node.size === "number" && node.size > 0)) {
    return false;
  }
  if (node.type === "leaf") {
    const views = node.data?.views;
    return Array.isArray(views) && views.length > 0;
  }
  if (node.type === "branch") {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const children = node.data as any[] | undefined;
    if (!Array.isArray(children) || children.length === 0) return false;
    return children.every(hasValidSizes);
  }
  return true;
}

/**
 * Returns true when the serialized dockview layout has a healthy structure:
 *  - container has positive width/height
 *  - every grid node has a positive `size`
 *  - every leaf has at least one view
 *  - top-level `panels` and `grid.root` are present
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function isLayoutShapeHealthy(layout: any): boolean {
  if (!layout?.panels || !layout?.grid?.root) return false;
  const { width, height } = layout.grid;
  if (!(typeof width === "number" && width > 0) || !(typeof height === "number" && height > 0)) {
    return false;
  }
  return hasValidSizes(layout.grid.root);
}
