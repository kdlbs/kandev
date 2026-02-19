"use client";

import { memo } from "react";
import { useEditorProvider } from "@/hooks/use-editor-resolver";
import { MonacoCodeEditor } from "@/components/editors/monaco/monaco-code-editor";
import { CodeMirrorCodeEditor } from "@/components/editors/codemirror/codemirror-code-editor";

export type FileEditorContentProps = {
  path: string;
  content: string;
  originalContent: string;
  isDirty: boolean;
  hasRemoteUpdate?: boolean;
  vcsDiff?: string;
  isSaving: boolean;
  sessionId?: string;
  worktreePath?: string;
  enableComments?: boolean;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
};

export const FileEditorContent = memo(function FileEditorContent(props: FileEditorContentProps) {
  const provider = useEditorProvider("code-editor");
  return provider === "monaco" ? (
    <MonacoCodeEditor {...props} />
  ) : (
    <CodeMirrorCodeEditor {...props} />
  );
});
