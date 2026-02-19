"use client";

import { useEffect, useRef } from "react";
import type { editor as monacoEditor } from "monaco-editor";
import { createRoot, type Root } from "react-dom/client";
import type { DiffComment } from "@/lib/diff/types";
import { CommentForm } from "@/components/diff/comment-form";
import { CommentDisplay } from "@/components/diff/comment-display";
import { createElement } from "react";

type ViewZoneEntry = { id: string; root: Root; editorSide: "modified" | "original" };

interface UseViewZonesParams {
  modifiedEditor: monacoEditor.ICodeEditor | null;
  originalEditor: monacoEditor.ICodeEditor | null;
  comments: DiffComment[];
  showCommentForm: boolean;
  selectedLineRange: { start: number; end: number; side: string } | null;
  editingCommentId: string | null;
  setEditingComment: (id: string | null) => void;
  handleCommentSubmitRef: React.RefObject<(content: string) => void>;
  handleCommentDeleteRef: React.RefObject<(commentId: string) => void>;
  handleCommentUpdateRef: React.RefObject<(commentId: string, content: string) => void>;
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
  zones: ViewZoneEntry[];
}

function addZone({ targetEditor, side, afterLine, heightPx, content, zones }: AddZoneParams) {
  const domNode = document.createElement("div");
  domNode.style.zIndex = "10";
  const root = createRoot(domNode);
  root.render(content);
  let zoneId = "";
  targetEditor.changeViewZones((accessor) => {
    zoneId = accessor.addZone({ afterLineNumber: afterLine, heightInPx: heightPx, domNode });
  });
  zones.push({ id: zoneId, root, editorSide: side });
}

function removeZones(
  zones: ViewZoneEntry[],
  modifiedEditor: monacoEditor.ICodeEditor,
  originalEditor: monacoEditor.ICodeEditor,
) {
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
  modifiedEditor: monacoEditor.ICodeEditor;
  originalEditor: monacoEditor.ICodeEditor;
  comments: DiffComment[];
  showCommentForm: boolean;
  selectedLineRange: { start: number; end: number; side: string } | null;
  editingCommentId: string | null;
  setEditingComment: (id: string | null) => void;
  handleCommentSubmitRef: React.RefObject<(content: string) => void>;
  handleCommentDeleteRef: React.RefObject<(commentId: string) => void>;
  handleCommentUpdateRef: React.RefObject<(commentId: string, content: string) => void>;
  clearModifiedGutter: () => void;
  clearOriginalGutter: () => void;
  setShowCommentForm: (v: boolean) => void;
  setSelectedLineRange: (v: null) => void;
};

function createZones(params: CreateZonesParams): ViewZoneEntry[] {
  const {
    modifiedEditor,
    originalEditor,
    comments,
    showCommentForm,
    selectedLineRange,
    editingCommentId,
    setEditingComment,
    handleCommentSubmitRef,
    handleCommentDeleteRef,
    handleCommentUpdateRef,
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
    const node = isEditing
      ? createElement(
          "div",
          { className: "px-2 py-0.5" },
          createElement(CommentForm, {
            initialContent: comment.text,
            onSubmit: (c: string) => handleCommentUpdateRef.current?.(comment.id, c),
            onCancel: () => setEditingComment(null),
            isEditing: true,
          }),
        )
      : createElement(
          "div",
          { className: "px-2 py-0.5" },
          createElement(CommentDisplay, {
            comment,
            onDelete: () => handleCommentDeleteRef.current?.(comment.id),
            onEdit: () => setEditingComment(comment.id),
            showCode: false,
            compact: true,
          }),
        );
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
    const node = createElement(
      "div",
      { className: "px-2 py-1" },
      createElement(CommentForm, {
        onSubmit: (c: string) => handleCommentSubmitRef.current?.(c),
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
      content: node,
      zones,
    });
  }

  return zones;
}

/** Manages inline ViewZones for comments and comment forms */
export function useViewZones({
  modifiedEditor,
  originalEditor,
  comments,
  showCommentForm,
  selectedLineRange,
  editingCommentId,
  setEditingComment,
  handleCommentSubmitRef,
  handleCommentDeleteRef,
  handleCommentUpdateRef,
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
      modifiedEditor,
      originalEditor,
      comments,
      showCommentForm,
      selectedLineRange,
      editingCommentId,
      setEditingComment,
      handleCommentSubmitRef,
      handleCommentDeleteRef,
      handleCommentUpdateRef,
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    modifiedEditor,
    originalEditor,
    comments,
    showCommentForm,
    selectedLineRange,
    editingCommentId,
  ]);
}
