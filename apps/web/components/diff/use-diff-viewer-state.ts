import { useState, useCallback, useEffect, useMemo, useRef, type RefObject } from "react";
import type {
  SelectedLineRange,
  FileDiffMetadata,
  DiffLineAnnotation,
  AnnotationSide,
} from "@pierre/diffs";
import type { FileDiffData, DiffComment } from "@/lib/diff/types";
import { buildDiffComment, useCommentActions } from "@/lib/diff/comment-utils";
import { useDiffComments } from "./use-diff-comments";
import { useDiffMetadata } from "./use-diff-metadata";
import { useExpandableDiff } from "./use-expandable-diff";
import type { RevertBlockInfo } from "./diff-viewer";
import type { AnnotationMetadata } from "./use-diff-annotation-renderer";

type BuildAnnotationsOpts = {
  comments: DiffComment[];
  editingCommentId: string | null;
  showCommentForm: boolean;
  selectedLines: SelectedLineRange | null;
  enableAcceptReject: boolean;
  fileDiffMetadata: FileDiffMetadata | null;
};

type HunkState = {
  addLine: number;
  delLine: number;
  lastCtxAdd: number;
  lastCtxDel: number;
  blockIdx: number;
};

/** Process a single change block within a hunk. */
function processChangeBlock(
  content: { additions: string[]; deletions: string[] },
  state: HunkState,
  result: DiffLineAnnotation<AnnotationMetadata>[],
  newLineMap: Map<string, string>,
  newRevertMap: Map<string, RevertBlockInfo>,
) {
  const aLen = content.additions.length;
  const dLen = content.deletions.length;
  if (aLen === 0 && dLen === 0) return;

  const cbId = `cb-${state.blockIdx++}`;
  const side: AnnotationSide = aLen > 0 ? "additions" : "deletions";
  const lineNumber = side === "additions" ? state.lastCtxAdd : state.lastCtxDel;
  result.push({ side, lineNumber, metadata: { type: "hunk-actions", changeBlockId: cbId } });
  for (let l = 0; l < aLen; l++) newLineMap.set(`additions:${state.addLine + l}`, cbId);
  for (let l = 0; l < dLen; l++) newLineMap.set(`deletions:${state.delLine + l}`, cbId);
  newRevertMap.set(cbId, {
    addStart: state.addLine,
    addCount: aLen,
    oldLines: content.deletions.map((l) => l.replace(/\r?\n$/, "")),
  });
}

/** Build hunk-level annotations, line->changeBlock map, and revert info. */
function buildHunkAnnotations(
  fileDiffMetadata: FileDiffMetadata,
  result: DiffLineAnnotation<AnnotationMetadata>[],
  newLineMap: Map<string, string>,
  newRevertMap: Map<string, RevertBlockInfo>,
) {
  const state: HunkState = { addLine: 0, delLine: 0, lastCtxAdd: 0, lastCtxDel: 0, blockIdx: 0 };
  for (const hunk of fileDiffMetadata.hunks) {
    if (hunk.additionCount === 0 && hunk.deletionCount === 0) continue;
    state.addLine = hunk.additionStart;
    state.delLine = hunk.deletionStart;
    state.lastCtxAdd = state.addLine > 1 ? state.addLine - 1 : state.addLine;
    state.lastCtxDel = state.delLine > 1 ? state.delLine - 1 : state.delLine;
    for (const content of hunk.hunkContent) {
      if (content.type === "context") {
        const len = content.lines.length;
        state.lastCtxAdd = state.addLine + len - 1;
        state.lastCtxDel = state.delLine + len - 1;
        state.addLine += len;
        state.delLine += len;
        continue;
      }
      processChangeBlock(content, state, result, newLineMap, newRevertMap);
      state.addLine += content.additions.length;
      state.delLine += content.deletions.length;
    }
  }
}

/** Build all diff line annotations (comments, new-comment form, hunk actions). */
function buildAnnotations(opts: BuildAnnotationsOpts) {
  const {
    comments,
    editingCommentId,
    showCommentForm,
    selectedLines,
    enableAcceptReject,
    fileDiffMetadata,
  } = opts;
  const result: DiffLineAnnotation<AnnotationMetadata>[] = comments.map((comment) => ({
    side: comment.side,
    lineNumber: comment.endLine,
    metadata: { type: "comment" as const, comment, isEditing: editingCommentId === comment.id },
  }));

  if (showCommentForm && selectedLines) {
    result.push({
      side: (selectedLines.side || "additions") as AnnotationSide,
      lineNumber: Math.max(selectedLines.start, selectedLines.end),
      metadata: { type: "new-comment-form" as const },
    });
  }

  const newLineMap = new Map<string, string>();
  const newRevertMap = new Map<string, RevertBlockInfo>();
  if (enableAcceptReject && fileDiffMetadata) {
    buildHunkAnnotations(fileDiffMetadata, result, newLineMap, newRevertMap);
  }

  return { annotations: result, lineMap: newLineMap, revertMap: newRevertMap };
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

type UseDiffViewerStateOpts = {
  data: FileDiffData;
  enableComments: boolean;
  enableAcceptReject: boolean;
  sessionId?: string;
  onCommentAdd?: (comment: DiffComment) => void;
  onCommentDelete?: (commentId: string) => void;
  onCommentRun?: (comment: DiffComment) => void;
  externalComments?: DiffComment[];
  onRevertBlock?: (filePath: string, info: RevertBlockInfo) => Promise<void> | void;
  /** Enable diff expansion (requires fetching full file content) */
  enableExpansion?: boolean;
  /** Base git ref for fetching old content (e.g., "origin/main", "HEAD~1") */
  baseRef?: string;
  /** Multi-repo subpath for the file (e.g. "kandev"); empty for single-repo. */
  repo?: string;
};

function useDiffViewerAnnotations({
  comments,
  editingCommentId,
  showCommentForm,
  selectedLines,
  enableAcceptReject,
  fileDiffMetadata,
  changeLineMapRef,
  revertInfoRef,
}: BuildAnnotationsOpts & {
  changeLineMapRef: RefObject<Map<string, string>>;
  revertInfoRef: RefObject<Map<string, RevertBlockInfo>>;
}) {
  const { annotations, lineMap, revertMap } = useMemo(
    () =>
      buildAnnotations({
        comments,
        editingCommentId,
        showCommentForm,
        selectedLines,
        enableAcceptReject,
        fileDiffMetadata,
      }),
    [
      comments,
      editingCommentId,
      showCommentForm,
      selectedLines,
      enableAcceptReject,
      fileDiffMetadata,
    ],
  );

  useEffect(() => {
    changeLineMapRef.current = lineMap;
    revertInfoRef.current = revertMap;
  }, [lineMap, revertMap, changeLineMapRef, revertInfoRef]);

  return annotations;
}

type CommentHandlerOpts = {
  selectedLines: SelectedLineRange | null;
  setSelectedLines: React.Dispatch<React.SetStateAction<SelectedLineRange | null>>;
  setShowCommentForm: React.Dispatch<React.SetStateAction<boolean>>;
  enableComments: boolean;
  onCommentAdd?: (comment: DiffComment) => void;
  onCommentRun?: (comment: DiffComment) => void;
  externalComments?: DiffComment[];
  data: FileDiffData;
  sessionId?: string;
  addComment: (range: SelectedLineRange, content: string) => DiffComment;
  removeComment: (commentId: string) => void;
  updateComment: (commentId: string, updates: Partial<DiffComment>) => void;
  setEditingComment: (commentId: string | null) => void;
  onCommentDelete?: (commentId: string) => void;
};

function useDiffViewerCommentHandlers(opts: CommentHandlerOpts) {
  const {
    selectedLines,
    setSelectedLines,
    setShowCommentForm,
    enableComments,
    onCommentAdd,
    onCommentRun,
    externalComments,
    data,
    sessionId,
    addComment,
    removeComment,
    updateComment,
    setEditingComment,
    onCommentDelete,
  } = opts;
  const handleLineSelectionEnd = useCallback(
    (range: SelectedLineRange | null) => {
      setSelectedLines(range);
      if (range && enableComments) setShowCommentForm(true);
    },
    [enableComments, setSelectedLines, setShowCommentForm],
  );

  const createCommentFromSelection = useCallback(
    (content: string): DiffComment | null => {
      if (!selectedLines) return null;
      return buildDiffComment({
        filePath: data.filePath,
        sessionId: sessionId || "",
        startLine: selectedLines.start,
        endLine: selectedLines.end,
        side: (selectedLines.side || "additions") as DiffComment["side"],
        text: content,
      });
    },
    [selectedLines, data.filePath, sessionId],
  );

  const submitComment = useCallback(
    (content: string, runAfter?: (c: DiffComment) => void) => {
      if (!selectedLines) return;
      if (onCommentAdd && externalComments !== undefined) {
        const comment = createCommentFromSelection(content);
        if (comment) {
          onCommentAdd(comment);
          runAfter?.(comment);
        }
      } else if (sessionId) {
        const stored = addComment(selectedLines, content);
        if (runAfter && stored) runAfter(stored);
      }
      setShowCommentForm(false);
      setSelectedLines(null);
    },
    [
      selectedLines,
      onCommentAdd,
      externalComments,
      sessionId,
      addComment,
      createCommentFromSelection,
      setShowCommentForm,
      setSelectedLines,
    ],
  );

  const handleCommentSubmit = useCallback(
    (content: string) => submitComment(content),
    [submitComment],
  );

  const handleCommentSubmitAndRun = useCallback(
    (content: string) => submitComment(content, onCommentRun),
    [submitComment, onCommentRun],
  );

  const { handleCommentDelete, handleCommentUpdate } = useCommentActions({
    removeComment,
    updateComment,
    setEditingComment,
    onCommentDelete,
    externalComments,
  });

  return {
    handleLineSelectionEnd,
    handleCommentSubmit,
    handleCommentSubmitAndRun: onCommentRun ? handleCommentSubmitAndRun : undefined,
    handleCommentDelete,
    handleCommentUpdate,
  };
}

function useRevertBlock(
  filePath: string,
  onRevertBlock?: (filePath: string, info: RevertBlockInfo) => Promise<void> | void,
) {
  const revertInfoRef = useRef<Map<string, RevertBlockInfo>>(new Map());
  const handleRevertBlock = useCallback(
    async (changeBlockId: string) => {
      const info = revertInfoRef.current.get(changeBlockId);
      if (!info) return;
      await onRevertBlock?.(filePath, info);
    },
    [filePath, onRevertBlock],
  );
  return { revertInfoRef, handleRevertBlock };
}

export function useDiffViewerState(opts: UseDiffViewerStateOpts) {
  const {
    data,
    enableComments,
    enableAcceptReject,
    sessionId,
    onCommentAdd,
    onCommentDelete,
    onCommentRun,
    externalComments,
    onRevertBlock,
    enableExpansion = false,
    baseRef,
    repo,
  } = opts;

  const [selectedLines, setSelectedLines] = useState<SelectedLineRange | null>(null);
  const [showCommentForm, setShowCommentForm] = useState(false);

  const {
    comments: internalComments,
    addComment,
    removeComment,
    updateComment,
    editingCommentId,
    setEditingComment,
  } = useDiffComments({
    sessionId: sessionId || "",
    filePath: data.filePath,
    diff: data.diff,
    newContent: data.newContent,
    oldContent: data.oldContent,
  });

  const comments = externalComments || internalComments;
  const baseDiffMetadata = useDiffMetadata(data);
  const expansion = useExpandableDiff({
    sessionId,
    filePath: data.filePath,
    baseRef,
    fileDiffMetadata: baseDiffMetadata,
    enableExpansion,
    repo,
  });

  const { revertInfoRef, handleRevertBlock } = useRevertBlock(data.filePath, onRevertBlock);
  const changeLineMapRef = useRef<Map<string, string>>(new Map());
  const hideTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const annotations = useDiffViewerAnnotations({
    comments,
    editingCommentId,
    showCommentForm,
    selectedLines,
    enableAcceptReject,
    fileDiffMetadata: expansion.metadata,
    changeLineMapRef,
    revertInfoRef,
  });

  const commentHandlers = useDiffViewerCommentHandlers({
    selectedLines,
    setSelectedLines,
    setShowCommentForm,
    enableComments,
    onCommentAdd,
    onCommentRun,
    externalComments,
    data,
    sessionId,
    addComment,
    removeComment,
    updateComment,
    setEditingComment,
    onCommentDelete,
  });

  return {
    comments,
    fileDiffMetadata: expansion.metadata,
    annotations,
    selectedLines,
    showCommentForm,
    setShowCommentForm,
    setSelectedLines,
    editingCommentId,
    setEditingComment,
    handleRevertBlock,
    ...commentHandlers,
    changeLineMapRef,
    hideTimeoutRef,
    isExpansionContentLoaded: expansion.isContentLoaded,
    isExpansionLoading: expansion.isLoading,
    expansionError: expansion.error,
    loadExpansionContent: expansion.loadContent,
    canExpand: expansion.canExpand,
  };
}
