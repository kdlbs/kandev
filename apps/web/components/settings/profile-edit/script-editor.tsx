"use client";

import { useEffect, useRef, useCallback } from "react";
import dynamic from "next/dynamic";
import type { BeforeMount, OnMount } from "@monaco-editor/react";
import { KANDEV_MONACO_DARK } from "@/lib/theme/editor-theme";
import type { ScriptPlaceholder } from "./script-editor-completions";

const MonacoEditor = dynamic(() => import("@monaco-editor/react").then((m) => m.default), {
  ssr: false,
  loading: () => (
    <div className="h-full w-full flex items-center justify-center text-muted-foreground text-xs border rounded-md bg-muted/20">
      Loading editor...
    </div>
  ),
});

type ScriptEditorProps = {
  value: string;
  onChange: (value: string) => void;
  language?: string;
  height?: string | number;
  placeholders?: ScriptPlaceholder[];
  executorType?: string;
  readOnly?: boolean;
  lineNumbers?: "on" | "off";
};

export function ScriptEditor({
  value,
  onChange,
  language = "shell",
  height = "300px",
  placeholders,
  executorType,
  readOnly = false,
  lineNumbers = "on",
}: ScriptEditorProps) {
  const disposableRef = useRef<{ dispose: () => void } | null>(null);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      disposableRef.current?.dispose();
    };
  }, []);

  const handleBeforeMount: BeforeMount = useCallback((monaco) => {
    monaco.editor.defineTheme("kandev-dark", KANDEV_MONACO_DARK);
  }, []);

  const handleMount: OnMount = useCallback(
    (editor, monaco) => {
      // Register placeholder completions if provided
      if (placeholders && placeholders.length > 0) {
        import("./script-editor-completions").then(({ createPlaceholderCompletionProvider }) => {
          disposableRef.current?.dispose();
          disposableRef.current = monaco.languages.registerCompletionItemProvider(
            language,
            createPlaceholderCompletionProvider(monaco, placeholders, executorType),
          );
        });
      }

      // Do not auto-focus — prevents unwanted scroll-to-editor on page load
    },
    [placeholders, executorType, language],
  );

  return (
    <MonacoEditor
      height={height}
      language={language}
      value={value}
      onChange={(v) => onChange(v ?? "")}
      beforeMount={handleBeforeMount}
      onMount={handleMount}
      theme="kandev-dark"
      options={{
        minimap: { enabled: false },
        lineNumbers,
        wordWrap: "on",
        fontSize: 13,
        scrollBeyondLastLine: false,
        readOnly,
        bracketPairColorization: { enabled: true },
        padding: { top: 8, bottom: 8 },
        renderLineHighlight: "none",
        overviewRulerLanes: 0,
        scrollbar: {
          vertical: "auto",
          horizontal: "auto",
          alwaysConsumeMouseWheel: false,
        },
      }}
    />
  );
}
