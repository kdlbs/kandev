export type ContentLine = {
  line: number;
  content: string;
};

export type ContentLineAnchor = {
  line: number;
  content: string;
  beforeLines: string[];
  afterLines: string[];
};

const CONTEXT_DEPTH = 3;

function normalizeLine(content: string): string {
  return content.replace(/\r/g, "").replace(/[\t ]+$/g, "");
}

export function captureContentLineAnchor(
  lines: readonly ContentLine[],
  index: number,
): ContentLineAnchor | null {
  const anchored = lines[index];
  if (!anchored) return null;
  return {
    line: anchored.line,
    content: normalizeLine(anchored.content),
    beforeLines: lines
      .slice(Math.max(0, index - CONTEXT_DEPTH), index)
      .reverse()
      .map(({ content }) => normalizeLine(content)),
    afterLines: lines
      .slice(index + 1, index + CONTEXT_DEPTH + 1)
      .map(({ content }) => normalizeLine(content)),
  };
}

function contextScore(
  lines: readonly ContentLine[],
  index: number,
  anchor: ContentLineAnchor,
): number {
  let score = 0;
  for (let distance = 1; distance <= CONTEXT_DEPTH; distance += 1) {
    const before = anchor.beforeLines[distance - 1];
    const after = anchor.afterLines[distance - 1];
    const previousLine = lines[index - distance];
    const nextLine = lines[index + distance];
    if (before !== undefined && previousLine && normalizeLine(previousLine.content) === before) {
      score += CONTEXT_DEPTH + 1 - distance;
    }
    if (after !== undefined && nextLine && normalizeLine(nextLine.content) === after) {
      score += CONTEXT_DEPTH + 1 - distance;
    }
  }
  return score;
}

function firstContextMatch(anchor: ContentLineAnchor, content: string): number {
  const normalized = normalizeLine(content);
  const beforeIndex = anchor.beforeLines.indexOf(normalized);
  const afterIndex = anchor.afterLines.indexOf(normalized);
  if (beforeIndex < 0) return afterIndex;
  if (afterIndex < 0) return beforeIndex;
  return Math.min(beforeIndex, afterIndex);
}

export function resolveContentLineAnchor<T extends ContentLine>(
  lines: readonly T[],
  anchor: ContentLineAnchor,
): T | null {
  if (lines.length === 0) return null;
  const normalizedAnchor = normalizeLine(anchor.content);
  const exact = lines
    .map((line, index) => ({ line, index }))
    .filter(
      ({ line }) => normalizedAnchor !== "" && normalizeLine(line.content) === normalizedAnchor,
    );

  if (exact.length > 0) {
    return exact.sort((left, right) => {
      const scoreDelta =
        contextScore(lines, right.index, anchor) - contextScore(lines, left.index, anchor);
      if (scoreDelta !== 0) return scoreDelta;
      return Math.abs(left.line.line - anchor.line) - Math.abs(right.line.line - anchor.line);
    })[0].line;
  }

  const survivingContext = lines
    .map((line) => ({ line, contextIndex: firstContextMatch(anchor, line.content) }))
    .filter(({ contextIndex }) => contextIndex >= 0);
  if (survivingContext.length > 0) {
    return survivingContext.sort((left, right) => {
      if (left.contextIndex !== right.contextIndex) return left.contextIndex - right.contextIndex;
      return Math.abs(left.line.line - anchor.line) - Math.abs(right.line.line - anchor.line);
    })[0].line;
  }

  return [...lines].sort(
    (left, right) => Math.abs(left.line - anchor.line) - Math.abs(right.line - anchor.line),
  )[0];
}
