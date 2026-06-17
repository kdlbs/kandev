"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import {
  buildPassthroughCommands,
  detectPassthroughSuggestion,
  fileReferenceToken,
  filterPassthroughCommands,
  replacePassthroughRange,
  type PassthroughCommand,
  type PassthroughSuggestionState,
} from "./passthrough-composer-helpers";

const COMPOSER_MAX_HEIGHT_PX = 128;
const FILE_SEARCH_LIMIT = 12;

export type AgentCommand = { name: string; description?: string };
export type SuggestionItem = PassthroughCommand | string;

export function droppedReferenceTokens(e: React.DragEvent): string[] {
  const text = e.dataTransfer.getData("text/plain") || e.dataTransfer.getData("text/uri-list");
  const textRefs = text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  if (textRefs.length > 0) return textRefs;
  return Array.from(e.dataTransfer.files)
    .map((file) => file.webkitRelativePath || file.name)
    .filter(Boolean);
}

export function suggestionLabel(item: SuggestionItem): string {
  return typeof item === "string" ? item : item.label;
}

export function suggestionDescription(item: SuggestionItem): string | undefined {
  return typeof item === "string" ? "Workspace file" : item.description;
}

function useComposerSizing(
  textareaRef: React.RefObject<HTMLTextAreaElement | null>,
  value: string,
  autoFocus: boolean,
) {
  useLayoutEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, COMPOSER_MAX_HEIGHT_PX)}px`;
  }, [textareaRef, value]);

  useEffect(() => {
    if (autoFocus) textareaRef.current?.focus();
  }, [autoFocus, textareaRef]);
}

function useComposerFocusShortcut(
  focusShortcut: KeyboardShortcut | undefined,
  textareaRef: React.RefObject<HTMLTextAreaElement | null>,
) {
  useKeyboardShortcut(
    focusShortcut ?? { key: "" as KeyboardShortcut["key"] },
    useCallback(
      (event: globalThis.KeyboardEvent) => {
        const el = document.activeElement;
        const isTyping =
          el instanceof HTMLInputElement ||
          el instanceof HTMLTextAreaElement ||
          (el instanceof HTMLElement && el.isContentEditable);
        if (isTyping && el !== textareaRef.current) return;
        event.preventDefault();
        textareaRef.current?.focus();
      },
      [textareaRef],
    ),
    { enabled: !!focusShortcut },
  );
}

function useFileResults(
  sessionId: string | null | undefined,
  suggestion: PassthroughSuggestionState,
) {
  const [fileResults, setFileResults] = useState<string[]>([]);
  useEffect(() => {
    if (suggestion?.kind !== "file" || !sessionId) return;
    let cancelled = false;
    const timer = window.setTimeout(
      async () => {
        const client = getWebSocketClient();
        if (!client) {
          if (!cancelled) setFileResults([]);
          return;
        }
        try {
          const response = await searchWorkspaceFiles(
            client,
            sessionId,
            suggestion.query,
            FILE_SEARCH_LIMIT,
          );
          if (!cancelled) setFileResults(response.files ?? []);
        } catch {
          if (!cancelled) setFileResults([]);
        }
      },
      suggestion.query ? 200 : 0,
    );
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [sessionId, suggestion]);
  return suggestion?.kind === "file" && sessionId ? fileResults : [];
}

function useSuggestionItems(
  availableCommands: AgentCommand[] | undefined,
  suggestion: PassthroughSuggestionState,
  fileResults: string[],
) {
  const commands = useMemo(() => buildPassthroughCommands(availableCommands), [availableCommands]);
  const commandQuery = suggestion?.kind === "command" ? suggestion.query : "";
  const filteredCommands = useMemo(
    () => filterPassthroughCommands(commands, commandQuery),
    [commands, commandQuery],
  );
  return suggestion?.kind === "command" ? filteredCommands : fileResults;
}

export function usePassthroughComposerController({
  onSubmit,
  autoFocus,
  sessionId,
  availableCommands,
  focusShortcut,
}: {
  onSubmit: (content: string) => Promise<void>;
  autoFocus: boolean;
  sessionId?: string | null;
  availableCommands?: AgentCommand[];
  focusShortcut?: KeyboardShortcut;
}) {
  const [value, setValue] = useState("");
  const [isSending, setIsSending] = useState(false);
  const [suggestion, setSuggestion] = useState<PassthroughSuggestionState>(null);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const fileResults = useFileResults(sessionId, suggestion);
  const suggestionItems = useSuggestionItems(availableCommands, suggestion, fileResults);
  const trimmed = value.trim();
  const canSubmit = trimmed.length > 0 && !isSending;
  const showSuggestions = !!suggestion && suggestionItems.length > 0;

  useComposerSizing(textareaRef, value, autoFocus);
  useComposerFocusShortcut(focusShortcut, textareaRef);
  useEffect(() => {
    setSelectedIndex(0);
  }, [suggestion?.kind, suggestion?.query, suggestionItems.length]);

  const updateValue = useCallback((next: string) => {
    setValue(next);
    const el = textareaRef.current;
    const cursor = el?.selectionStart ?? next.length;
    setSuggestion(detectPassthroughSuggestion(next, cursor));
  }, []);

  const submit = useCallback(async () => {
    if (!canSubmit) return;
    setIsSending(true);
    try {
      await onSubmit(trimmed);
      setValue("");
      setSuggestion(null);
    } catch {
      // Preserve typed text so the user can retry after a transient WS/API failure.
    } finally {
      setIsSending(false);
    }
  }, [canSubmit, onSubmit, trimmed]);

  const insertSelection = useCallback(
    (item: SuggestionItem) => {
      if (!suggestion) return;
      const textarea = textareaRef.current;
      const cursor = textarea?.selectionStart ?? value.length;
      const label = suggestionLabel(item);
      const insertion = suggestion.kind === "command" ? `${label} ` : fileReferenceToken(label);
      const next = replacePassthroughRange(value, suggestion.triggerStart, cursor, insertion);
      setValue(next.value);
      setSuggestion(null);
      requestAnimationFrame(() => {
        textarea?.focus();
        textarea?.setSelectionRange(next.caret, next.caret);
      });
    },
    [suggestion, value],
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      const refs = droppedReferenceTokens(e);
      if (refs.length === 0) return;
      e.preventDefault();
      const textarea = textareaRef.current;
      const cursor = textarea?.selectionStart ?? value.length;
      const insertion = refs.map(fileReferenceToken).join("\n");
      const next = replacePassthroughRange(value, cursor, cursor, insertion);
      setValue(next.value);
      setSuggestion(null);
      requestAnimationFrame(() => {
        textarea?.focus();
        textarea?.setSelectionRange(next.caret, next.caret);
      });
    },
    [value],
  );

  return {
    value,
    isSending,
    canSubmit,
    textareaRef,
    suggestion,
    suggestionItems,
    showSuggestions,
    selectedIndex,
    setSelectedIndex,
    updateValue,
    submit,
    insertSelection,
    handleDrop,
    setSuggestion,
  };
}

export function handleSuggestionKey(
  e: KeyboardEvent<HTMLTextAreaElement>,
  args: {
    showSuggestions: boolean;
    suggestionItems: SuggestionItem[];
    selectedIndex: number;
    setSelectedIndex: (v: number | ((i: number) => number)) => void;
    insertSelection: (item: SuggestionItem) => void;
  },
): boolean {
  if (!args.showSuggestions) return false;
  if (e.key === "ArrowDown") {
    e.preventDefault();
    args.setSelectedIndex((i) => Math.min(i + 1, args.suggestionItems.length - 1));
    return true;
  }
  if (e.key === "ArrowUp") {
    e.preventDefault();
    args.setSelectedIndex((i) => Math.max(i - 1, 0));
    return true;
  }
  if (e.key === "Tab" || e.key === "Enter") {
    e.preventDefault();
    const selectedIndex = Math.min(args.selectedIndex, args.suggestionItems.length - 1);
    const selectedItem = args.suggestionItems[selectedIndex];
    if (selectedItem) args.insertSelection(selectedItem);
    return true;
  }
  return false;
}
