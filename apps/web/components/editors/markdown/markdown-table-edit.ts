export type MarkdownTableEditResult = {
  source: string;
  selectionOffset: number;
};

type SourceLine = {
  content: string;
  ending: string;
  start: number;
  endExclusive: number;
};

type TableCell = {
  start: number;
  endExclusive: number;
  separatorAfter: number | undefined;
  leadingWhitespace: string;
  trailingWhitespace: string;
};

type TableRow = {
  line: SourceLine;
  contentEnd: number;
  cells: TableCell[];
};

type ParsedTable = {
  rows: TableRow[];
  visibleRowCount: number;
  columnCount: number;
};

/**
 * Inserts a blank body row after a visible table row.
 *
 * The table delimiter is not part of the visible row index. Inserting after
 * the header therefore uses the delimiter as the source anchor, keeping the
 * two required header rows adjacent.
 */
export function insertMarkdownTableRow(
  source: string,
  visibleRowIndex: number,
): MarkdownTableEditResult {
  return insertTableRow(source, visibleRowIndex);
}

/** Inserts a blank column to the right of a visible table column. */
export function insertMarkdownTableColumn(
  source: string,
  columnIndex: number,
): MarkdownTableEditResult {
  const table = parseTable(source);
  if (!table || !Number.isInteger(columnIndex) || columnIndex < 0) {
    return unchanged(source);
  }
  if (columnIndex >= table.columnCount) return unchanged(source);

  let output = "";
  let sourceCursor = 0;
  let selectionOffset = 0;

  table.rows.forEach((row, rowIndex) => {
    output += source.slice(sourceCursor, row.line.start);
    const insertion = createColumnInsertion(row, columnIndex, rowIndex === 1);
    const content = `${row.line.content.slice(0, insertion.offset)}${insertion.text}${row.line.content.slice(
      insertion.offset,
    )}`;
    const rowOutputStart = output.length;
    output += `${content}${row.line.ending}`;
    if (rowIndex === 0) {
      selectionOffset = rowOutputStart + insertion.offset + insertion.selectionOffset;
    }
    sourceCursor = row.line.endExclusive;
  });

  output += source.slice(sourceCursor);
  return { source: output, selectionOffset };
}

/**
 * Compatibility wrapper for the Task 05 append action. New callers should use
 * {@link insertMarkdownTableRow} with a visible row index.
 */
export function appendMarkdownTableRow(
  source: string,
  columnCount: number,
): MarkdownTableEditResult {
  const table = parseTable(source);
  if (!table || columnCount < 1 || table.visibleRowCount < 1) return unchanged(source);
  return insertTableRow(source, table.visibleRowCount - 1, columnCount);
}

/**
 * Compatibility wrapper for the Task 05 append action. New callers should use
 * {@link insertMarkdownTableColumn} with a visible column index.
 */
export function appendMarkdownTableColumn(source: string): MarkdownTableEditResult {
  const table = parseTable(source);
  if (!table || table.columnCount < 1) return unchanged(source);
  return insertMarkdownTableColumn(source, table.columnCount - 1);
}

function splitSourceLines(source: string): SourceLine[] {
  const lines: SourceLine[] = [];
  const pattern = /([^\r\n]*)(\r\n|\n|$)/g;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(source)) !== null) {
    if (match[0].length === 0) break;
    const start = match.index;
    lines.push({
      content: match[1],
      ending: match[2],
      start,
      endExclusive: start + match[0].length,
    });
  }
  return lines;
}

function parseTable(source: string): ParsedTable | undefined {
  const lines = splitSourceLines(source);
  const firstBlank = lines.findIndex((line) => line.content.trim().length === 0);
  const rowLines = lines.slice(0, firstBlank === -1 ? lines.length : firstBlank);
  if (rowLines.length < 2) return undefined;

  const rows = rowLines.map((line) => parseTableRow(line));
  const columnCount = rows[0].cells.length;
  if (columnCount < 1 || rows[1].cells.length < columnCount) return undefined;

  return {
    rows,
    visibleRowCount: rowLines.length - 1,
    columnCount,
  };
}

function parseTableRow(line: SourceLine): TableRow {
  const contentEnd = line.content.trimEnd().length;
  const pipes = findUnescapedPipes(line.content, contentEnd);
  const leadingPipe = pipes.length > 0 && line.content.slice(0, pipes[0]).trim().length === 0;
  const trailingPipe = pipes.length > 0 && pipes[pipes.length - 1] === contentEnd - 1;
  const interiorPipes = pipes.slice(leadingPipe ? 1 : 0, trailingPipe ? -1 : undefined);
  const starts = [leadingPipe ? pipes[0] + 1 : 0, ...interiorPipes.map((pipe) => pipe + 1)];
  const ends = [...interiorPipes, trailingPipe ? pipes[pipes.length - 1] : contentEnd];
  const cells = starts.map((start, index) => {
    const endExclusive = ends[index] ?? contentEnd;
    const text = line.content.slice(start, endExclusive);
    const whitespaceOnly = text.trim().length === 0;
    const whitespaceSplit = Math.ceil(text.length / 2);
    return {
      start,
      endExclusive,
      separatorAfter:
        endExclusive < contentEnd && line.content[endExclusive] === "|" ? endExclusive : undefined,
      leadingWhitespace: whitespaceOnly
        ? text.slice(0, whitespaceSplit)
        : (text.match(/^\s*/)?.[0] ?? ""),
      trailingWhitespace: whitespaceOnly
        ? text.slice(whitespaceSplit)
        : (text.match(/\s*$/)?.[0] ?? ""),
    };
  });

  return { line, contentEnd, cells };
}

function findUnescapedPipes(source: string, contentEnd: number): number[] {
  const pipes: number[] = [];
  for (let index = 0; index < contentEnd; index++) {
    if (source[index] !== "|" || isEscaped(source, index)) continue;
    pipes.push(index);
  }
  return pipes;
}

function isEscaped(source: string, index: number): boolean {
  let backslashes = 0;
  for (let cursor = index - 1; cursor >= 0 && source[cursor] === "\\"; cursor--) {
    backslashes++;
  }
  return backslashes % 2 === 1;
}

type RowInsertionDetails = {
  anchorRow: TableRow;
  indentationLength: number;
  leadingPipe: boolean;
  row: string;
};

function getRowInsertionDetails(
  source: string,
  visibleRowIndex: number,
  columnCountOverride?: number,
): RowInsertionDetails | undefined {
  const table = parseTable(source);
  if (!table || !Number.isInteger(visibleRowIndex) || visibleRowIndex < 0) return undefined;
  if (visibleRowIndex >= table.visibleRowCount) return undefined;

  const header = table.rows[0];
  const anchorRow = table.rows[visibleRowIndex === 0 ? 1 : visibleRowIndex + 1];
  const columnCount = columnCountOverride ?? table.columnCount;
  if (!header || !anchorRow || columnCount < 1) return undefined;

  const indentation = header.line.content.match(/^\s*/)?.[0] ?? "";
  const leadingPipe = header.line.content.trimStart().startsWith("|");
  return {
    anchorRow,
    indentationLength: indentation.length,
    leadingPipe,
    row: createEmptyRow(
      columnCount,
      indentation,
      leadingPipe,
      hasUnescapedTrailingPipe(header.line.content),
    ),
  };
}

function insertTableRow(
  source: string,
  visibleRowIndex: number,
  columnCountOverride?: number,
): MarkdownTableEditResult {
  const details = getRowInsertionDetails(source, visibleRowIndex, columnCountOverride);
  if (!details) return unchanged(source);

  const { anchorRow, indentationLength, leadingPipe, row } = details;
  const lineEnding = anchorRow.line.ending || detectEol(source);
  const insertionOffset = anchorRow.line.endExclusive;
  const inserted = createInsertedRow(anchorRow, row, lineEnding);
  const rowStart = insertionOffset + (anchorRow.line.ending ? 0 : lineEnding.length);
  const cellStart = rowStart + indentationLength;

  return {
    source: `${source.slice(0, insertionOffset)}${inserted}${source.slice(insertionOffset)}`,
    selectionOffset: cellStart + (leadingPipe ? 1 : 0),
  };
}

function createInsertedRow(anchorRow: TableRow, row: string, lineEnding: string): string {
  return anchorRow.line.ending ? `${row}${lineEnding}` : `${lineEnding}${row}`;
}

type ColumnInsertion = {
  offset: number;
  text: string;
  selectionOffset: number;
};

function createColumnInsertion(
  row: TableRow,
  columnIndex: number,
  delimiter: boolean,
): ColumnInsertion {
  const selected = row.cells[columnIndex];
  const next = row.cells[columnIndex + 1];
  const { before, after } = inferCellSpacing(row);
  const selectedTrailing = selected?.trailingWhitespace || before;
  const nextLeading = next?.leadingWhitespace || after;
  const cellText = delimiter ? "---" : "";

  if (selected?.separatorAfter !== undefined) {
    return {
      offset: selected.separatorAfter + 1,
      text: `${nextLeading}${cellText}${selectedTrailing}|`,
      selectionOffset: nextLeading.length,
    };
  }

  const offset = row.contentEnd;
  return {
    offset,
    text: `${before}|${after}${cellText}`,
    selectionOffset: before.length + 1 + after.length,
  };
}

function inferCellSpacing(row: TableRow): { before: string; after: string } {
  const first = row.cells[0];
  const second = row.cells[1];
  if (second) {
    return { before: first.trailingWhitespace, after: second.leadingWhitespace };
  }
  return {
    before: first?.trailingWhitespace ?? "",
    after: first?.leadingWhitespace ?? "",
  };
}

function createEmptyRow(
  columnCount: number,
  indentation: string,
  leadingPipe: boolean,
  trailingPipe: boolean,
): string {
  const cells = Array.from({ length: columnCount }, () => "").join(" | ");
  return `${indentation}${leadingPipe ? "| " : ""}${cells}${trailingPipe ? " |" : ""}`;
}

function detectEol(source: string): string {
  return source.includes("\r\n") ? "\r\n" : "\n";
}

function hasUnescapedTrailingPipe(source: string): boolean {
  const trimmed = source.trimEnd();
  if (!trimmed.endsWith("|")) return false;
  return !isEscaped(trimmed, trimmed.length - 1);
}

function unchanged(source: string): MarkdownTableEditResult {
  return { source, selectionOffset: 0 };
}
