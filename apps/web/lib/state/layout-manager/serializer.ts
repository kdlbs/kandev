import type { DockviewApi, SerializedDockview } from "dockview-react";
import type {
  LayoutState,
  LayoutColumn,
  LayoutGroup,
  LayoutPanel,
  LayoutNode,
  LayoutOrientation,
} from "./types";
import { computeColumnWidths, computeGroupHeights } from "./sizing";
import {
  SIDEBAR_LOCK,
  DEV_SERVER_PANEL_ID,
  KNOWN_PANEL_IDS,
  STRUCTURAL_COMPONENTS,
  TERMINAL_DEFAULT_ID,
  canonicalPanelTitle,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
} from "./constants";
import { panelTitle } from "./panel-title";

/** Canonical English, because `normalizePanel` produces a stored LayoutState. */
// i18n-exempt: canonical English persisted in saved layouts, compared when restoring.
const TERMINAL_CANONICAL_TITLE = canonicalPanelTitle(TERMINAL_DEFAULT_ID) ?? "Terminal";

// Dockview serialized grid node types (internal format)
type SerializedLeafNode = {
  type: "leaf";
  data: {
    id: string;
    views: string[];
    activeView: string;
    hideHeader?: boolean;
    locked?: string;
  };
  size: number;
};

type SerializedBranchNode = {
  type: "branch";
  data: SerializedGridNode[];
  size: number;
};

type SerializedGridNode = SerializedLeafNode | SerializedBranchNode;

type SerializationCtx = { counter: number; usedGroupIds: Set<string> };

function nextGroupId(ctx: SerializationCtx): string {
  let groupId: string;
  do {
    groupId = `group-${++ctx.counter}`;
  } while (ctx.usedGroupIds.has(groupId));
  ctx.usedGroupIds.add(groupId);
  return groupId;
}

// ─── Serialization: LayoutState → SerializedDockview ───────────────────────

function serializeGroup(
  group: LayoutGroup,
  columnId: string,
  size: number,
  ctx: SerializationCtx,
): SerializedLeafNode {
  const groupId = group.id ?? nextGroupId(ctx);
  if (group.id) ctx.usedGroupIds.add(group.id);
  const views = group.panels.map((p) => p.id);
  const isSidebar = columnId === "sidebar";
  return {
    type: "leaf",
    data: {
      id: groupId,
      views,
      activeView: group.activePanel ?? views[0],
      ...(isSidebar ? { locked: SIDEBAR_LOCK } : {}),
    },
    size,
  };
}

/**
 * Serialize a LayoutNode tree into a dockview grid node.
 * @param nodeSize   - this node's size in the parent's split direction
 * @param orthoSize  - the orthogonal dimension (becomes children's available space)
 */
function serializeTreeNode(
  node: LayoutNode,
  nodeSize: number,
  orthoSize: number,
  columnId: string,
  ctx: SerializationCtx,
): SerializedGridNode {
  if (node.type === "leaf") {
    return serializeGroup(node.group, columnId, nodeSize, ctx);
  }

  // Branch: distribute orthoSize among children proportionally
  const totalCaptured = node.children.reduce((s, c) => s + (c.size ?? 0), 0);
  const children = node.children.map((child) => {
    const childSize =
      totalCaptured > 0
        ? Math.round(((child.size ?? 0) / totalCaptured) * orthoSize)
        : Math.floor(orthoSize / node.children.length);
    // Recurse: child's orthoSize = this node's nodeSize (orientation alternates)
    return serializeTreeNode(child, childSize, nodeSize, columnId, ctx);
  });

  return { type: "branch", data: children, size: nodeSize };
}

/** Serialize a column, using tree if available, otherwise flat groups. */
function serializeColumn(
  column: LayoutColumn,
  width: number,
  totalHeight: number,
  ctx: SerializationCtx,
): SerializedGridNode {
  // Tree-based serialization (preserves nested splits)
  if (column.tree) {
    if (column.tree.type === "leaf") {
      const leaf = serializeGroup(column.tree.group, column.id, totalHeight, ctx);
      return { ...leaf, size: width };
    }
    return serializeTreeNode(column.tree, width, totalHeight, column.id, ctx);
  }

  // Flat groups fallback (presets, backward compat)
  const groups = column.groups;
  if (groups.length === 1) {
    const leaf = serializeGroup(groups[0], column.id, totalHeight, ctx);
    return { ...leaf, size: width };
  }
  const heights = computeGroupHeights(groups, totalHeight);
  const children = groups.map((g, i) => serializeGroup(g, column.id, heights[i], ctx));
  return { type: "branch", data: children, size: width };
}

function serializePanels(state: LayoutState): Record<
  string,
  {
    id: string;
    contentComponent: string;
    title: string;
    tabComponent?: string;
    params?: Record<string, unknown>;
  }
> {
  const panels: Record<
    string,
    {
      id: string;
      contentComponent: string;
      title: string;
      tabComponent?: string;
      params?: Record<string, unknown>;
    }
  > = {};

  for (const column of state.columns) {
    for (const group of column.groups) {
      for (const panel of group.panels) {
        panels[panel.id] = {
          id: panel.id,
          contentComponent: panel.component,
          // RENDER direction: this feeds `api.fromJSON`, so the tab the user
          // reads is resolved here, in the active locale. `LayoutState` itself
          // keeps the canonical English title — see `panelFromDockviewPanel`,
          // which canonicalizes again on capture so a translated title can
          // never reach storage.
          title: panelTitle(panel.id, panel.title, panel.component),
          ...(panel.tabComponent ? { tabComponent: panel.tabComponent } : {}),
          ...(panel.params ? { params: panel.params } : {}),
        };
      }
    }
  }
  return panels;
}

/**
 * Convert a LayoutState to dockview's SerializedDockview format.
 */
export function toSerializedDockview(
  state: LayoutState,
  totalWidth: number,
  totalHeight: number,
  pinnedWidths: Map<string, number>,
): SerializedDockview {
  const ctx: SerializationCtx = {
    counter: 0,
    usedGroupIds: new Set(
      state.columns.flatMap((column) =>
        column.groups.flatMap((group) => (group.id ? [group.id] : [])),
      ),
    ),
  };
  const rootOrientation = state.rootOrientation ?? "HORIZONTAL";
  const widths = computeColumnWidths(state.columns, totalWidth, pinnedWidths);

  const root: SerializedBranchNode = {
    type: "branch",
    data: state.columns.map((col, i) => serializeColumn(col, widths[i], totalHeight, ctx)),
    size: totalHeight,
  };

  return {
    grid: {
      root,
      width: totalWidth,
      height: totalHeight,
      orientation: rootOrientation,
    },
    panels: serializePanels(state),
    activeGroup: undefined,
  } as unknown as SerializedDockview;
}

// ─── Deserialization: DockviewApi → LayoutState ────────────────────────────

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function panelFromDockviewPanel(panel: any): LayoutPanel {
  return {
    id: panel.id,
    component: panel.view?.contentComponent ?? panel.id,
    // CAPTURE direction: dockview holds the localized title, and this result is
    // what gets persisted, so a registry panel goes back to canonical English.
    title: canonicalPanelTitle(panel.id, panel.view?.contentComponent) ?? panel.title ?? panel.id,
    ...(panel.view?.tabComponent ? { tabComponent: panel.view.tabComponent } : {}),
    ...(panel.params ? { params: panel.params as Record<string, unknown> } : {}),
  };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function groupFromGridNode(node: any): LayoutGroup | null {
  // Dockview v4.x: leaf nodes expose the group via node.view (DockviewGroupPanel)
  const group = node?.view ?? node?.element?._group ?? node?._group;
  if (!group || !group.panels) return null;

  const panels: LayoutPanel[] = (group.panels ?? []).map(panelFromDockviewPanel);
  if (panels.length === 0) return null;

  return { id: group.id, panels, activePanel: group.activePanel?.id };
}

/** Recursively capture a dockview grid node into a LayoutNode + flat groups. */
function captureNode(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  node: any,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  parentSv: any,
  index: number,
  flatGroups: LayoutGroup[],
): LayoutNode {
  const size = parentSv?.getViewSize?.(index) as number | undefined;

  if (node.children) {
    // BranchNode: recurse into children
    const childSv = node.splitview;
    const children: LayoutNode[] = [];
    for (let i = 0; i < node.children.length; i++) {
      children.push(captureNode(node.children[i], childSv, i, flatGroups));
    }
    return { type: "branch", children, size };
  }

  // LeafNode: extract group
  const group = groupFromGridNode(node) ?? { panels: [] };
  if (group.panels.length > 0) flatGroups.push(group);
  return { type: "leaf", group, size };
}

/** Panel IDs that indicate the "right" column when no structural identity exists. */
const RIGHT_PANEL_IDS = new Set(["files", "changes"]);

function hasCanonicalRightIdentity(column: Pick<LayoutColumn, "id" | "groups">): boolean {
  const groups = column.groups ?? [];
  return (
    column.id === "right" ||
    groups.some((group) => group.id === RIGHT_TOP_GROUP || group.id === RIGHT_BOTTOM_GROUP)
  );
}

/** True when the column contains the Agent surface or the canonical center group. */
export function isCenterColumn(column: Pick<LayoutColumn, "id" | "groups">): boolean {
  if (hasCanonicalRightIdentity(column)) return false;
  const groups = column.groups ?? [];
  return (
    column.id === "center" ||
    groups.some(
      (group) =>
        group.id === CENTER_GROUP ||
        group.panels.some(
          (panel) =>
            panel.component === "chat" || panel.id === "chat" || panel.id.startsWith("session:"),
        ),
    )
  );
}

/** True when the column owns the standard right-side groups or column ID. */
export function isRightColumn(column: Pick<LayoutColumn, "id" | "groups">): boolean {
  if (hasCanonicalRightIdentity(column)) return true;
  if (isCenterColumn(column)) return false;
  const groups = column.groups ?? [];
  return groups.some((group) => group.panels.some((panel) => RIGHT_PANEL_IDS.has(panel.id)));
}

/** Determine column ID and pinned status from its groups. */
function inferColumnMeta(
  groups: LayoutGroup[],
  index: number,
): { columnId: string; isPinned: boolean } {
  if (groups.length === 0) return { columnId: `col-${index}`, isPinned: false };

  const column = { id: `col-${index}`, groups };

  if (groups.some((group) => group.panels.some((panel) => panel.id === "sidebar"))) {
    return { columnId: "sidebar", isPinned: true };
  }
  if (isCenterColumn(column)) return { columnId: "center", isPinned: false };

  if (isRightColumn(column)) return { columnId: "right", isPinned: true };

  const firstPanelId = groups[0].panels[0]?.id;
  if (firstPanelId) return { columnId: firstPanelId, isPinned: false };

  return { columnId: `col-${index}`, isPinned: false };
}

function readRootOrientation(
  root: { orientation?: unknown } | null | undefined,
  splitview: { orientation?: unknown } | null | undefined,
): LayoutOrientation {
  return root?.orientation === "VERTICAL" || splitview?.orientation === "VERTICAL"
    ? "VERTICAL"
    : "HORIZONTAL";
}

/**
 * Walk the dockview grid tree and map back to LayoutState.
 * Captures both the recursive tree structure (for faithful restoration)
 * and a flat groups list (for filtering and column identification).
 */
export function fromDockviewApi(api: DockviewApi): LayoutState {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const root = (api as any).component?.gridview?.root;
  const sv = root?.splitview;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const rootChildren = root?.children;

  if (!rootChildren || !sv) {
    console.warn("fromDockviewApi: no root splitview or children found");
    return { columns: [] };
  }

  const columns: LayoutColumn[] = [];
  for (let i = 0; i < rootChildren.length; i++) {
    const child = rootChildren[i];
    const width = sv.getViewSize?.(i) ?? 0;
    const flatGroups: LayoutGroup[] = [];
    const tree = captureNode(child, sv, i, flatGroups);
    const { columnId, isPinned } = inferColumnMeta(flatGroups, i);

    columns.push({
      id: columnId,
      ...(isPinned ? { pinned: true } : {}),
      width,
      groups: flatGroups,
      // Only store tree for columns with nested structure (branches)
      ...(child.children ? { tree } : {}),
    });
  }

  return { columns, rootOrientation: readRootOrientation(root, sv) };
}

// ─── Ephemeral Filtering ───────────────────────────────────────────────────

/** Check if a panel should be kept (known ID or structural component). */
function isStructuralPanel(p: LayoutPanel): boolean {
  return KNOWN_PANEL_IDS.has(p.id) || STRUCTURAL_COMPONENTS.has(p.component);
}

/**
 * Normalize dynamic panel IDs so they get fresh sessions on restore.
 *
 * DB-backed ordinary terminals (id shape `shell-<uuid>`) are preserved
 * intact — the row survives in the backend across reloads and the panel
 * must keep its id + params (environmentId, taskID) so the WS reattach
 * lands on the live PTY and the store lookup finds the matching shell
 * record (so the badge renders).
 *
 * Tab presentation is also coerced for every terminal panel — the
 * registry was updated to use the custom `terminalTab` and a plain
 * `"Terminal"` title, but layouts persisted before that change carry
 * the old plain tab + `"Terminal {seq}"` title text and would render
 * wrong on restore.
 */
function isDbBackedTerminalId(id: string): boolean {
  // shell-<uuid> — 36-char hex+dashes after the prefix. Excludes the
  // legacy `shell-default` passthrough id and any `terminal-saved-*`
  // ids from older layouts.
  return /^shell-[0-9a-f-]{36}$/.test(id);
}

function normalizePanel(
  p: LayoutPanel,
  counters: { terminal: number; browser: number },
): LayoutPanel {
  if (p.component === "terminal") {
    // The dev-server panel shares the terminal component but is a read-only
    // view of the dev process output — it owns no PTY, so it must keep its
    // own id, params and title instead of being coerced into a shell tab.
    if (p.id === DEV_SERVER_PANEL_ID) {
      return {
        ...p,
        params: { type: DEV_SERVER_PANEL_ID },
        title: canonicalPanelTitle(p.id, p.component) ?? p.title,
      };
    }
    if (KNOWN_PANEL_IDS.has(p.id) || isDbBackedTerminalId(p.id)) {
      // Default panel (terminal-default) and user-created DB-backed
      // terminals (shell-<uuid>) — preserve id + params so the live PTY
      // and store record match on restore. Coerce presentation in case
      // the saved layout predates the custom tab + plain title.
      return { ...p, tabComponent: "terminalTab", title: TERMINAL_CANONICAL_TITLE };
    }
    // Legacy ephemeral terminal id (e.g. `terminal-saved-1` from older
    // layouts) — mint a fresh id so it reconnects to a new PTY.
    const id = `terminal-saved-${++counters.terminal}`;
    return {
      ...p,
      id,
      params: { terminalId: id },
      tabComponent: "terminalTab",
      title: TERMINAL_CANONICAL_TITLE,
    };
  }

  // A layout saved before this split (or by an older build) can carry a
  // translated title; normalise it back to canonical English so the render
  // direction is the only place a locale is applied.
  if (KNOWN_PANEL_IDS.has(p.id))
    return { ...p, title: canonicalPanelTitle(p.id, p.component) ?? p.title };

  if (p.component === "browser") {
    const url = (p.params?.url as string) ?? "";
    const id = url ? `browser:${url}` : `browser-saved-${++counters.browser}`;
    return { ...p, id, params: { url }, title: canonicalPanelTitle(id, p.component) ?? p.title };
  }
  return p;
}

function filterGroup(
  group: LayoutGroup,
  counters: { terminal: number; browser: number },
): LayoutGroup {
  const kept = group.panels.filter(isStructuralPanel).map((p) => normalizePanel(p, counters));
  if (kept.length === 0) return { panels: [], activePanel: undefined };
  const activeStillExists = group.activePanel && kept.some((p) => p.id === group.activePanel);
  return {
    panels: kept,
    activePanel: activeStillExists ? group.activePanel : kept[0].id,
  };
}

/** Recursively filter ephemeral panels from a layout tree node. */
function filterTreeNode(
  node: LayoutNode,
  counters: { terminal: number; browser: number },
): LayoutNode {
  if (node.type === "leaf") {
    return { type: "leaf", group: filterGroup(node.group, counters), size: node.size };
  }
  const children = node.children.map((c) => filterTreeNode(c, counters));
  return { type: "branch", children, size: node.size };
}

/** Collect all groups from a tree (flat list for panels record). */
function collectGroupsFromTree(node: LayoutNode): LayoutGroup[] {
  if (node.type === "leaf") return [node.group];
  return node.children.flatMap(collectGroupsFromTree);
}

/**
 * Filter out ephemeral panels (file editors, commit details, diff viewers).
 * Preserves tree structure and empty groups for split preservation.
 */
export function filterEphemeral(state: LayoutState): LayoutState {
  const counters = { terminal: 0, browser: 0 };
  const columns: LayoutColumn[] = [];

  for (const col of state.columns) {
    // Filter the tree if present
    const tree = col.tree ? filterTreeNode(col.tree, counters) : undefined;

    // Filter flat groups
    const groups = tree
      ? collectGroupsFromTree(tree)
      : col.groups.map((g) => filterGroup(g, counters));

    if (groups.length > 0) {
      columns.push({ ...col, groups, ...(tree ? { tree } : {}) });
    }
  }

  return { columns };
}
