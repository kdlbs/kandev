export type TableElementContext = {
  table: HTMLTableElement;
  wrapper: HTMLDivElement;
};

export type EdgeActionPosition = {
  index: number;
  left: number;
  top: number;
};

export type TableEdgePointerMode = "fine" | "coarse";

export type TableEdgeGeometry = {
  boundaries: number[];
  columnActions: EdgeActionPosition[];
  columnWidths: number[];
  height: number;
  layerHeight: number;
  layerWidth: number;
  resizeHeight: number;
  resizeTop: number;
  rowActions: EdgeActionPosition[];
  tableLeft: number;
  tableWidth: number;
  tableTop: number;
};

// These pixel values are shared with the CSS media-query lanes. Insertion and
// resize hitboxes occupy separate rows above the table so neither can cover a
// cell, including on coarse pointers where both controls are 44px square.
export const TABLE_EDGE_LAYOUT = {
  fine: { actionSize: 24, laneGap: 4, resizeHeight: 12 },
  coarse: { actionSize: 44, laneGap: 4, resizeHeight: 44 },
} as const;

export function getTableContext(host: HTMLDivElement | null): TableElementContext | null {
  const wrapper = host?.closest<HTMLDivElement>(".md-table-wrapper");
  const table = wrapper?.querySelector<HTMLTableElement>(".md-table.md-block-active");
  if (!wrapper || !table) return null;
  return { table, wrapper };
}

export function getVisibleRows(table: HTMLTableElement): HTMLTableRowElement[] {
  return Array.from(table.rows).filter((row) => !row.classList.contains("md-table-delimiter-row"));
}

export function readTableGeometry(
  context: TableElementContext,
  pointerMode: TableEdgePointerMode,
): TableEdgeGeometry | null {
  const { table, wrapper } = context;
  const visibleRows = getVisibleRows(table);
  const headerCells = Array.from(visibleRows[0]?.cells ?? []);
  const tableRect = table.getBoundingClientRect();
  const wrapperRect = wrapper.getBoundingClientRect();
  if (
    visibleRows.length === 0 ||
    headerCells.length === 0 ||
    tableRect.width <= 0 ||
    tableRect.height <= 0
  ) {
    return null;
  }

  const layout = TABLE_EDGE_LAYOUT[pointerMode];
  const scrollLeft = wrapper.scrollLeft;
  const scrollTop = wrapper.scrollTop;
  const tableLeft = tableRect.left - wrapperRect.left + scrollLeft;
  const tableTop = tableRect.top - wrapperRect.top + scrollTop;
  const cellRects = headerCells.map((cell) => cell.getBoundingClientRect());
  const columnActionTop = tableTop - layout.resizeHeight - layout.laneGap - layout.actionSize / 2;
  const resizeTop = tableTop - layout.resizeHeight - layout.laneGap;
  const columnActions = cellRects.map((rect, index) => ({
    index,
    left: rect.right - wrapperRect.left + scrollLeft,
    top: columnActionTop,
  }));
  const rowActions = visibleRows.map((row, index) => {
    const rect = row.getBoundingClientRect();
    return {
      index,
      left: Math.max(layout.actionSize / 2, tableLeft - layout.actionSize / 2),
      top: rect.bottom - wrapperRect.top + scrollTop,
    };
  });

  return {
    boundaries: cellRects.slice(0, -1).map((rect) => rect.right - wrapperRect.left + scrollLeft),
    columnActions,
    columnWidths: cellRects.map((rect) => rect.width),
    height: tableRect.height,
    layerHeight: Math.max(wrapper.clientHeight, tableTop + tableRect.height),
    layerWidth: Math.max(wrapper.clientWidth, tableLeft + tableRect.width),
    resizeHeight: layout.resizeHeight,
    resizeTop,
    rowActions,
    tableLeft,
    tableWidth: tableRect.width,
    tableTop,
  };
}

export function sameGeometry(
  left: TableEdgeGeometry | null,
  right: TableEdgeGeometry | null,
): boolean {
  if (!left || !right) return left === right;
  return (
    left.height === right.height &&
    left.layerHeight === right.layerHeight &&
    left.layerWidth === right.layerWidth &&
    left.resizeHeight === right.resizeHeight &&
    left.resizeTop === right.resizeTop &&
    left.tableLeft === right.tableLeft &&
    left.tableWidth === right.tableWidth &&
    left.tableTop === right.tableTop &&
    arraysEqual(left.boundaries, right.boundaries) &&
    arraysEqual(left.columnWidths, right.columnWidths) &&
    positionsEqual(left.columnActions, right.columnActions) &&
    positionsEqual(left.rowActions, right.rowActions)
  );
}

function arraysEqual(left: readonly number[], right: readonly number[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function positionsEqual(left: readonly EdgeActionPosition[], right: readonly EdgeActionPosition[]) {
  return (
    left.length === right.length &&
    left.every(
      (position, index) =>
        position.index === right[index]?.index &&
        position.left === right[index]?.left &&
        position.top === right[index]?.top,
    )
  );
}
