import type { FileDiffData, DiffComment, AnnotationSide, CommentAnnotation } from './types';

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
  type: 'create' | 'replace' | 'patch' | 'delete' | 'rename';
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
 */
export function normalizeDiffString(diff: string, filePath: string): string {
  if (!diff) return '';

  const trimmed = diff.trim();

  // Check if the diff already has headers
  if (trimmed.startsWith('diff --git')) {
    return trimmed;
  }

  // Check if it has file headers but not diff header
  const hasFileHeaders = trimmed.includes('---') && trimmed.includes('+++');

  if (hasFileHeaders) {
    // Add just the diff header
    return `diff --git a/${filePath} b/${filePath}\n${trimmed}`;
  }

  // Add minimal headers
  const headers = [
    `diff --git a/${filePath} b/${filePath}`,
    `--- a/${filePath}`,
    `+++ b/${filePath}`,
  ];
  return headers.join('\n') + '\n' + trimmed;
}

/**
 * Transform a backend FileMutation to FileDiffData for @pierre/diffs.
 * The DiffViewer component handles diff generation from content using the library.
 */
export function transformFileMutation(
  filePath: string,
  mutation: FileMutation
): FileDiffData {
  return {
    filePath: mutation.new_path || filePath,
    oldContent: mutation.old_content || '',
    newContent: mutation.new_content || mutation.content || '',
    diff: mutation.diff ? normalizeDiffString(mutation.diff, filePath) : undefined,
    additions: 0,
    deletions: 0,
  };
}

/**
 * Transform a git status diff string to FileDiffData.
 * Language detection is handled automatically by the library.
 */
export function transformGitDiff(
  filePath: string,
  diff: string,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  _status: 'A' | 'M' | 'D' | '??' | string
): FileDiffData {
  return {
    filePath,
    oldContent: '',
    newContent: '',
    diff: normalizeDiffString(diff, filePath),
    additions: 0,
    deletions: 0,
  };
}

/**
 * Convert DiffComment[] to @pierre/diffs DiffLineAnnotation[]
 */
export function commentsToAnnotations(
  comments: DiffComment[]
): CommentAnnotation[] {
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
  return line.startsWith('diff --git') || line.startsWith('---') || line.startsWith('+++');
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

function processDiffLine({ line, counters, startLine, endLine, side, resultLines }: ProcessDiffLineParams): void {
  if (line.startsWith('+')) {
    if (side === 'additions' && isInRange(counters.currentNewLine, startLine, endLine)) {
      resultLines.push(line.substring(1));
    }
    counters.currentNewLine++;
  } else if (line.startsWith('-')) {
    if (side === 'deletions' && isInRange(counters.currentOldLine, startLine, endLine)) {
      resultLines.push(line.substring(1));
    }
    counters.currentOldLine++;
  } else if (line.startsWith(' ') || (!line.startsWith('@') && line !== '')) {
    const content = line.startsWith(' ') ? line.substring(1) : line;
    const lineNum = side === 'additions' ? counters.currentNewLine : counters.currentOldLine;
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
  side: AnnotationSide
): string {
  const lines = diff.split('\n');
  const resultLines: string[] = [];
  const counters: DiffLineCounters = { currentOldLine: 0, currentNewLine: 0 };

  for (const line of lines) {
    if (processHunkHeader(line, counters)) continue;
    if (isDiffHeader(line)) continue;
    processDiffLine({ line, counters, startLine, endLine, side, resultLines });
  }

  return resultLines.join('\n');
}

/**
 * Extract code content from full file content for a line range
 */
export function extractCodeFromContent(
  content: string,
  startLine: number,
  endLine: number
): string {
  const lines = content.split('\n');
  return lines.slice(startLine - 1, endLine).join('\n');
}

/**
 * Compute simple line-level diff stats between two strings.
 * Returns the number of added and deleted lines.
 */
export function computeLineDiffStats(
  original: string,
  current: string,
): { additions: number; deletions: number } {
  const originalLines = original.split('\n');
  const currentLines = current.split('\n');
  let additions = 0;
  let deletions = 0;
  const maxLen = Math.max(originalLines.length, currentLines.length);
  for (let i = 0; i < maxLen; i++) {
    const origLine = originalLines[i];
    const currLine = currentLines[i];
    if (origLine === undefined && currLine !== undefined) additions++;
    else if (origLine !== undefined && currLine === undefined) deletions++;
    else if (origLine !== currLine) { additions++; deletions++; }
  }
  return { additions, deletions };
}
