import type {
  LayoutColumn,
  LayoutGroup,
  LayoutNode,
  LayoutOrientation,
  LayoutPanel,
  LayoutState,
} from "./layout-manager/types";

export const HIDDEN_RIGHT_PANE_METADATA_KEY = "kandevHiddenRightPane";
const HIDDEN_RIGHT_PANE_VERSION = 1 as const;

export type HiddenRightPane = {
  version: typeof HIDDEN_RIGHT_PANE_VERSION;
  sourceIndex: number;
  sourceColumnId: string;
  rootOrientation: LayoutOrientation;
  column: LayoutColumn;
  context: {
    columnIds: string[];
    panelIds: string[];
  };
};

export type RightPaneToggleState = {
  available: boolean;
  visible: boolean;
  hidden: boolean;
};

type PaneSelection = {
  index: number;
  column: LayoutColumn;
};

function groupsInColumn(column: LayoutColumn): LayoutGroup[] {
  if (!column.tree) return column.groups;
  return collectGroups(column.tree);
}

function collectGroups(node: LayoutNode): LayoutGroup[] {
  return node.type === "leaf" ? [node.group] : node.children.flatMap(collectGroups);
}

function panelIdsInColumn(column: LayoutColumn): string[] {
  return groupsInColumn(column).flatMap((group) => group.panels.map((panel) => panel.id));
}

function panelIdsInLayout(layout: LayoutState): string[] {
  return layout.columns.flatMap(panelIdsInColumn);
}

function hasAgentPanel(column: LayoutColumn): boolean {
  return panelIdsInColumn(column).some(
    (id) => id === "chat" || id.startsWith("session:") || id === "agent",
  );
}

function hasPanels(column: LayoutColumn): boolean {
  return panelIdsInColumn(column).length > 0;
}

function isHorizontal(layout: LayoutState): boolean {
  return (layout.rootOrientation ?? "HORIZONTAL") === "HORIZONTAL";
}

function workbenchColumnEntries(layout: LayoutState): Array<PaneSelection> {
  return layout.columns.flatMap((column, index) =>
    column.id === "sidebar" ? [] : [{ index, column }],
  );
}

/** Select the actual final side-by-side workbench region. */
export function selectRightPane(layout: LayoutState): PaneSelection | null {
  if (!isHorizontal(layout)) return null;
  const columns = workbenchColumnEntries(layout);
  if (columns.length < 2) return null;
  const target = columns.at(-1);
  if (!target || !hasPanels(target.column) || hasAgentPanel(target.column)) return null;
  return target;
}

function makeContext(layout: LayoutState, sourceIndex: number): HiddenRightPane["context"] {
  const remaining = layout.columns.filter((_, index) => index !== sourceIndex);
  return {
    columnIds: remaining.filter((column) => column.id !== "sidebar").map((column) => column.id),
    panelIds: panelIdsInLayout({ ...layout, columns: remaining }).sort(),
  };
}

/** Capture the selected pane and the layout left after hiding it. */
export function captureRightPane(
  layout: LayoutState,
): { layout: LayoutState; hiddenRightPane: HiddenRightPane } | null {
  const selection = selectRightPane(layout);
  if (!selection) return null;
  const { index, column } = selection;
  return {
    layout: { ...layout, columns: layout.columns.filter((_, i) => i !== index) },
    hiddenRightPane: {
      version: HIDDEN_RIGHT_PANE_VERSION,
      sourceIndex: index,
      sourceColumnId: column.id,
      rootOrientation: layout.rootOrientation ?? "HORIZONTAL",
      column,
      context: makeContext(layout, index),
    },
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isLayoutPanel(value: unknown): value is LayoutPanel {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === "string" &&
    value.id.length > 0 &&
    typeof value.component === "string" &&
    typeof value.title === "string"
  );
}

function isLayoutGroup(value: unknown): value is LayoutGroup {
  if (!isRecord(value) || !Array.isArray(value.panels)) return false;
  return value.panels.length > 0 && value.panels.every(isLayoutPanel);
}

function isLayoutNode(value: unknown): value is LayoutNode {
  if (!isRecord(value) || (value.type !== "leaf" && value.type !== "branch")) return false;
  if (value.type === "leaf") return isLayoutGroup(value.group);
  return (
    Array.isArray(value.children) && value.children.length > 0 && value.children.every(isLayoutNode)
  );
}

function isLayoutColumn(value: unknown): value is LayoutColumn {
  if (!isRecord(value) || typeof value.id !== "string") return false;
  if (!Array.isArray(value.groups) || !value.groups.every(isLayoutGroup)) return false;
  return value.tree === undefined || isLayoutNode(value.tree);
}

function hasUniquePanelIds(column: LayoutColumn): boolean {
  const ids = panelIdsInColumn(column);
  return new Set(ids).size === ids.length;
}

function isHiddenRightPane(value: unknown): value is HiddenRightPane {
  if (!isRecord(value)) return false;
  if (
    value.version !== HIDDEN_RIGHT_PANE_VERSION ||
    typeof value.sourceIndex !== "number" ||
    !Number.isInteger(value.sourceIndex) ||
    value.sourceIndex < 0 ||
    typeof value.sourceColumnId !== "string" ||
    (value.rootOrientation !== "HORIZONTAL" && value.rootOrientation !== "VERTICAL") ||
    !isLayoutColumn(value.column) ||
    !hasUniquePanelIds(value.column) ||
    !isRecord(value.context)
  ) {
    return false;
  }
  const context = value.context;
  return (
    Array.isArray(context.columnIds) &&
    context.columnIds.every((id) => typeof id === "string") &&
    Array.isArray(context.panelIds) &&
    context.panelIds.every((id) => typeof id === "string")
  );
}

/** Read and validate optional recovery metadata from an env layout record. */
export function readHiddenRightPane(record: object | null): HiddenRightPane | null {
  if (!isRecord(record)) return null;
  const value = record[HIDDEN_RIGHT_PANE_METADATA_KEY];
  return isHiddenRightPane(value) ? value : null;
}

/** Remove app-owned metadata before passing a record to Dockview. */
export function stripHiddenRightPaneMetadata(record: object): object {
  if (!isRecord(record) || !(HIDDEN_RIGHT_PANE_METADATA_KEY in record)) return record;
  const { [HIDDEN_RIGHT_PANE_METADATA_KEY]: _metadata, ...dockviewRecord } = record;
  return dockviewRecord;
}

/** Compose an env layout record without storing hidden recovery in portable layouts. */
export function withHiddenRightPaneMetadata(
  layout: object,
  hiddenRightPane: HiddenRightPane | null,
): object {
  const record = { ...(isRecord(layout) ? layout : {}) } as Record<string, unknown>;
  if (hiddenRightPane) record[HIDDEN_RIGHT_PANE_METADATA_KEY] = hiddenRightPane;
  else delete record[HIDDEN_RIGHT_PANE_METADATA_KEY];
  return record;
}

function hasContextOverlap(layout: LayoutState, hiddenRightPane: HiddenRightPane): boolean {
  const currentColumnIds = new Set(
    layout.columns.filter((column) => column.id !== "sidebar").map((column) => column.id),
  );
  const currentPanelIds = new Set(panelIdsInLayout(layout));
  return (
    hiddenRightPane.context.columnIds.length === 0 ||
    hiddenRightPane.context.columnIds.some((id) => currentColumnIds.has(id)) ||
    hiddenRightPane.context.panelIds.some((id) => currentPanelIds.has(id))
  );
}

function isCompatibleWithCurrentLayout(
  layout: LayoutState,
  hiddenRightPane: HiddenRightPane,
): boolean {
  if (!isHorizontal(layout)) return false;
  if ((layout.rootOrientation ?? "HORIZONTAL") !== hiddenRightPane.rootOrientation) return false;
  if (workbenchColumnEntries(layout).length === 0) return false;
  return hasContextOverlap(layout, hiddenRightPane);
}

function isPaneFullyPresent(layout: LayoutState, hiddenRightPane: HiddenRightPane): boolean {
  const currentPanelIds = new Set(panelIdsInLayout(layout));
  const hiddenPanelIds = panelIdsInColumn(hiddenRightPane.column);
  return hiddenPanelIds.length > 0 && hiddenPanelIds.every((id) => currentPanelIds.has(id));
}

function filterGroup(group: LayoutGroup, usedPanelIds: Set<string>): LayoutGroup | null {
  const panels = group.panels.filter((panel) => {
    if (usedPanelIds.has(panel.id)) return false;
    usedPanelIds.add(panel.id);
    return true;
  });
  if (panels.length === 0) return null;
  const activePanel = panels.some((panel) => panel.id === group.activePanel)
    ? group.activePanel
    : panels[0].id;
  return { ...group, panels, activePanel };
}

function filterNode(node: LayoutNode, usedPanelIds: Set<string>): LayoutNode | null {
  if (node.type === "leaf") {
    const group = filterGroup(node.group, usedPanelIds);
    return group ? { ...node, group } : null;
  }
  const children = node.children
    .map((child) => filterNode(child, usedPanelIds))
    .filter(Boolean) as LayoutNode[];
  if (children.length === 0) return null;
  if (children.length === 1) return children[0];
  return { ...node, children };
}

function filterColumn(column: LayoutColumn, usedPanelIds: Set<string>): LayoutColumn | null {
  if (column.tree) {
    const tree = filterNode(column.tree, usedPanelIds);
    if (!tree) return null;
    return { ...column, tree, groups: collectGroups(tree) };
  }
  const groups = column.groups
    .map((group) => filterGroup(group, usedPanelIds))
    .filter(Boolean) as LayoutGroup[];
  return groups.length > 0 ? { ...column, groups } : null;
}

/** Restore a retained pane into the current layout, preserving live edits. */
export function restoreRightPane(
  layout: LayoutState,
  hiddenRightPane: HiddenRightPane,
): LayoutState | null {
  if (
    !isHiddenRightPane(hiddenRightPane) ||
    !isCompatibleWithCurrentLayout(layout, hiddenRightPane)
  ) {
    return null;
  }
  const usedPanelIds = new Set(panelIdsInLayout(layout));
  const column = filterColumn(hiddenRightPane.column, usedPanelIds);
  if (!column) return null;

  const columns = [...layout.columns];
  const insertionIndex = Math.min(hiddenRightPane.sourceIndex, columns.length);
  columns.splice(insertionIndex, 0, column);
  return { ...layout, columns };
}

/** Derive the control state. A retained pane always wins over a new target. */
export function getRightPaneToggleState(
  layout: LayoutState,
  hiddenRightPane: HiddenRightPane | null,
): RightPaneToggleState {
  if (
    hiddenRightPane &&
    isCompatibleWithCurrentLayout(layout, hiddenRightPane) &&
    !isPaneFullyPresent(layout, hiddenRightPane)
  ) {
    return { available: true, visible: false, hidden: true };
  }
  const selection = selectRightPane(layout);
  return selection
    ? { available: true, visible: true, hidden: false }
    : { available: false, visible: false, hidden: false };
}
