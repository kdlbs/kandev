// Types
export type {
  AnnotationSide,
  DiffComment,
  FileDiffData,
  CommentAnnotation,
  DiffCommentsState,
  DiffCommentsActions,
  DiffCommentsSlice,
  CommentBlockData,
  DiffViewerProps,
  DiffViewInlineProps,
} from './types';

// Adapter functions
export {
  formatLineRange,
  transformFileMutation,
  transformGitDiff,
  commentsToAnnotations,
  normalizeDiffString,
  extractCodeFromDiff,
  extractCodeFromContent,
  type FileMutation,
  type ModifyFilePayload,
} from './adapter';
