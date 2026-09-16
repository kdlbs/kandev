import type {
  LayoutColumn,
  LayoutGroup,
  LayoutNode,
  LayoutOrientation,
  LayoutPanel,
  LayoutState,
} from "./layout-manager/types";
import { LAYOUT_PINNED_MIN_PX } from "./layout-manager/caps";
import { getPinnedWidth } from "./layout-manager/sizing";

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

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function isFiniteNonNegative(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function isSerializableValue(value: unknown, ancestors = new Set<object>()): boolean {
  if (value === null || typeof value === "string" || typeof value === "boolean") return true;
  if (typeof value === "number") return Number.isFinite(value);
  if (typeof value !== "object") return false;
  if (ancestors.has(value)) return false;

  ancestors.add(value);
  const valid = Array.isArray(value)
    ? value.every((item) => isSerializableValue(item, ancestors))
    : Object.values(value).every((item) => isSerializableValue(item, ancestors));
  ancestors.delete(value);
  return valid;
}

function isLayoutPanel(value: unknown): value is LayoutPanel {
  if (!isRecord(value)) return false;
  return (
    isNonEmptyString(value.id) &&
    isNonEmptyString(value.component) &&
    typeof value.title === "string" &&
    (value.tabComponent === undefined || isNonEmptyString(value.tabComponent)) &&
    (value.params === undefined || (isRecord(value.params) && isSerializableValue(value.params)))
  );
}

function isLayoutGroup(value: unknown): value is LayoutGroup {
  if (!isRecord(value) || !Array.isArray(value.panels)) return false;
  if (
    value.panels.length === 0 ||
    !value.panels.every(isLayoutPanel) ||
    (value.id !== undefined && !isNonEmptyString(value.id)) ||
    (value.activePanel !== undefined && !isNonEmptyString(value.activePanel))
  ) {
    return false;
  }
  return (
    value.activePanel === undefined || value.panels.some((panel) => panel.id === value.activePanel)
  );
}

function isLayoutNode(value: unknown): value is LayoutNode {
  if (!isRecord(value) || (value.type !== "leaf" && value.type !== "branch")) return false;
  if (value.size !== undefined && !isFiniteNonNegative(value.size)) return false;
  if (value.type === "leaf") return isLayoutGroup(value.group);
  return (
    Array.isArray(value.children) && value.children.length > 0 && value.children.every(isLayoutNode)
  );
}

function isOptionalNonNegative(value: unknown): boolean {
  return value === undefined || isFiniteNonNegative(value);
}

function isOptionalPositive(value: unknown): boolean {
  return value === undefined || (isFiniteNonNegative(value) && value > 0);
}

function hasValidColumnGeometry(value: Record<string, unknown>): boolean {
  if (!isOptionalNonNegative(value.width)) return false;
  if (!isOptionalPositive(value.minWidth) || !isOptionalPositive(value.maxWidth)) return false;
  if (typeof value.minWidth !== "number" || typeof value.maxWidth !== "number") return true;
  return value.maxWidth >= value.minWidth;
}

function isLayoutColumn(value: unknown): value is LayoutColumn {
  if (!isRecord(value) || !isNonEmptyString(value.id)) return false;
  if (value.pinned !== undefined && typeof value.pinned !== "boolean") return false;
  if (!hasValidColumnGeometry(value)) return false;
  if (!Array.isArray(value.groups) || !value.groups.every(isLayoutGroup)) return false;
  if (value.tree !== undefined && !isLayoutNode(value.tree)) return false;
  if (value.tree === undefined) return true;

  const treeGroups = collectGroups(value.tree);
  const groups = value.groups as LayoutGroup[];
  return (
    treeGroups.length === groups.length &&
    treeGroups.every((group, index) => sameLayoutGroup(group, groups[index]))
  );
}

function sameSerializableValue(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  if (Array.isArray(left) || Array.isArray(right)) {
    return (
      Array.isArray(left) &&
      Array.isArray(right) &&
      left.length === right.length &&
      left.every((item, index) => sameSerializableValue(item, right[index]))
    );
  }
  if (isRecord(left) || isRecord(right)) {
    if (!isRecord(left) || !isRecord(right)) return false;
    const leftKeys = Object.keys(left);
    const rightKeys = Object.keys(right);
    return (
      leftKeys.length === rightKeys.length &&
      leftKeys.every((key) => key in right && sameSerializableValue(left[key], right[key]))
    );
  }
  return false;
}

function sameLayoutPanel(left: LayoutPanel, right: LayoutPanel): boolean {
  return (
    left.id === right.id &&
    left.component === right.component &&
    left.title === right.title &&
    left.tabComponent === right.tabComponent &&
    sameSerializableValue(left.params, right.params)
  );
}

function sameLayoutGroup(left: LayoutGroup, right: LayoutGroup): boolean {
  return (
    left.id === right.id &&
    left.activePanel === right.activePanel &&
    left.panels.length === right.panels.length &&
    left.panels.every((panel, index) => sameLayoutPanel(panel, right.panels[index]))
  );
}

function hasUniquePanelIds(column: LayoutColumn): boolean {
  const ids = panelIdsInColumn(column);
  return new Set(ids).size === ids.length;
}

function hasUniqueGroupIds(column: LayoutColumn): boolean {
  const ids = groupsInColumn(column)
    .map((group) => group.id)
    .filter((id): id is string => id !== undefined);
  return new Set(ids).size === ids.length;
}

function isValidLayoutState(value: unknown): value is LayoutState {
  if (!isRecord(value) || !Array.isArray(value.columns)) return false;
  if (
    value.rootOrientation !== undefined &&
    value.rootOrientation !== "HORIZONTAL" &&
    value.rootOrientation !== "VERTICAL"
  ) {
    return false;
  }
  const columns = value.columns as unknown[];
  if (!columns.every(isLayoutColumn)) return false;
  const validColumns = columns as LayoutColumn[];

  const columnIds = validColumns.map((column) => column.id);
  if (new Set(columnIds).size !== columnIds.length) return false;
  if (!validColumns.every(hasUniqueGroupIds)) return false;
  const groupIds = validColumns
    .flatMap(groupsInColumn)
    .map((group) => group.id)
    .filter((id): id is string => id !== undefined);
  if (new Set(groupIds).size !== groupIds.length) return false;
  const panelIds = panelIdsInLayout({ columns: validColumns });
  return new Set(panelIds).size === panelIds.length;
}

function isUniqueNonEmptyStringList(value: unknown): value is string[] {
  return (
    Array.isArray(value) && value.length === new Set(value).size && value.every(isNonEmptyString)
  );
}

function isValidHiddenContext(value: unknown): value is HiddenRightPane["context"] {
  return (
    isRecord(value) &&
    isUniqueNonEmptyStringList(value.columnIds) &&
    isUniqueNonEmptyStringList(value.panelIds)
  );
}

function isHiddenRightPane(value: unknown): value is HiddenRightPane {
  if (!isRecord(value)) return false;
  if (
    value.version !== HIDDEN_RIGHT_PANE_VERSION ||
    typeof value.sourceIndex !== "number" ||
    !Number.isInteger(value.sourceIndex) ||
    value.sourceIndex < 0 ||
    !isNonEmptyString(value.sourceColumnId) ||
    (value.rootOrientation !== "HORIZONTAL" && value.rootOrientation !== "VERTICAL")
  ) {
    return false;
  }
  if (!isLayoutColumn(value.column)) return false;
  if (!hasUniquePanelIds(value.column) || !hasUniqueGroupIds(value.column)) return false;
  if (value.sourceColumnId !== value.column.id) return false;
  return isValidHiddenContext(value.context);
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
  if (!isValidLayoutState(layout) || !isHiddenRightPane(hiddenRightPane)) return false;
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

function rebalanceRestoredFlexWidths(
  columns: LayoutColumn[],
  restoredColumnId: string,
  options: { totalWidth?: number; pinnedWidths?: ReadonlyMap<string, number> },
): LayoutColumn[] {
  const { totalWidth, pinnedWidths } = options;
  if (typeof totalWidth !== "number" || !Number.isFinite(totalWidth) || totalWidth <= 0) {
    return columns;
  }

  const restoredColumn = columns.find((column) => column.id === restoredColumnId);
  if (!restoredColumn || restoredColumn.pinned || restoredColumn.width === undefined) {
    return columns;
  }

  const pinnedTotal = columns.reduce(
    (total, column) =>
      total +
      (column.pinned ? getPinnedWidth(column, totalWidth, pinnedWidths?.get(column.id)) : 0),
    0,
  );
  const availableFlex = Math.floor(totalWidth - pinnedTotal);
  const flexColumns = columns.filter((column) => !column.pinned);
  const remainingFlex = flexColumns.filter((column) => column.id !== restoredColumnId);
  if (availableFlex <= 0 || remainingFlex.length === 0) {
    return availableFlex > 0
      ? columns.map((column) =>
          column.id === restoredColumnId ? { ...column, width: availableFlex } : column,
        )
      : columns;
  }

  const minimum = Math.max(
    1,
    Math.min(LAYOUT_PINNED_MIN_PX, Math.floor(availableFlex / flexColumns.length)),
  );
  const targetWidth = Math.max(
    minimum,
    Math.min(restoredColumn.width, availableFlex - minimum * remainingFlex.length),
  );
  const remainingWidth = availableFlex - targetWidth;
  const weights = remainingFlex.map((column) =>
    typeof column.width === "number" && Number.isFinite(column.width) && column.width > 0
      ? column.width
      : 1,
  );
  const totalWeight = weights.reduce((total, width) => total + width, 0);
  let allocated = 0;
  let remainingIndex = 0;

  return columns.map((column) => {
    if (column.id === restoredColumnId) return { ...column, width: targetWidth };
    if (column.pinned) return column;

    const weight = weights[remainingIndex] ?? 0;
    const isLast = remainingIndex === remainingFlex.length - 1;
    const width = isLast
      ? remainingWidth - allocated
      : Math.round((remainingWidth * weight) / totalWeight);
    allocated += width;
    remainingIndex++;
    return { ...column, width };
  });
}

/** Restore a retained pane into the current layout, preserving live edits. */
export function restoreRightPane(
  layout: LayoutState,
  hiddenRightPane: HiddenRightPane,
  options: {
    totalWidth?: number;
    pinnedWidths?: ReadonlyMap<string, number>;
  } = {},
): LayoutState | null {
  if (
    !isHiddenRightPane(hiddenRightPane) ||
    !isCompatibleWithCurrentLayout(layout, hiddenRightPane)
  ) {
    return null;
  }
  if (layout.columns.some((column) => column.id === hiddenRightPane.sourceColumnId)) {
    return null;
  }
  const usedPanelIds = new Set(panelIdsInLayout(layout));
  const column = filterColumn(hiddenRightPane.column, usedPanelIds);
  if (!column) return null;

  const columns = [...layout.columns];
  const insertionIndex = Math.min(hiddenRightPane.sourceIndex, columns.length);
  columns.splice(insertionIndex, 0, column);
  const restored = {
    ...layout,
    columns: rebalanceRestoredFlexWidths(columns, column.id, options),
  };
  return isValidLayoutState(restored) ? restored : null;
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
