"use client";

import { useEffect, useRef, useCallback } from "react";
import dynamic from "next/dynamic";
import type { BeforeMount, OnMount } from "@monaco-editor/react";
import { KANDEV_MONACO_DARK } from "@/lib/theme/editor-theme";
import {
  createPlaceholderCompletionProvider,
  type ScriptPlaceholder,
} from "./script-editor-completions";

const MonacoEditor = dynamic(() => import("@monaco-editor/react").then((m) => m.default), {
  ssr: false,
  loading: () => (
    <div className="h-full w-full flex items-center justify-center text-muted-foreground text-xs border rounded-md bg-muted/20">
      Loading editor...
    </div>
  ),
});

type Monaco = typeof import("monaco-editor");

export type CompletionProviderFactory = (
  monaco: Monaco,
) => import("monaco-editor").languages.CompletionItemProvider;

// Per-language singletons so a markdown editor and a shell editor can coexist
// without their providers fighting over the same global slot.
const disposablesByLanguage: Map<string, { dispose: () => void }> = new Map();
const instanceCountsByLanguage: Map<string, number> = new Map();

function disposeFor(language: string) {
  const existing = disposablesByLanguage.get(language);
  if (existing) {
    existing.dispose();
    disposablesByLanguage.delete(language);
  }
}

function swapProvider(monaco: Monaco, language: string, factory: CompletionProviderFactory) {
  disposeFor(language);
  disposablesByLanguage.set(
    language,
    monaco.languages.registerCompletionItemProvider(language, factory(monaco)),
  );
}

function unregisterProvider(language: string) {
  const next = (instanceCountsByLanguage.get(language) ?? 1) - 1;
  if (next <= 0) {
    disposeFor(language);
    instanceCountsByLanguage.delete(language);
    return;
  }
  instanceCountsByLanguage.set(language, next);
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
  /**
   * Optional custom completion provider factory. Takes precedence over
   * `placeholders` — use for non-placeholder languages (e.g. markdown
   * file-path autocomplete).
   */
  completionProvider?: CompletionProviderFactory;
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
  completionProvider,
  readOnly = false,
  lineNumbers = "on",
}: ScriptEditorProps) {
  const mountedRef = useRef(false);
  const monacoRef = useRef<Monaco | null>(null);

  useEffect(() => {
    return () => {
      if (mountedRef.current) {
        unregisterProvider(language);
        mountedRef.current = false;
      }
    };
  }, [language]);

  const ensureProviderRegistered = useCallback(
    (monaco: Monaco) => {
      const factory = resolveFactory(completionProvider, placeholders, executorType);
      if (!factory) return;
      if (!mountedRef.current) {
        mountedRef.current = true;
        instanceCountsByLanguage.set(language, (instanceCountsByLanguage.get(language) ?? 0) + 1);
      }
      swapProvider(monaco, language, factory);
    },
    [completionProvider, placeholders, executorType, language],
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

function resolveFactory(
  custom: CompletionProviderFactory | undefined,
  placeholders: ScriptPlaceholder[] | undefined,
  executorType: string | undefined,
): CompletionProviderFactory | null {
  if (custom) return custom;
  if (placeholders && placeholders.length > 0) {
    return (monaco) => createPlaceholderCompletionProvider(monaco, placeholders, executorType);
  }
  return null;
}
