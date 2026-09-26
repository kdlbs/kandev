"use client";

import { lazy, Suspense, type ReactNode } from "react";
import { useEditorProvider } from "@/hooks/use-editor-resolver";
import { CodeMirrorCodeBlock } from "@/components/editors/codemirror/codemirror-code-block";
import { ShikiCodeBlock } from "@/components/editors/shiki/shiki-code-block";

const LazyMonacoCodeBlock = lazy(async () => {
  const module = await import("@/components/editors/monaco/monaco-code-block");
  return { default: module.MonacoCodeBlock };
});

type CodeBlockProps = {
  children: ReactNode;
  className?: string;
};

export function CodeBlock(props: CodeBlockProps) {
  const provider = useEditorProvider("chat-code-block");
  if (provider === "monaco") {
    return (
      <Suspense fallback={<CodeMirrorCodeBlock {...props} />}>
        <LazyMonacoCodeBlock {...props} />
      </Suspense>
    );
  }
  if (provider === "shiki") return <ShikiCodeBlock {...props} />;
  return <CodeMirrorCodeBlock {...props} />;
}
