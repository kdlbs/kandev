export type FileTreeRowBounds = Readonly<{
  bottom: number;
  path: string | null;
  top: number;
}>;

export type FileTreeViewportGeometry = Readonly<{
  bottom: number;
  clientHeight: number;
  rows: readonly FileTreeRowBounds[];
  scrollTop: number;
  top: number;
  treeHeight: number;
}>;

export type FileTreeGeometryOptions = Readonly<{
  contentTopInset?: number;
  endPadding?: number;
  expectedPaths?: readonly string[];
  tolerance?: number;
}>;

export const FILE_TREE_ROW_EDGE_TOLERANCE_PX = 2;
export const FILE_TREE_END_PADDING_PX = 8;

function expectedPathIssues(
  rows: readonly FileTreeRowBounds[],
  expectedPaths: readonly string[] | undefined,
): string[] {
  if (!expectedPaths) return [];
  const actualPaths = rows.flatMap((row) => (row.path === null ? [] : [row.path]));
  if (
    actualPaths.length === expectedPaths.length &&
    actualPaths.every((path, index) => path === expectedPaths[index])
  ) {
    return [];
  }
  return [
    `file tree paths differ: expected ${JSON.stringify(expectedPaths)}, got ${JSON.stringify(actualPaths)}`,
  ];
}

function viewportEdgeIssues(
  geometry: FileTreeViewportGeometry,
  options: FileTreeGeometryOptions = {},
): string[] {
  const tolerance = options.tolerance ?? FILE_TREE_ROW_EDGE_TOLERANCE_PX;
  const contentTopInset = options.contentTopInset ?? 0;
  const endPadding = options.endPadding ?? FILE_TREE_END_PADDING_PX;
  const issues: string[] = [];
  const [firstRow] = geometry.rows;
  const lastRow = geometry.rows.at(-1);

  if (!firstRow || !lastRow) return issues;

  const contentTop = geometry.top + contentTopInset;
  if (firstRow.top > contentTop + tolerance) {
    issues.push(
      `file tree has a blank gap at the viewport top: first row starts ${firstRow.top - contentTop}px below content top`,
    );
  }

  const treeBottom = geometry.top - geometry.scrollTop + geometry.treeHeight;
  const treeIsShort = geometry.treeHeight < geometry.clientHeight;
  if (!treeIsShort) {
    const treeEndsAtViewport = treeBottom <= geometry.bottom + endPadding + tolerance;
    const contentBottom = treeEndsAtViewport
      ? Math.min(geometry.bottom, treeBottom) - endPadding
      : geometry.bottom;
    if (lastRow.bottom < contentBottom - tolerance) {
      issues.push(
        `file tree has a blank gap at the viewport bottom: last row ends ${contentBottom - lastRow.bottom}px before content bottom`,
      );
    }
  }

  return issues;
}

function adjacentRowIssues(rows: readonly FileTreeRowBounds[], tolerance: number): string[] {
  const issues: string[] = [];
  for (let index = 1; index < rows.length; index += 1) {
    const previous = rows[index - 1];
    const current = rows[index];
    if (!previous || !current) continue;
    const gap = current.top - previous.bottom;
    if (gap < -tolerance) {
      issues.push(
        `file tree rows ${previous.path ?? "unknown"} and ${current.path ?? "unknown"} overlap by ${-gap}px`,
      );
    } else if (gap > tolerance) {
      issues.push(
        `file tree rows ${previous.path ?? "unknown"} and ${current.path ?? "unknown"} have a blank gap of ${gap}px`,
      );
    }
  }

  return issues;
}

export function fileTreeGeometryIssues(
  geometry: FileTreeViewportGeometry,
  options: FileTreeGeometryOptions = {},
): string[] {
  if (geometry.rows.length === 0) return ["file tree has no mounted rows in its viewport"];
  const tolerance = options.tolerance ?? FILE_TREE_ROW_EDGE_TOLERANCE_PX;
  return [
    ...expectedPathIssues(geometry.rows, options.expectedPaths),
    ...viewportEdgeIssues(geometry, options),
    ...adjacentRowIssues(geometry.rows, tolerance),
  ];
}
