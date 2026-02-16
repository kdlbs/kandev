'use client';

import { useEditorProvider } from '@/hooks/use-editor-resolver';
import { MonacoCodeBlock } from '@/components/editors/monaco/monaco-code-block';
import { CodeMirrorCodeBlock } from '@/components/editors/codemirror/codemirror-code-block';

type CodeBlockProps = {
  children: React.ReactNode;
  className?: string;
};

export function CodeBlock(props: CodeBlockProps) {
  const provider = useEditorProvider('chat-code-block');
  return provider === 'monaco'
    ? <MonacoCodeBlock {...props} />
    : <CodeMirrorCodeBlock {...props} />;
}
