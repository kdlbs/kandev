export type MarkdownTableEditResult = {
  source: string;
  selectionOffset: number;
};

type SourceLine = {
  content: string;
  ending: string;
};

export function appendMarkdownTableRow(
  source: string,
  columnCount: number,
): MarkdownTableEditResult {
  const lines = splitSourceLines(source);
  const rowCount = countTableRows(lines);
  if (rowCount === 0 || columnCount < 1) return { source, selectionOffset: 0 };

  const firstRow = lines[0].content;
  const row = createEmptyRow(
    columnCount,
    firstRow.trimStart().startsWith("|"),
    hasUnescapedTrailingPipe(firstRow),
  );
  const eol = detectEol(source);
  const lastRow = lines[rowCount - 1];
  const insertionOffset = lines
    .slice(0, rowCount)
    .reduce((total, line) => total + line.content.length + line.ending.length, 0);
  const prefix = lastRow.ending ? "" : eol;
  const suffix = lastRow.ending ? eol : "";
  const inserted = `${prefix}${row}${suffix}`;
  const rowStart = insertionOffset + prefix.length;

  return {
    source: `${source.slice(0, insertionOffset)}${inserted}${source.slice(insertionOffset)}`,
    selectionOffset: rowStart + (row.startsWith("|") ? 1 : 0),
  };
}

export function appendMarkdownTableColumn(source: string): MarkdownTableEditResult {
  const lines = splitSourceLines(source);
  const rowCount = countTableRows(lines);
  if (rowCount < 2) return { source, selectionOffset: 0 };

  let selectionOffset = 0;
  let outputOffset = 0;
  const nextLines = lines.map((line, index) => {
    if (index >= rowCount) {
      outputOffset += line.content.length + line.ending.length;
      return line;
    }

    const trailingWhitespace = line.content.match(/\s*$/)?.[0] ?? "";
    const body = line.content.slice(0, line.content.length - trailingWhitespace.length);
    const addition = columnAddition(index === 1, hasUnescapedTrailingPipe(body));
    if (index === 0) {
      selectionOffset = outputOffset + body.length + (addition === "  |" ? 1 : 2);
    }
    const nextLine = { content: `${body}${addition}${trailingWhitespace}`, ending: line.ending };
    outputOffset += nextLine.content.length + nextLine.ending.length;
    return nextLine;
  });

  return {
    source: nextLines.map((line) => `${line.content}${line.ending}`).join(""),
    selectionOffset,
  };
}

function columnAddition(delimiter: boolean, trailingPipe: boolean): string {
  if (delimiter) return trailingPipe ? " --- |" : " | ---";
  return trailingPipe ? "  |" : " | ";
}

function splitSourceLines(source: string): SourceLine[] {
  const lines: SourceLine[] = [];
  const pattern = /([^\r\n]*)(\r\n|\n|$)/g;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(source)) !== null) {
    if (match[0].length === 0) break;
    lines.push({ content: match[1], ending: match[2] });
  }
  return lines;
}

function countTableRows(lines: readonly SourceLine[]): number {
  const firstBlank = lines.findIndex((line) => line.content.trim().length === 0);
  return firstBlank === -1 ? lines.length : firstBlank;
}

function detectEol(source: string): string {
  return source.includes("\r\n") ? "\r\n" : "\n";
}

function createEmptyRow(columnCount: number, leadingPipe: boolean, trailingPipe: boolean): string {
  const cells = Array.from({ length: columnCount }, () => "").join(" | ");
  return `${leadingPipe ? "| " : ""}${cells}${trailingPipe ? " |" : ""}`;
}

function hasUnescapedTrailingPipe(source: string): boolean {
  const trimmed = source.trimEnd();
  if (!trimmed.endsWith("|")) return false;
  let backslashes = 0;
  for (let index = trimmed.length - 2; index >= 0 && trimmed[index] === "\\"; index--) {
    backslashes++;
  }
  return backslashes % 2 === 0;
}
