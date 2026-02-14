// Main components - resolver-aware
export { DiffViewerResolved as DiffViewer } from './diff-viewer-resolver';
export { DiffViewInlineResolved as DiffViewInline } from './diff-viewer-resolver';
export { FileDiffViewer } from './file-diff-viewer';
export { CommentForm } from './comment-form';
export { CommentDisplay } from './comment-display';

// Direct access to Pierre implementation (for internal use)
export { DiffViewer as PierreDiffViewer } from './diff-viewer';
export { DiffViewInline as PierreDiffViewInline } from './diff-viewer';

// Hooks
export { useDiffComments } from './use-diff-comments';
