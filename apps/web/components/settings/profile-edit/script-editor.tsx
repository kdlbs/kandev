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

type Monaco = typeof import("monaco-editor");

// Module-level singleton: only one completion provider registered for "shell" at a time.
let globalDisposable: { dispose: () => void } | null = null;
let registeredInstanceCount = 0;

function swapProvider(
  monaco: Monaco,
  language: string,
  placeholders: ScriptPlaceholder[],
  executorType?: string,
) {
  globalDisposable?.dispose();
  import("./script-editor-completions").then(({ createPlaceholderCompletionProvider }) => {
    globalDisposable?.dispose();
    globalDisposable = monaco.languages.registerCompletionItemProvider(
      language,
      createPlaceholderCompletionProvider(monaco, placeholders, executorType),
    );
  });
}

function registerProvider(
  monaco: Monaco,
  language: string,
  placeholders: ScriptPlaceholder[],
  executorType?: string,
) {
  swapProvider(monaco, language, placeholders, executorType);
  registeredInstanceCount++;
}

// Re-register without incrementing the lifecycle counter (e.g., when placeholders change)
function reRegisterProvider(
  monaco: Monaco,
  language: string,
  placeholders: ScriptPlaceholder[],
  executorType?: string,
) {
  swapProvider(monaco, language, placeholders, executorType);
}

function unregisterProvider() {
  registeredInstanceCount--;
  if (registeredInstanceCount <= 0) {
    globalDisposable?.dispose();
    globalDisposable = null;
    registeredInstanceCount = 0;
  }
}

/** Compute editor height from content lines (min 80px, max 400px). */
export function computeEditorHeight(value: string, minLines = 3): string {
  const lineCount = Math.max((value || "").split("\n").length, minLines);
  const lineHeight = 19;
  const padding = 16;
  const height = Math.min(Math.max(lineCount * lineHeight + padding, 80), 400);
  return `${height}px`;
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
  const monacoRef = useRef<Monaco | null>(null);

  useEffect(() => {
    return () => {
      if (mountedRef.current) {
        unregisterProvider();
        mountedRef.current = false;
      }
    };
  }, []);

  // Register/re-register the provider whenever Monaco is mounted and placeholders
  // change. The singleton `registerProvider` disposes any previous registration,
  // so swapping is idempotent. `mountedRef` guards the lifecycle counter.
  const ensureProviderRegistered = useCallback(
    (monaco: Monaco) => {
      if (!placeholders || placeholders.length === 0) return;
      if (!mountedRef.current) {
        mountedRef.current = true;
        registerProvider(monaco, language, placeholders, executorType);
      } else {
        reRegisterProvider(monaco, language, placeholders, executorType);
      }
    },
    [placeholders, executorType, language],
  );

  useEffect(() => {
    if (monacoRef.current) ensureProviderRegistered(monacoRef.current);
  }, [ensureProviderRegistered]);

  const handleBeforeMount: BeforeMount = useCallback((monaco) => {
    monaco.editor.defineTheme("kandev-dark", KANDEV_MONACO_DARK);
  }, []);

  const handleMount: OnMount = useCallback(
    (_editor, monaco) => {
      monacoRef.current = monaco;
      ensureProviderRegistered(monaco);
    },
    [ensureProviderRegistered],
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
        fixedOverflowWidgets: true,
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
