"use client";

import type { MessageAttachment } from "@/lib/services/session-launch-service";
import { useEffect, useRef } from "react";
import type { editor as monacoEditor } from "monaco-editor";
import { createRoot, type Root } from "react-dom/client";
import type { DiffComment } from "@/lib/diff/types";
import { CommentForm } from "@/components/diff/comment-form";
import { CommentDisplay } from "@/components/diff/comment-display";
import { createElement } from "react";
import { observeCommentViewZone } from "@/hooks/observe-comment-view-zone";

type ViewZoneEntry = {
  id: string;
  root: Root;
  stopObserving: () => void;
  editorSide: "modified" | "original";
  renderWithSubmitDisabled?: (disabled: boolean) => void;
};

interface UseViewZonesParams {
  commentScope?: string;
  attachmentTaskId?: string | null;
  modifiedEditor: monacoEditor.ICodeEditor | null;
  originalEditor: monacoEditor.ICodeEditor | null;
  comments: DiffComment[];
  showCommentForm: boolean;
  submitDisabled: boolean;
  selectedLineRange: { start: number; end: number; side: string } | null;
  editingCommentId: string | null;
  setEditingComment: (id: string | null) => void;
  handleCommentSubmitRef: React.RefObject<
    (content: string, attachments?: MessageAttachment[]) => void
  >;
  handleCommentSubmitAndRunRef?: React.RefObject<
    ((content: string, attachments?: MessageAttachment[]) => void) | undefined
  >;
  handleCommentDeleteRef: React.RefObject<(commentId: string) => void>;
  handleCommentUpdateRef: React.RefObject<
    (commentId: string, content: string, attachments?: MessageAttachment[]) => void
  >;
  handleCommentRunRef?: React.RefObject<((comment: DiffComment) => void) | undefined>;
  clearModifiedGutter: () => void;
  clearOriginalGutter: () => void;
  setShowCommentForm: (v: boolean) => void;
  setSelectedLineRange: (v: null) => void;
}

interface AddZoneParams {
  targetEditor: monacoEditor.ICodeEditor;
  side: "modified" | "original";
  afterLine: number;
  heightPx: number;
  content: React.ReactNode;
  renderWithSubmitDisabled?: (disabled: boolean) => React.ReactNode;
  zones: ViewZoneEntry[];
}

function addZone({
  targetEditor,
  side,
  afterLine,
  heightPx,
  content,
  renderWithSubmitDisabled,
  zones,
}: AddZoneParams) {
  const domNode = document.createElement("div");
  domNode.style.zIndex = "10";
  const zoneContent = document.createElement("div");
  domNode.append(zoneContent);
  const root = createRoot(zoneContent);
  root.render(content);
  let zoneId = "";
  const zone = { afterLineNumber: afterLine, heightInPx: heightPx, domNode };
  targetEditor.changeViewZones((accessor) => {
    zoneId = accessor.addZone(zone);
  });
  zones.push({
    id: zoneId,
    root,
    stopObserving: observeCommentViewZone(targetEditor, zone, zoneId, zoneContent),
    editorSide: side,
    renderWithSubmitDisabled: renderWithSubmitDisabled
      ? (disabled) => root.render(renderWithSubmitDisabled(disabled))
      : undefined,
  });
}

function removeZones(
  zones: ViewZoneEntry[],
  modifiedEditor: monacoEditor.ICodeEditor,
  originalEditor: monacoEditor.ICodeEditor,
) {
  for (const zone of zones) zone.stopObserving();
  modifiedEditor.changeViewZones((accessor) => {
    for (const z of zones) {
      if (z.editorSide === "modified") accessor.removeZone(z.id);
    }
  });
  originalEditor.changeViewZones((accessor) => {
    for (const z of zones) {
      if (z.editorSide === "original") accessor.removeZone(z.id);
    }
  });
  const roots = zones.map((z) => z.root);
  queueMicrotask(() => roots.forEach((r) => r.unmount()));
}

type CreateZonesParams = {
  attachmentTaskId?: string | null;
  modifiedEditor: monacoEditor.ICodeEditor;
  originalEditor: monacoEditor.ICodeEditor;
  comments: DiffComment[];
  showCommentForm: boolean;
  submitDisabled: boolean;
  selectedLineRange: { start: number; end: number; side: string } | null;
  editingCommentId: string | null;
  setEditingComment: (id: string | null) => void;
  handleCommentSubmitRef: React.RefObject<
    (content: string, attachments?: MessageAttachment[]) => void
  >;
  handleCommentSubmitAndRunRef?: React.RefObject<
    ((content: string, attachments?: MessageAttachment[]) => void) | undefined
  >;
  handleCommentDeleteRef: React.RefObject<(commentId: string) => void>;
  handleCommentUpdateRef: React.RefObject<
    (commentId: string, content: string, attachments?: MessageAttachment[]) => void
  >;
  handleCommentRunRef?: React.RefObject<((comment: DiffComment) => void) | undefined>;
  clearModifiedGutter: () => void;
  clearOriginalGutter: () => void;
  setShowCommentForm: (v: boolean) => void;
  setSelectedLineRange: (v: null) => void;
};

function createCommentNode(
  comment: DiffComment,
  params: Pick<
    CreateZonesParams,
    | "attachmentTaskId"
    | "setEditingComment"
    | "handleCommentUpdateRef"
    | "handleCommentDeleteRef"
    | "handleCommentRunRef"
  >,
  isEditing: boolean,
) {
  if (isEditing) {
    return createElement(
      "div",
      { className: "px-2 py-0.5" },
      createElement(CommentForm, {
        initialContent: comment.text,
        attachmentOptions: params.attachmentTaskId
          ? { taskId: params.attachmentTaskId, initial: comment.attachments }
          : undefined,
        onSubmit: (c, attachments) =>
          params.handleCommentUpdateRef.current?.(comment.id, c, attachments),
        onCancel: () => params.setEditingComment(null),
        isEditing: true,
      }),
    );
  }
  return createElement(
    "div",
    { className: "px-2 py-0.5" },
    createElement(CommentDisplay, {
      comment,
      onDelete: () => params.handleCommentDeleteRef.current?.(comment.id),
      onEdit: () => params.setEditingComment(comment.id),
      onRun: params.handleCommentRunRef?.current
        ? () => params.handleCommentRunRef!.current?.(comment)
        : undefined,
      showCode: false,
      compact: true,
    }),
  );
}

function createZones(params: CreateZonesParams): ViewZoneEntry[] {
  const {
    modifiedEditor,
    originalEditor,
    comments,
    attachmentTaskId,
    showCommentForm,
    submitDisabled,
    selectedLineRange,
    editingCommentId,
    handleCommentSubmitRef,
    handleCommentSubmitAndRunRef,
    clearModifiedGutter,
    clearOriginalGutter,
    setShowCommentForm,
    setSelectedLineRange,
  } = params;
  const zones: ViewZoneEntry[] = [];

  for (const comment of comments) {
    const side = comment.side === "deletions" ? "original" : "modified";
    const editor = side === "modified" ? modifiedEditor : originalEditor;
    const isEditing = editingCommentId === comment.id;
    const node = createCommentNode(comment, params, isEditing);
    addZone({
      targetEditor: editor,
      side,
      afterLine: comment.endLine,
      heightPx: isEditing ? 120 : 32,
      content: node,
      zones,
    });
  }

  if (showCommentForm && selectedLineRange) {
    const side = selectedLineRange.side === "deletions" ? "original" : "modified";
    const editor = side === "modified" ? modifiedEditor : originalEditor;
    const renderWithSubmitDisabled = (disabled: boolean) =>
      createElement(
        "div",
        { className: "px-2 py-1" },
        createElement(CommentForm, {
          attachmentOptions: attachmentTaskId ? { taskId: attachmentTaskId } : undefined,
          onSubmit: (c, attachments) =>
            handleCommentSubmitRef.current?.(c, ...(attachments ? [attachments] : [])),
          onSubmitAndRun: handleCommentSubmitAndRunRef?.current
            ? (c, attachments) =>
                handleCommentSubmitAndRunRef.current?.(c, ...(attachments ? [attachments] : []))
            : undefined,
          submitDisabled: disabled,
          onCancel: () => {
            setShowCommentForm(false);
            setSelectedLineRange(null);
            clearModifiedGutter();
            clearOriginalGutter();
          },
        }),
      );
    addZone({
      targetEditor: editor,
      side,
      afterLine: Math.max(selectedLineRange.start, selectedLineRange.end),
      heightPx: 120,
      content: renderWithSubmitDisabled(submitDisabled),
      renderWithSubmitDisabled,
      zones,
    });
  }

  return zones;
}

/** Manages inline ViewZones for comments and comment forms */
export function useViewZones({
  attachmentTaskId,
  commentScope,
  modifiedEditor,
  originalEditor,
  comments,
  showCommentForm,
  submitDisabled,
  selectedLineRange,
  editingCommentId,
  setEditingComment,
  handleCommentSubmitRef,
  handleCommentSubmitAndRunRef,
  handleCommentDeleteRef,
  handleCommentUpdateRef,
  handleCommentRunRef,
  clearModifiedGutter,
  clearOriginalGutter,
  setShowCommentForm,
  setSelectedLineRange,
}: UseViewZonesParams) {
  const viewZonesRef = useRef<ViewZoneEntry[]>([]);

  useEffect(() => {
    if (!modifiedEditor || !originalEditor) return;

    const oldZones = viewZonesRef.current;
    if (oldZones.length > 0) {
      removeZones(oldZones, modifiedEditor, originalEditor);
      viewZonesRef.current = [];
    }

    const newZones = createZones({
      attachmentTaskId,
      modifiedEditor,
      originalEditor,
      comments,
      showCommentForm,
      submitDisabled,
      selectedLineRange,
      editingCommentId,
      setEditingComment,
      handleCommentSubmitRef,
      handleCommentSubmitAndRunRef,
      handleCommentDeleteRef,
      handleCommentUpdateRef,
      handleCommentRunRef,
      clearModifiedGutter,
      clearOriginalGutter,
      setShowCommentForm,
      setSelectedLineRange,
    });

    viewZonesRef.current = newZones;

    return () => {
      try {
        removeZones(newZones, modifiedEditor, originalEditor);
      } catch {
        /* editors may be disposed */
      }
    };
    // Submit readiness is updated in place below so open comment drafts survive refreshes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    attachmentTaskId,
    commentScope,
    modifiedEditor,
    originalEditor,
    comments,
    showCommentForm,
    selectedLineRange,
    editingCommentId,
  ]);

  useEffect(() => {
    for (const zone of viewZonesRef.current) {
      zone.renderWithSubmitDisabled?.(submitDisabled);
    }
  }, [submitDisabled]);
}
