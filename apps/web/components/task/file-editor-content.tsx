"use client";

import { memo } from "react";
import { useEditorProvider } from "@/hooks/use-editor-resolver";
import { MonacoCodeEditor } from "@/components/editors/monaco/monaco-code-editor";
import { CodeMirrorCodeEditor } from "@/components/editors/codemirror/codemirror-code-editor";
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
  markdownPreview?: boolean;
  onToggleMarkdownPreview?: () => void;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
};

export const FileEditorContent = memo(function FileEditorContent(props: FileEditorContentProps) {
  const provider = useEditorProvider("code-editor");

  if (props.markdownPreview && props.onToggleMarkdownPreview) {
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
        onTogglePreview={props.onToggleMarkdownPreview}
      />
    );
  }

  return provider === "monaco" ? (
    <MonacoCodeEditor {...props} />
  ) : (
    <CodeMirrorCodeEditor {...props} />
  );
});
