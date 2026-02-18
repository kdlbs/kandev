import type { DockviewApi, SerializedDockview } from 'dockview-react';
import type { LayoutState, LayoutColumn, LayoutGroup, LayoutPanel, LayoutNode } from './types';
import { computeColumnWidths, computeGroupHeights } from './sizing';
import { SIDEBAR_LOCK, KNOWN_PANEL_IDS, STRUCTURAL_COMPONENTS, LAYOUT_SIDEBAR_MAX_PX } from './constants';

// Dockview serialized grid node types (internal format)
type SerializedLeafNode = {
  type: 'leaf';
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
  type: 'branch';
  data: SerializedGridNode[];
  size: number;
};

type SerializedGridNode = SerializedLeafNode | SerializedBranchNode;

type SerializationCtx = { counter: number };

function nextGroupId(ctx: SerializationCtx): string {
  return `group-${++ctx.counter}`;
}

// ─── Serialization: LayoutState → SerializedDockview ───────────────────────

function serializeGroup(
  group: LayoutGroup,
  columnId: string,
  size: number,
  ctx: SerializationCtx,
): SerializedLeafNode {
  const groupId = nextGroupId(ctx);
  const views = group.panels.map((p) => p.id);
  const isSidebar = columnId === 'sidebar';
  return {
    type: 'leaf',
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
  if (node.type === 'leaf') {
    return serializeGroup(node.group, columnId, nodeSize, ctx);
  }

  // Branch: distribute orthoSize among children proportionally
  const totalCaptured = node.children.reduce((s, c) => s + (c.size ?? 0), 0);
  const children = node.children.map((child) => {
    const childSize = totalCaptured > 0
      ? Math.round((child.size ?? 0) / totalCaptured * orthoSize)
      : Math.floor(orthoSize / node.children.length);
    // Recurse: child's orthoSize = this node's nodeSize (orientation alternates)
    return serializeTreeNode(child, childSize, nodeSize, columnId, ctx);
  });

  return { type: 'branch', data: children, size: nodeSize };
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
    if (column.tree.type === 'leaf') {
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
  return { type: 'branch', data: children, size: width };
}

function serializePanels(
  state: LayoutState,
): Record<string, { id: string; contentComponent: string; title: string; tabComponent?: string; params?: Record<string, unknown> }> {
  const panels: Record<string, {
    id: string;
    contentComponent: string;
    title: string;
    tabComponent?: string;
    params?: Record<string, unknown>;
  }> = {};

  for (const column of state.columns) {
    for (const group of column.groups) {
      for (const panel of group.panels) {
        panels[panel.id] = {
          id: panel.id,
          contentComponent: panel.component,
          title: panel.title,
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
  const ctx: SerializationCtx = { counter: 0 };
  const widths = computeColumnWidths(state.columns, totalWidth, pinnedWidths);

  const root: SerializedBranchNode = {
    type: 'branch',
    data: state.columns.map((col, i) => serializeColumn(col, widths[i], totalHeight, ctx)),
    size: totalHeight,
  };

  return {
    grid: {
      root,
      width: totalWidth,
      height: totalHeight,
      orientation: 'HORIZONTAL',
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
    title: panel.title ?? panel.id,
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

  return { panels, activePanel: group.activePanel?.id };
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
    return { type: 'branch', children, size };
  }

  // LeafNode: extract group
  const group = groupFromGridNode(node) ?? { panels: [] };
  if (group.panels.length > 0) flatGroups.push(group);
  return { type: 'leaf', group, size };
}

/** Determine column ID and pinned status from its groups. */
function inferColumnMeta(
  groups: LayoutGroup[],
  index: number,
): { columnId: string; isPinned: boolean } {
  if (groups.length === 0) return { columnId: `col-${index}`, isPinned: false };

  const hasSidebar = groups[0].panels.some((p) => p.id === 'sidebar');
  if (hasSidebar) return { columnId: 'sidebar', isPinned: true };

  const firstPanelId = groups[0].panels[0]?.id;
  if (firstPanelId === 'chat') return { columnId: 'center', isPinned: false };
  if (firstPanelId) return { columnId: firstPanelId, isPinned: false };

  return { columnId: `col-${index}`, isPinned: false };
}

/**
 * Walk the dockview grid tree and map back to LayoutState.
 * Captures both the recursive tree structure (for faithful restoration)
 * and a flat groups list (for filtering and column identification).
 */
export function fromDockviewApi(api: DockviewApi): LayoutState {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const sv = (api as any).component?.gridview?.root?.splitview;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const rootChildren = (api as any).component?.gridview?.root?.children;

  if (!rootChildren || !sv) {
    console.warn('fromDockviewApi: no root splitview or children found');
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
      ...(isPinned ? { pinned: true, maxWidth: LAYOUT_SIDEBAR_MAX_PX } : {}),
      width,
      groups: flatGroups,
      // Only store tree for columns with nested structure (branches)
      ...(child.children ? { tree } : {}),
    });
  }

  return { columns };
}

// ─── Ephemeral Filtering ───────────────────────────────────────────────────

/** Check if a panel should be kept (known ID or structural component). */
function isStructuralPanel(p: LayoutPanel): boolean {
  return KNOWN_PANEL_IDS.has(p.id) || STRUCTURAL_COMPONENTS.has(p.component);
}

/**
 * Normalize dynamic panel IDs so they get fresh sessions on restore.
 */
function normalizePanel(p: LayoutPanel, counters: { terminal: number; browser: number }): LayoutPanel {
  if (KNOWN_PANEL_IDS.has(p.id)) return p;

  if (p.component === 'terminal') {
    const id = `terminal-saved-${++counters.terminal}`;
    return { ...p, id, params: { terminalId: id } };
  }
  if (p.component === 'browser') {
    const url = (p.params?.url as string) ?? '';
    const id = url ? `browser:${url}` : `browser-saved-${++counters.browser}`;
    return { ...p, id, params: { url } };
  }
  return p;
}

function filterGroup(
  group: LayoutGroup,
  counters: { terminal: number; browser: number },
): LayoutGroup {
  const kept = group.panels
    .filter(isStructuralPanel)
    .map((p) => normalizePanel(p, counters));
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
  if (node.type === 'leaf') {
    return { type: 'leaf', group: filterGroup(node.group, counters), size: node.size };
  }
  const children = node.children.map((c) => filterTreeNode(c, counters));
  return { type: 'branch', children, size: node.size };
}

/** Collect all groups from a tree (flat list for panels record). */
function collectGroupsFromTree(node: LayoutNode): LayoutGroup[] {
  if (node.type === 'leaf') return [node.group];
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
