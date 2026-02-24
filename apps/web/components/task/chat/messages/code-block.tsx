"use client";

import { useEditorProvider } from "@/hooks/use-editor-resolver";
import { MonacoCodeBlock } from "@/components/editors/monaco/monaco-code-block";
import { CodeMirrorCodeBlock } from "@/components/editors/codemirror/codemirror-code-block";
import { ShikiCodeBlock } from "@/components/editors/shiki/shiki-code-block";

type CodeBlockProps = {
  children: React.ReactNode;
  className?: string;
};

export function CodeBlock(props: CodeBlockProps) {
  const provider = useEditorProvider("chat-code-block");
  if (provider === "monaco") return <MonacoCodeBlock {...props} />;
  if (provider === "shiki") return <ShikiCodeBlock {...props} />;
  return <CodeMirrorCodeBlock {...props} />;
}
