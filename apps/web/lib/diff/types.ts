import type { DiffLineAnnotation } from '@pierre/diffs';

/**
 * Annotation side - matches @pierre/diffs AnnotationSide
 */
export type AnnotationSide = 'additions' | 'deletions';

/**
 * Comment attached to a line range in a diff
 */
export interface DiffComment {
  id: string;
  sessionId: string;
  filePath: string;
  startLine: number;
  endLine: number;
  side: AnnotationSide;
  codeContent: string; // The actual code lines the user selected
  annotation: string; // User's comment text
  createdAt: string;
  status: 'pending' | 'sent';
}

/**
 * File diff data in a unified format for @pierre/diffs.
 * Language is auto-detected by the library from file extension.
 */
export interface FileDiffData {
  filePath: string;
  oldContent: string;
  newContent: string;
  diff?: string; // Unified diff string (for patch mode)
  additions: number;
  deletions: number;
}

/**
 * @pierre/diffs annotation type with our comment metadata
 */
export type CommentAnnotation = DiffLineAnnotation<{
  comment: DiffComment;
  isEditing: boolean;
}>;

/**
 * Store state for diff comments
 */
export interface DiffCommentsState {
  // Comments by session and file: sessionId -> filePath -> comments
  bySession: Record<string, Record<string, DiffComment[]>>;
  // Comments queued for next chat message (referenced by id)
  pendingForChat: string[];
  // Currently editing comment (null if none)
  editingCommentId: string | null;
}

/**
 * Actions for the diff comments store
 */
export interface DiffCommentsActions {
  addComment: (comment: DiffComment) => void;
  updateComment: (commentId: string, updates: Partial<DiffComment>) => void;
  removeComment: (sessionId: string, filePath: string, commentId: string) => void;
  addToPending: (commentId: string) => void;
  removeFromPending: (commentId: string) => void;
  clearPending: () => void;
  setEditingComment: (commentId: string | null) => void;
  markCommentsSent: (commentIds: string[]) => void;
  getCommentsForFile: (sessionId: string, filePath: string) => DiffComment[];
  getPendingComments: () => DiffComment[];
  clearSessionComments: (sessionId: string) => void;
}

/**
 * Combined state and actions
 */
export type DiffCommentsSlice = DiffCommentsState & DiffCommentsActions;

/**
 * Rich text input block type for comment blocks
 */
export interface CommentBlockData {
  type: 'comment-block';
  filePath: string;
  commentIds: string[];
}

/**
 * Props for DiffViewer component
 */
export interface DiffViewerProps {
  /** Diff data to display */
  data: FileDiffData;
  /** View mode: split or unified */
  viewMode?: 'split' | 'unified';
  /** Enable line selection for comments */
  enableComments?: boolean;
  /** Session ID for comment storage */
  sessionId?: string;
  /** Callback when comment is added */
  onCommentAdd?: (comment: DiffComment) => void;
  /** Callback when comment is deleted */
  onCommentDelete?: (commentId: string) => void;
  /** External comments (controlled mode) */
  comments?: DiffComment[];
  /** Additional class name */
  className?: string;
  /** Whether to show in compact mode (for chat) */
  compact?: boolean;
}

/**
 * Props for inline diff view in chat messages
 */
export interface DiffViewInlineProps {
  /** Diff data to display */
  data: FileDiffData;
  /** Session ID for comment storage */
  sessionId?: string;
  /** Enable comments */
  enableComments?: boolean;
  /** Additional class name */
  className?: string;
}
