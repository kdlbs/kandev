import type { SplitSide, DiffFile } from '@git-diff-view/react';

/**
 * Comment attached to a line range in a diff
 */
export interface DiffComment {
  id: string;
  startLine: number;
  endLine: number;
  side: SplitSide;
  content: string;
  createdAt: string;
}

/**
 * Extended line data for the library's extendData prop
 */
export interface ExtendLineData {
  type: 'comment';
  comment: DiffComment;
}

/**
 * Props for the main GitDiffViewer component
 */
export interface GitDiffViewerProps {
  /** Old file content */
  oldContent: string;
  /** New file content */
  newContent: string;
  /** File path (used for comment storage key) */
  filePath: string;
  /** File language for syntax highlighting (e.g., 'tsx', 'typescript') */
  language?: string;
  /** Initial view mode */
  defaultViewMode?: 'split' | 'unified';
  /** Whether to show the toolbar */
  showToolbar?: boolean;
  /** Whether to enable comments */
  enableComments?: boolean;
  /** External comments (if managing state externally) */
  comments?: DiffComment[];
  /** Callback when a comment is added */
  onCommentAdd?: (comment: DiffComment) => void;
  /** Callback when a comment is deleted */
  onCommentDelete?: (commentId: string) => void;
  /** Additional class name for the container */
  className?: string;
}

/**
 * Selection state during drag
 */
export interface DragSelectionState {
  startLine: number;
  endLine: number;
  side: SplitSide;
  isActive: boolean;
}

/**
 * Props for comment widget components
 */
export interface CommentWidgetProps {
  lineNumber: number;
  side: SplitSide;
  startLine: number;
  diffFile: DiffFile;
  wrapperRef: React.RefObject<HTMLDivElement | null>;
  viewMode: 'split' | 'unified';
  filePath: string;
  onSave: (comment: DiffComment) => void;
  onCancel: () => void;
  onStartLineChange: (startLine: number) => void;
}

export { SplitSide };
