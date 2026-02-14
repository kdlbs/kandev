'use client';

import { useEditorProvider } from '@/hooks/use-editor-resolver';
import { MonacoCodeEditor } from '@/components/editors/monaco/monaco-code-editor';
import { CodeMirrorCodeEditor } from '@/components/editors/codemirror/codemirror-code-editor';

type FileEditorContentProps = {
  path: string;
  content: string;
  originalContent: string;
  isDirty: boolean;
  isSaving: boolean;
  sessionId?: string;
  worktreePath?: string;
  enableComments?: boolean;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onDelete?: () => void;
};

export function FileEditorContent(props: FileEditorContentProps) {
  const provider = useEditorProvider('code-editor');
  return provider === 'monaco'
    ? <MonacoCodeEditor {...props} />
    : <CodeMirrorCodeEditor {...props} />;
}
