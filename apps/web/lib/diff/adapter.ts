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
  let currentNewLine = 0;
  let currentOldLine = 0;

  for (const line of lines) {
    // Parse hunk header
    const hunkMatch = line.match(/^@@\s*-(\d+)(?:,\d+)?\s*\+(\d+)(?:,\d+)?\s*@@/);
    if (hunkMatch) {
      currentOldLine = parseInt(hunkMatch[1], 10);
      currentNewLine = parseInt(hunkMatch[2], 10);
      continue;
    }

    // Skip file headers
    if (line.startsWith('diff --git') || line.startsWith('---') || line.startsWith('+++')) {
      continue;
    }

    if (line.startsWith('+') && !line.startsWith('+++')) {
      if (side === 'additions' && currentNewLine >= startLine && currentNewLine <= endLine) {
        resultLines.push(line.substring(1));
      }
      currentNewLine++;
    } else if (line.startsWith('-') && !line.startsWith('---')) {
      if (side === 'deletions' && currentOldLine >= startLine && currentOldLine <= endLine) {
        resultLines.push(line.substring(1));
      }
      currentOldLine++;
    } else if (line.startsWith(' ') || (!line.startsWith('@') && line !== '')) {
      const content = line.startsWith(' ') ? line.substring(1) : line;
      const lineNum = side === 'additions' ? currentNewLine : currentOldLine;
      if (lineNum >= startLine && lineNum <= endLine) {
        resultLines.push(content);
      }
      currentOldLine++;
      currentNewLine++;
    }
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
