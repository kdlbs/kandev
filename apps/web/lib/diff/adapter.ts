import type { FileDiffData, DiffComment, AnnotationSide, CommentAnnotation } from "./types";

/**
 * Format line range for display (e.g., "L10" or "L10-15")
 */
export function formatLineRange(startLine: number, endLine: number): string {
  return startLine === endLine ? `L${startLine}` : `L${startLine}-${endLine}`;
}

/**
 * Frontend types that mirror backend FileMutation
 */
export interface FileMutation {
  type: "create" | "replace" | "patch" | "delete" | "rename";
  content?: string;
  old_content?: string;
  new_content?: string;
  diff?: string;
  new_path?: string;
  start_line?: number;
  end_line?: number;
}

export interface ModifyFilePayload {
  file_path: string;
  mutations: FileMutation[];
}

/**
 * Normalize a diff string by ensuring it has proper headers.
 * Required because backend diffs may not include full git headers.
 *
 * Detects new-file (`@@ -0,0 +X,Y @@`) and deleted-file (`@@ -X,Y +0,0 @@`)
 * hunks and emits the canonical `--- /dev/null` / `+++ /dev/null` markers.
 * @pierre/diffs 1.1.x relies on these to classify the change type; when both
 * sides point at `a/file` and `b/file` for a new-file patch the renderer
 * throws "deletionLine and additionLine are null" because it expects a
 * modification and there's no old content to read.
 */
export function normalizeDiffString(diff: string, filePath: string): string {
  if (!diff) return "";

  const trimmed = diff.trim();

  // Detect new/deleted file from the first hunk header so we can force the
  // correct old/new file markers even when the caller pre-attached headers.
  const isNewFile = /@@\s+-0,0\s+\+\d/.test(trimmed);
  const isDeletedFile = /@@\s+-\d[\d,]*\s+\+0,0\s+@@/.test(trimmed);

  // Strip any existing diff --git / index / mode / --- / +++ headers so we
  // can re-emit them with the right /dev/null markers below. Hunk content
  // (lines starting with @@) and everything after stays untouched.
  const body = trimmed.replace(
    /^(diff --git[^\n]*\n|index [^\n]*\n|(?:new|deleted) file mode [^\n]*\n|--- [^\n]*\n|\+\+\+ [^\n]*\n)+/,
    "",
  );

  const headers = [`diff --git a/${filePath} b/${filePath}`];
  if (isNewFile) headers.push("new file mode 100644");
  else if (isDeletedFile) headers.push("deleted file mode 100644");
  headers.push(
    isNewFile ? "--- /dev/null" : `--- a/${filePath}`,
    isDeletedFile ? "+++ /dev/null" : `+++ b/${filePath}`,
  );
  // Ensure body ends with a newline. @pierre/diffs 1.1.x tolerates a missing
  // EOF newline on real text content, but its hunk row generator can over-
  // count rows when the last line lacks a terminator, occasionally tripping
  // "deletionLine and additionLine are null" inside DiffHunksRenderer.
  const tail = body.endsWith("\n") ? "" : "\n";
  return headers.join("\n") + "\n" + body + tail;
}

/**
 * Transform a backend FileMutation to FileDiffData for @pierre/diffs.
 * The DiffViewer component handles diff generation from content using the library.
 */
export function transformFileMutation(filePath: string, mutation: FileMutation): FileDiffData {
  const resolvedPath = mutation.new_path || filePath;
  return {
    filePath: resolvedPath,
    oldContent: mutation.old_content || "",
    newContent: mutation.new_content || mutation.content || "",
    diff: resolveMutationDiff(mutation, resolvedPath),
    additions: 0,
    deletions: 0,
  };
}

/** Resolve the diff string for a mutation: explicit diff > create diff > undefined. */
function resolveMutationDiff(mutation: FileMutation, filePath: string): string | undefined {
  if (mutation.diff) return normalizeDiffString(mutation.diff, filePath);
  if (mutation.type === "create" && mutation.content)
    return generateCreateDiff(mutation.content, filePath);
  return undefined;
}

/**
 * Generate an all-additions diff for new file creation.
 * Shows the entire content as green (added) lines in the diff viewer.
 */
function generateCreateDiff(content: string, filePath: string): string {
  const lines = content.split("\n");
  const header = [
    `diff --git a/${filePath} b/${filePath}`,
    `new file mode 100644`,
    `--- /dev/null`,
    `+++ b/${filePath}`,
    `@@ -0,0 +1,${lines.length} @@`,
  ];
  const addedLines = lines.map((l) => `+${l}`);
  return [...header, ...addedLines].join("\n");
}

/**
 * Transform a git status diff string to FileDiffData.
 * Language detection is handled automatically by the library.
 */
export function transformGitDiff(
  filePath: string,
  diff: string,
  _status: "A" | "M" | "D" | "??" | string,
): FileDiffData {
  return {
    filePath,
    oldContent: "",
    newContent: "",
    diff: normalizeDiffString(diff, filePath),
    additions: 0,
    deletions: 0,
  };
}

/**
 * Convert DiffComment[] to @pierre/diffs DiffLineAnnotation[]
 */
export function commentsToAnnotations(comments: DiffComment[]): CommentAnnotation[] {
  return comments.map((comment) => ({
    side: comment.side,
    lineNumber: comment.endLine, // Anchor at the end of the range
    metadata: {
      comment,
      isEditing: false,
    },
  }));
}

function isDiffHeader(line: string): boolean {
  return line.startsWith("diff --git") || line.startsWith("---") || line.startsWith("+++");
}

function isInRange(lineNum: number, startLine: number, endLine: number): boolean {
  return lineNum >= startLine && lineNum <= endLine;
}

type DiffLineCounters = { currentOldLine: number; currentNewLine: number };

function processHunkHeader(line: string, counters: DiffLineCounters): boolean {
  const hunkMatch = line.match(/^@@\s*-(\d+)(?:,\d+)?\s*\+(\d+)(?:,\d+)?\s*@@/);
  if (!hunkMatch) return false;
  counters.currentOldLine = parseInt(hunkMatch[1], 10);
  counters.currentNewLine = parseInt(hunkMatch[2], 10);
  return true;
}

type ProcessDiffLineParams = {
  line: string;
  counters: DiffLineCounters;
  startLine: number;
  endLine: number;
  side: AnnotationSide;
  resultLines: string[];
};

function processDiffLine({
  line,
  counters,
  startLine,
  endLine,
  side,
  resultLines,
}: ProcessDiffLineParams): void {
  if (line.startsWith("+")) {
    if (side === "additions" && isInRange(counters.currentNewLine, startLine, endLine)) {
      resultLines.push(line.substring(1));
    }
    counters.currentNewLine++;
  } else if (line.startsWith("-")) {
    if (side === "deletions" && isInRange(counters.currentOldLine, startLine, endLine)) {
      resultLines.push(line.substring(1));
    }
    counters.currentOldLine++;
  } else if (line.startsWith(" ") || (!line.startsWith("@") && line !== "")) {
    const content = line.startsWith(" ") ? line.substring(1) : line;
    const lineNum = side === "additions" ? counters.currentNewLine : counters.currentOldLine;
    if (isInRange(lineNum, startLine, endLine)) resultLines.push(content);
    counters.currentOldLine++;
    counters.currentNewLine++;
  }
}

/**
 * Extract code content from diff for a line range
 */
export function extractCodeFromDiff(
  diff: string,
  startLine: number,
  endLine: number,
  side: AnnotationSide,
): string {
  const lines = diff.split("\n");
  const resultLines: string[] = [];
  const counters: DiffLineCounters = { currentOldLine: 0, currentNewLine: 0 };

  for (const line of lines) {
    if (processHunkHeader(line, counters)) continue;
    if (isDiffHeader(line)) continue;
    processDiffLine({ line, counters, startLine, endLine, side, resultLines });
  }

  return resultLines.join("\n");
}

/**
 * Extract code content from full file content for a line range
 */
export function extractCodeFromContent(
  content: string,
  startLine: number,
  endLine: number,
): string {
  const lines = content.split("\n");
  return lines.slice(startLine - 1, endLine).join("\n");
}

/**
 * Compute simple line-level diff stats between two strings.
 * Returns the number of added and deleted lines.
 */
export function computeLineDiffStats(
  original: string,
  current: string,
): { additions: number; deletions: number } {
  const originalLines = original.split("\n");
  const currentLines = current.split("\n");
  let additions = 0;
  let deletions = 0;
  const maxLen = Math.max(originalLines.length, currentLines.length);
  for (let i = 0; i < maxLen; i++) {
    const origLine = originalLines[i];
    const currLine = currentLines[i];
    if (origLine === undefined && currLine !== undefined) additions++;
    else if (origLine !== undefined && currLine === undefined) deletions++;
    else if (origLine !== currLine) {
      additions++;
      deletions++;
    }
  }
  return { additions, deletions };
}
