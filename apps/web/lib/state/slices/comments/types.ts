/**
 * Unified comment system types.
 *
 * All comment types (diff, plan, file-editor) share a common base and are
 * distinguished via a `source` discriminant.
 */

// ---------------------------------------------------------------------------
// Comment union
// ---------------------------------------------------------------------------

export type AnnotationSide = 'additions' | 'deletions';

type CommentBase = {
  id: string;
  sessionId: string;
  text: string;
  createdAt: string;
  status: 'pending' | 'sent';
};

export type DiffComment = CommentBase & {
  source: 'diff';
  filePath: string;
  startLine: number;
  endLine: number;
  side: AnnotationSide;
  codeContent: string;
};

export type PlanComment = CommentBase & {
  source: 'plan';
  selectedText: string;
  from?: number;
  to?: number;
};

export type FileEditorComment = CommentBase & {
  source: 'file-editor';
  filePath: string;
  selectedText: string;
  startLine?: number;
  endLine?: number;
};

export type Comment = DiffComment | PlanComment | FileEditorComment;

// ---------------------------------------------------------------------------
// Type guards
// ---------------------------------------------------------------------------

export function isDiffComment(c: Comment): c is DiffComment {
  return c.source === 'diff';
}

export function isPlanComment(c: Comment): c is PlanComment {
  return c.source === 'plan';
}

export function isFileEditorComment(c: Comment): c is FileEditorComment {
  return c.source === 'file-editor';
}

// ---------------------------------------------------------------------------
// Store state & actions
// ---------------------------------------------------------------------------

export type CommentsState = {
  byId: Record<string, Comment>;
  bySession: Record<string, string[]>;
  pendingForChat: string[];
  editingCommentId: string | null;
};

export type CommentsActions = {
  addComment: (comment: Comment) => void;
  updateComment: (commentId: string, updates: Partial<Comment>) => void;
  removeComment: (commentId: string) => void;
  addToPending: (commentId: string) => void;
  removeFromPending: (commentId: string) => void;
  clearPending: () => void;
  setEditingComment: (commentId: string | null) => void;
  markCommentsSent: (commentIds: string[]) => void;
  clearSessionComments: (sessionId: string) => void;
  hydrateSession: (sessionId: string) => void;
  getCommentsForFile: (sessionId: string, filePath: string) => DiffComment[];
  getPendingComments: () => Comment[];
};

export type CommentsSlice = CommentsState & CommentsActions;
