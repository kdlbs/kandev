import { isEnvScopedDockviewComponent } from "@/lib/state/dockview-env-scoped-components";
import { isLayoutShapeHealthy } from "@/lib/state/dockview-layout-health";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { RENDERABLE_COMPONENTS } from "./renderable-components";
import type { LayoutColumn, LayoutGroup, LayoutNode, LayoutPanel, LayoutState } from "./types";

const debug = createDebugLogger("dockview:restore");

/* eslint-disable @typescript-eslint/no-explicit-any */
type SanitizeLayoutOptions =
  | { stripSessionPanels: true; stripEnvScopedPanels?: boolean; excludeSessionIds?: never }
  | {
      stripSessionPanels?: false | undefined;
      stripEnvScopedPanels?: boolean;
      excludeSessionIds?: Set<string>;
    };

function describeSanitizeMode(options: {
  stripSessionPanels?: boolean;
  stripEnvScopedPanels?: boolean;
  excludeSessionIds?: Set<string>;
}): string {
  if (options.stripSessionPanels && options.stripEnvScopedPanels)
    return "stripSessionsAndEnvScoped";
  if (options.stripSessionPanels) return "stripAllSessions";
  if (options.stripEnvScopedPanels) return "stripEnvScoped";
  if (options.excludeSessionIds) return "excludeSpecificSessions";
  return "keepAll";
}

function logSanitizeOutcome(
  options: {
    stripSessionPanels?: boolean;
    stripEnvScopedPanels?: boolean;
    excludeSessionIds?: Set<string>;
  },
  totalPanels: Record<string, any>,
  validPanels: Record<string, any>,
  invalidIds: Set<string>,
): void {
  if (!isDebug()) return;
  debug("sanitizeSerializedLayout", {
    mode: describeSanitizeMode(options),
    excludeSessionCount: options.excludeSessionIds?.size ?? 0,
    excludeSessionIds: options.excludeSessionIds
      ? Array.from(options.excludeSessionIds)
      : undefined,
    totalPanels: Object.keys(totalPanels).length,
    keptPanels: Object.keys(validPanels).length,
    strippedPanels: Array.from(invalidIds),
  });
}

function shouldKeepSessionPanel(id: string, options: SanitizeLayoutOptions): boolean {
  if (options.stripSessionPanels) return false;
  if (!options.excludeSessionIds) return true;

  // Per-env restore: drop session panels that we know belong to a
  // different env (a phantom from a previously-deleted task). Sessions
  // we have no mapping for are kept — they may be a still-loading WS
  // arrival, and useAutoSessionTab's reconcile will clean them up if
  // they turn out to be stale.
  const sid = id.slice("session:".length);
  return !options.excludeSessionIds.has(sid);
}

function shouldKeepPanel(
  id: string,
  panel: any,
  validComponents: ReadonlySet<string>,
  options: SanitizeLayoutOptions,
): boolean {
  const comp = panel.contentComponent;

  // Session panels are scoped to a specific environment; when restoring the
  // global fallback (no envId yet), they belong to the previous task and
  // would leak in as duplicate tabs. Strip them in that case. The session
  // check must happen before component-validity, since session panels are
  // serialized with contentComponent: "chat" (a valid component) and would
  // otherwise short-circuit the strip guard.
  if (id.startsWith("session:")) return shouldKeepSessionPanel(id, options);

  if (options.stripEnvScopedPanels && isEnvScopedDockviewComponent(comp)) return false;
  return !!(comp && validComponents.has(comp));
}
/* eslint-enable @typescript-eslint/no-explicit-any */

/** Leaf group ids present in a serialized dockview grid root. */
export function serializedGridGroupIds(root: unknown): Set<string> {
  const ids = new Set<string>();
  const walk = (node: unknown) => {
    if (!node || typeof node !== "object") return;
    const candidate = node as { type?: string; data?: unknown };
    if (candidate.type === "leaf") {
      const leaf = (candidate.data ?? {}) as { id?: unknown };
      if (typeof leaf.id === "string") ids.add(leaf.id);
      return;
    }
    if (candidate.type === "branch" && Array.isArray(candidate.data)) {
      candidate.data.forEach(walk);
    }
  };
  walk(root);
  return ids;
}

/** The group a maximize overlay maximizes: the grid root's last child leaf,
 *  which is the only non-sidebar group in a maximize payload. */
export function maximizedGroupIdOf(serialized: unknown): string | null {
  const root = (serialized as { grid?: { root?: unknown } } | null)?.grid?.root;
  if (!root || typeof root !== "object") return null;
  const candidate = root as { type?: string; data?: unknown };
  if (candidate.type === "leaf") {
    const leaf = (candidate.data ?? {}) as { id?: unknown };
    return typeof leaf.id === "string" ? leaf.id : null;
  }
  if (candidate.type !== "branch" || !Array.isArray(candidate.data)) return null;
  const last = candidate.data[candidate.data.length - 1];
  if (!last || typeof last !== "object") return null;
  const leaf = last as { type?: string; data?: { id?: unknown } };
  return leaf.type === "leaf" && typeof leaf.data?.id === "string" ? leaf.data.id : null;
}

/**
 * Drop every panel whose component the renderer cannot instantiate, plus the
 * scoping rules the caller asks for, from a serialized dockview payload.
 *
 * This is the choke point for stored payloads that reach `api.fromJSON`:
 * `dockview-react` looks the component up in its `components` map, so an entry
 * whose component is not registered throws inside the initialization queue
 * instead of rendering a placeholder. Returns null when nothing usable is left.
 */
/* eslint-disable @typescript-eslint/no-explicit-any */
export function sanitizeSerializedLayout(
  layout: any,
  validComponents: ReadonlySet<string> = RENDERABLE_COMPONENTS,
  options: SanitizeLayoutOptions = {},
): any {
  if (!isLayoutShapeHealthy(layout)) {
    debug("sanitizeSerializedLayout: layout shape unhealthy, returning null");
    return null;
  }

  const invalidIds = new Set<string>();
  const validPanels: Record<string, any> = {};
  for (const [id, panel] of Object.entries(layout.panels)) {
    if (shouldKeepPanel(id, panel, validComponents, options)) {
      validPanels[id] = panel;
    } else {
      invalidIds.add(id);
    }
  }

  logSanitizeOutcome(options, layout.panels, validPanels, invalidIds);

  if (invalidIds.size === 0) return layout;

  function cleanNode(node: any): any {
    if (node.type === "leaf") {
      const views = (node.data.views as string[]).filter((v) => !invalidIds.has(v));
      if (views.length === 0) return null;
      const activeView = views.includes(node.data.activeView) ? node.data.activeView : views[0];
      return { ...node, data: { ...node.data, views, activeView } };
    }
    if (node.type === "branch") {
      const children = (node.data as any[]).map(cleanNode).filter(Boolean);
      if (children.length === 0) return null;
      return { ...node, data: children };
    }
    return node;
  }

  const cleanedRoot = cleanNode(layout.grid.root);
  if (!cleanedRoot) {
    debug("sanitizeSerializedLayout: cleanedRoot is null after stripping, returning null");
    return null;
  }

  return {
    ...layout,
    grid: { ...layout.grid, root: cleanedRoot },
    panels: validPanels,
  };
}
/* eslint-enable @typescript-eslint/no-explicit-any */

/**
 * Filter a live `LayoutState` to renderable components and, when supplied, the
 * session IDs that belong to the target environment. Drop only groups and
 * columns emptied by the filter; preserve pre-existing empty groups and
 * `rootOrientation`, and repair a dangling `activePanel`.
 */
export function filterLayoutStateByComponents(
  state: LayoutState,
  validComponents: ReadonlySet<string> = RENDERABLE_COMPONENTS,
  validSessionIds?: ReadonlySet<string>,
): LayoutState {
  const columns = state.columns
    .map((column) => filterColumn(column, validComponents, validSessionIds))
    .filter((column): column is LayoutColumn => column !== null);
  return { ...state, columns };
}

function keepLivePanel(
  panel: LayoutPanel,
  validComponents: ReadonlySet<string>,
  validSessionIds?: ReadonlySet<string>,
): boolean {
  if (!validComponents.has(panel.component)) return false;
  if (!validSessionIds || !panel.id.startsWith("session:")) return true;
  return validSessionIds.has(panel.id.slice("session:".length));
}

function filterGroup(
  group: LayoutGroup,
  validComponents: ReadonlySet<string>,
  validSessionIds?: ReadonlySet<string>,
): LayoutGroup | null {
  const hadPanels = group.panels.length > 0;
  const panels: LayoutPanel[] = [];
  const droppedPanelIds = new Set<string>();
  for (const panel of group.panels) {
    if (keepLivePanel(panel, validComponents, validSessionIds)) panels.push(panel);
    else droppedPanelIds.add(panel.id);
  }
  // Only a group this filter emptied is dropped; a pre-existing empty group is
  // live state the rest of the app preserves.
  if (hadPanels && panels.length === 0) return null;
  const activePanelDropped =
    group.activePanel !== undefined && droppedPanelIds.has(group.activePanel);
  return {
    ...group,
    panels,
    activePanel: activePanelDropped ? panels[0]?.id : group.activePanel,
  };
}
function filterTreeNode(
  node: LayoutNode,
  validComponents: ReadonlySet<string>,
  validSessionIds?: ReadonlySet<string>,
): LayoutNode | null {
  if (node.type === "leaf") {
    const group = filterGroup(node.group, validComponents, validSessionIds);
    return group ? { ...node, group } : null;
  }
  const children = node.children
    .map((child) => filterTreeNode(child, validComponents, validSessionIds))
    .filter((child): child is LayoutNode => child !== null);
  if (children.length === 0) return null;
  return { ...node, children };
}
function filterColumn(
  column: LayoutColumn,
  validComponents: ReadonlySet<string>,
  validSessionIds?: ReadonlySet<string>,
): LayoutColumn | null {
  if (column.tree) {
    const tree = filterTreeNode(column.tree, validComponents, validSessionIds);
    if (!tree) return null;
    return { ...column, tree, groups: groupsInNode(tree) };
  }

  if (!Array.isArray(column.groups) || column.groups.length === 0) return column;

  const groups = column.groups
    .map((group) => filterGroup(group, validComponents, validSessionIds))
    .filter((group): group is LayoutGroup => group !== null);
  if (groups.length === 0) return null;
  return { ...column, groups };
}

function groupsInNode(node: LayoutNode): LayoutGroup[] {
  return node.type === "leaf" ? [node.group] : node.children.flatMap(groupsInNode);
}
