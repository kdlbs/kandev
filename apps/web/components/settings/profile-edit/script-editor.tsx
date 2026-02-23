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

// Module-level singleton: only one completion provider registered for "shell" at a time.
let globalDisposable: { dispose: () => void } | null = null;
let registeredInstanceCount = 0;

function registerProvider(
  monaco: typeof import("monaco-editor"),
  language: string,
  placeholders: ScriptPlaceholder[],
  executorType?: string,
) {
  // Always dispose previous to avoid duplicates, then re-register with latest data
  globalDisposable?.dispose();
  import("./script-editor-completions").then(({ createPlaceholderCompletionProvider }) => {
    globalDisposable?.dispose();
    globalDisposable = monaco.languages.registerCompletionItemProvider(
      language,
      createPlaceholderCompletionProvider(monaco, placeholders, executorType),
    );
  });
  registeredInstanceCount++;
}

function unregisterProvider() {
  registeredInstanceCount--;
  if (registeredInstanceCount <= 0) {
    globalDisposable?.dispose();
    globalDisposable = null;
    registeredInstanceCount = 0;
  }
}

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
  const mountedRef = useRef(false);

  useEffect(() => {
    return () => {
      if (mountedRef.current) {
        unregisterProvider();
        mountedRef.current = false;
      }
    };
  }, []);

  const handleBeforeMount: BeforeMount = useCallback((monaco) => {
    monaco.editor.defineTheme("kandev-dark", KANDEV_MONACO_DARK);
  }, []);

  const handleMount: OnMount = useCallback(
    (_editor, monaco) => {
      if (placeholders && placeholders.length > 0) {
        mountedRef.current = true;
        registerProvider(monaco, language, placeholders, executorType);
      }
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
        wordBasedSuggestions: "off",
        scrollbar: {
          vertical: "auto",
          horizontal: "auto",
          alwaysConsumeMouseWheel: false,
        },
      }}
    />
  );
}
