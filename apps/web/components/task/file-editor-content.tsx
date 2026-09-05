"use client";

import { memo } from "react";
import { useEditorProvider } from "@/hooks/use-editor-resolver";
import { MonacoCodeEditor } from "@/components/editors/monaco/monaco-code-editor";
import { CodeMirrorCodeEditor } from "@/components/editors/codemirror/codemirror-code-editor";
import type { FilePreviewKind } from "@/lib/utils/file-types";
import { MarkdownPreviewContent } from "./markdown-preview-content";

export type FileEditorContentProps = {
  path: string;
  content: string;
  originalContent: string;
  isDirty: boolean;
  hasRemoteUpdate?: boolean;
  vcsDiff?: string;
  isSaving: boolean;
  sessionId?: string;
  taskId?: string | null;
  repositoryId?: string | null;
  worktreePath?: string;
  repo?: string;
  enableComments?: boolean;
  previewKind?: FilePreviewKind;
  renderedPreview?: boolean;
  onTogglePreview?: () => void;
  onPreviewHtml?: () => void;
  isPublishingHtmlPreview?: boolean;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
  onDownload?: () => void;
};

export const FileEditorContent = memo(function FileEditorContent(props: FileEditorContentProps) {
  const provider = useEditorProvider("code-editor");

  if (props.renderedPreview && props.previewKind === "markdown" && props.onTogglePreview) {
    return (
      <MarkdownPreviewContent
        path={props.path}
        content={props.content}
        worktreePath={props.worktreePath}
        sessionId={props.sessionId}
        taskId={props.taskId}
        repositoryId={props.repositoryId}
        repositoryName={props.repo}
        enableComments={props.enableComments}
        onDownload={props.onDownload}
        onTogglePreview={props.onTogglePreview}
      />
    );
  }

  return provider === "monaco" ? (
    <MonacoCodeEditor {...props} />
  ) : (
    <CodeMirrorCodeEditor {...props} />
  );
});
