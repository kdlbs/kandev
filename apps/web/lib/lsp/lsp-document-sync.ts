import type { LspRange } from "./lsp-json-rpc";

const TEXT_DOCUMENT_SYNC_NONE = 0;
const TEXT_DOCUMENT_SYNC_FULL = 1;
const TEXT_DOCUMENT_SYNC_INCREMENTAL = 2;

type DocumentContentChange = { text: string; range?: LspRange };
type DocumentSaveParams = { textDocument: { uri: string }; text?: string };

function textDocumentSyncKind(serverCapabilities: Record<string, unknown> | null): number {
  const sync = serverCapabilities?.textDocumentSync;
  if (typeof sync === "number") return sync;
  if (!sync || typeof sync !== "object") return TEXT_DOCUMENT_SYNC_NONE;
  const change = (sync as { change?: unknown }).change;
  return typeof change === "number" ? change : TEXT_DOCUMENT_SYNC_NONE;
}

function isUnsafeTextBoundary(text: string, offset: number): boolean {
  if (offset <= 0 || offset >= text.length) return false;
  const before = text.charCodeAt(offset - 1);
  const after = text.charCodeAt(offset);
  const splitsSurrogate =
    before >= 0xd800 && before <= 0xdbff && after >= 0xdc00 && after <= 0xdfff;
  const splitsCrLf = before === 0x0d && after === 0x0a;
  return splitsSurrogate || splitsCrLf;
}

function commonPrefixLength(previousText: string, nextText: string): number {
  const limit = Math.min(previousText.length, nextText.length);
  let offset = 0;
  while (offset < limit && previousText.charCodeAt(offset) === nextText.charCodeAt(offset))
    offset++;
  while (
    offset > 0 &&
    (isUnsafeTextBoundary(previousText, offset) || isUnsafeTextBoundary(nextText, offset))
  ) {
    offset--;
  }
  return offset;
}

function changedTextEnds(previousText: string, nextText: string, start: number) {
  let previousEnd = previousText.length;
  let nextEnd = nextText.length;
  while (
    previousEnd > start &&
    nextEnd > start &&
    previousText.charCodeAt(previousEnd - 1) === nextText.charCodeAt(nextEnd - 1)
  ) {
    previousEnd--;
    nextEnd--;
  }
  while (
    previousEnd < previousText.length &&
    nextEnd < nextText.length &&
    (isUnsafeTextBoundary(previousText, previousEnd) || isUnsafeTextBoundary(nextText, nextEnd))
  ) {
    previousEnd++;
    nextEnd++;
  }
  return { nextEnd, previousEnd };
}

function positionAt(text: string, offset: number) {
  const lines = text.slice(0, offset).split(/\r\n|\r|\n/);
  return { line: lines.length - 1, character: lines.at(-1)?.length ?? 0 };
}

function incrementalContentChange(previousText: string, nextText: string): DocumentContentChange {
  const start = commonPrefixLength(previousText, nextText);
  const { previousEnd, nextEnd } = changedTextEnds(previousText, nextText, start);
  return {
    range: {
      start: positionAt(previousText, start),
      end: positionAt(previousText, previousEnd),
    },
    text: nextText.slice(start, nextEnd),
  };
}

export function buildDocumentContentChanges(
  serverCapabilities: Record<string, unknown> | null,
  previousText: string,
  nextText: string,
): DocumentContentChange[] {
  if (previousText === nextText) return [];
  const syncKind = textDocumentSyncKind(serverCapabilities);
  if (syncKind === TEXT_DOCUMENT_SYNC_FULL) return [{ text: nextText }];
  if (syncKind === TEXT_DOCUMENT_SYNC_INCREMENTAL) {
    return [incrementalContentChange(previousText, nextText)];
  }
  return [];
}

export function buildDocumentSaveParams(
  serverCapabilities: Record<string, unknown> | null,
  uri: string,
  text: string,
): DocumentSaveParams | null {
  const sync = serverCapabilities?.textDocumentSync;
  if (!sync || typeof sync !== "object" || Array.isArray(sync)) return null;

  const save = (sync as { save?: unknown }).save;
  if (save === true) return { textDocument: { uri } };
  if (!save || typeof save !== "object" || Array.isArray(save)) return null;

  return (save as { includeText?: unknown }).includeText === true
    ? { textDocument: { uri }, text }
    : { textDocument: { uri } };
}
