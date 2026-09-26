"use client";

import { useEffect, useRef, useCallback } from "react";
import dynamic from "@/lib/routing/client-dynamic";
import type { BeforeMount, OnMount } from "@monaco-editor/react";
import { EDITOR_FONT_FAMILY, EDITOR_FONT_SIZE, KANDEV_MONACO_DARK } from "@/lib/theme/editor-theme";
import type { PromptReference } from "@/lib/prompts/expand-prompt-references";
import {
  createPlaceholderCompletionProvider,
  createPromptMentionCompletionProvider,
  scopeCompletionProviderToModel,
  type ScriptPlaceholder,
} from "./script-editor-completions";
import { useTranslation } from "react-i18next";

// A component rather than an inline literal so the label resolves at render:
// `dynamic()` is evaluated at module load, where a `t()` would freeze the copy
// at the boot locale and never follow a locale switch.
function EditorLoading() {
  const { t } = useTranslation();
  return (
    <div className="h-full w-full flex items-center justify-center text-muted-foreground text-xs border rounded-md bg-muted/20">
      {t("executors:loadingEditor")}
    </div>
  );
}

const MonacoEditor = dynamic(() => import("@monaco-editor/react").then((m) => m.default), {
  ssr: false,
  loading: () => <EditorLoading />,
});

type Monaco = typeof import("monaco-editor");
type MonacoModel = import("monaco-editor").editor.ITextModel;

export type CompletionProviderFactory = (
  monaco: Monaco,
) => import("monaco-editor").languages.CompletionItemProvider;

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
  /**
   * Saved custom prompts to suggest as `@name` mentions. Registered
   * alongside the placeholder/custom completion provider (independent
   * trigger character, so both can be active at once).
   * Callers should preserve the array identity when its contents are unchanged
   * to avoid unnecessary provider re-registration.
   */
  mentionPrompts?: PromptReference[];
  readOnly?: boolean;
  lineNumbers?: "on" | "off";
  ariaLabel?: string;
  testId?: string;
};

type CompletionProviderOptions = Pick<
  ScriptEditorProps,
  "language" | "completionProvider" | "placeholders" | "executorType" | "mentionPrompts"
> & { language: string };

function useCompletionProviders({
  language,
  completionProvider,
  placeholders,
  executorType,
  mentionPrompts,
}: CompletionProviderOptions): OnMount {
  const monacoRef = useRef<Monaco | null>(null);
  const modelRef = useRef<MonacoModel | null>(null);
  const primaryDisposableRef = useRef<{ dispose: () => void } | null>(null);
  const mentionDisposableRef = useRef<{ dispose: () => void } | null>(null);
  const primaryRegistrationRef = useRef<() => void>(() => undefined);
  const mentionRegistrationRef = useRef<() => void>(() => undefined);

  const disposePrimaryProvider = useCallback(() => {
    primaryDisposableRef.current?.dispose();
    primaryDisposableRef.current = null;
  }, []);

  const disposeMentionProvider = useCallback(() => {
    mentionDisposableRef.current?.dispose();
    mentionDisposableRef.current = null;
  }, []);

  const registerPrimaryProvider = useCallback(() => {
    disposePrimaryProvider();
    const monaco = monacoRef.current;
    const model = modelRef.current;
    const factory = resolveFactory(completionProvider, placeholders, executorType);
    if (!monaco || !model || !factory) return;
    primaryDisposableRef.current = monaco.languages.registerCompletionItemProvider(
      language,
      scopeCompletionProviderToModel(factory(monaco), model),
    );
  }, [completionProvider, disposePrimaryProvider, executorType, language, placeholders]);

  const registerMentionProvider = useCallback(() => {
    disposeMentionProvider();
    const monaco = monacoRef.current;
    const model = modelRef.current;
    const factory = resolveMentionFactory(mentionPrompts);
    if (!monaco || !model || !factory) return;
    mentionDisposableRef.current = monaco.languages.registerCompletionItemProvider(
      language,
      scopeCompletionProviderToModel(factory(monaco), model),
    );
  }, [disposeMentionProvider, language, mentionPrompts]);

  // Monaco calls onMount only once. Keep its callback pointed at the latest
  // provider registrations so data that loads before Monaco finishes mounting
  // is available on the first registration.
  primaryRegistrationRef.current = registerPrimaryProvider;
  mentionRegistrationRef.current = registerMentionProvider;

  useEffect(() => {
    registerPrimaryProvider();
    return disposePrimaryProvider;
  }, [disposePrimaryProvider, registerPrimaryProvider]);

  useEffect(() => {
    registerMentionProvider();
    return disposeMentionProvider;
  }, [disposeMentionProvider, registerMentionProvider]);

  const handleMount: OnMount = useCallback((editor, monaco) => {
    monacoRef.current = monaco;
    modelRef.current = editor.getModel();
    primaryRegistrationRef.current();
    mentionRegistrationRef.current();
  }, []);

  return handleMount;
}

export function ScriptEditor({
  value,
  onChange,
  language = "shell",
  height = "300px",
  placeholders,
  executorType,
  completionProvider,
  mentionPrompts,
  readOnly = false,
  lineNumbers = "on",
  ariaLabel,
  testId,
}: ScriptEditorProps) {
  const handleMount = useCompletionProviders({
    language,
    completionProvider,
    placeholders,
    executorType,
    mentionPrompts,
  });
  const handleBeforeMount: BeforeMount = useCallback((monaco) => {
    monaco.editor.defineTheme("kandev-dark", KANDEV_MONACO_DARK);
  }, []);

  return (
    <div data-testid={testId} className="h-full w-full">
      <MonacoEditor
        height={height}
        language={language}
        value={value}
        onChange={(v) => onChange(v ?? "")}
        beforeMount={handleBeforeMount}
        onMount={handleMount}
        theme="kandev-dark"
        options={{
          ariaLabel,
          minimap: { enabled: false },
          lineNumbers,
          wordWrap: "on",
          fontFamily: EDITOR_FONT_FAMILY,
          fontSize: EDITOR_FONT_SIZE,
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
    </div>
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

function resolveMentionFactory(
  mentionPrompts: PromptReference[] | undefined,
): CompletionProviderFactory | null {
  if (!mentionPrompts || mentionPrompts.length === 0) return null;
  return (monaco) => createPromptMentionCompletionProvider(monaco, mentionPrompts);
}
