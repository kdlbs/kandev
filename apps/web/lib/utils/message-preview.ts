export const MESSAGE_PREVIEW_MAX_LINES = 200;
export const MESSAGE_PREVIEW_MAX_CODE_UNITS = 16_000;

export type MessagePreviewLimits = {
  maxLines?: number;
  maxCodeUnits?: number;
};

export type MessagePreview = {
  content: string;
  truncated: boolean;
  logicalLines: number;
  codeUnits: number;
};

function isSurrogatePair(text: string, index: number): boolean {
  const first = text.charCodeAt(index);
  const second = text.charCodeAt(index + 1);
  return first >= 0xd800 && first <= 0xdbff && second >= 0xdc00 && second <= 0xdfff;
}

function isLineBreak(text: string, index: number): boolean {
  return text[index] === "\n" || text[index] === "\r";
}

function characterLengthAt(text: string, index: number): number {
  if (text[index] === "\r" && text[index + 1] === "\n") return 2;
  if (isSurrogatePair(text, index)) return 2;
  return 1;
}

function previewResult(
  source: string,
  end: number,
  truncated: boolean,
  logicalLines: number,
): MessagePreview {
  return {
    content: source.slice(0, end),
    truncated,
    logicalLines,
    codeUnits: end,
  };
}

/** Select a source prefix without splitting Unicode pairs or CRLF endings. */
export function getMessagePreview(
  source: string,
  {
    maxLines = MESSAGE_PREVIEW_MAX_LINES,
    maxCodeUnits = MESSAGE_PREVIEW_MAX_CODE_UNITS,
  }: MessagePreviewLimits = {},
): MessagePreview {
  if (source.length === 0) return previewResult(source, 0, false, 0);

  let index = 0;
  let codeUnits = 0;
  let completedLines = 0;
  let currentLineHasContent = false;

  while (index < source.length) {
    const lineBreak = isLineBreak(source, index);
    const characterLength = characterLengthAt(source, index);

    if (codeUnits + characterLength > maxCodeUnits) {
      return previewResult(source, index, true, completedLines + (currentLineHasContent ? 1 : 0));
    }

    if (lineBreak) {
      if (completedLines >= maxLines) {
        return previewResult(source, index, true, completedLines);
      }
      index += characterLength;
      codeUnits += characterLength;
      completedLines += 1;
      currentLineHasContent = false;
      continue;
    }

    if (completedLines >= maxLines) {
      return previewResult(source, index, true, completedLines);
    }

    index += characterLength;
    codeUnits += characterLength;
    currentLineHasContent = true;
  }

  return previewResult(source, index, false, completedLines + (currentLineHasContent ? 1 : 0));
}
