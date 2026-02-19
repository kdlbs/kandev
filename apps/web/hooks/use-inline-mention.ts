"use client";

import { useState, useCallback, useRef, useMemo, useEffect } from "react";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import { getFileName } from "@/lib/utils/file-path";
import type { RichTextInputHandle } from "@/components/task/chat/rich-text-input";

export type MentionItem = {
  id: string;
  kind: "prompt" | "file" | "plan";
  label: string;
  description?: string;
  /** What happens on selection. Each kind provides its own. */
  onSelect: (
    input: RichTextInputHandle,
    value: string,
    triggerStart: number,
    onChange: (v: string) => void,
  ) => void;
};

type Position = {
  x: number;
  y: number;
};

// Debounce delay for file search (ms)
const FILE_SEARCH_DEBOUNCE = 300;

// Close menu if no results after this many characters
const NO_RESULTS_CLOSE_THRESHOLD = 3;

function isValidMentionTrigger(text: string, pos: number): boolean {
  if (pos === 0) return true;
  const charBefore = text[pos - 1];
  return charBefore === " " || charBefore === "\n" || charBefore === "\t";
}

function filterItems(items: MentionItem[], query: string): MentionItem[] {
  if (!query) return items;
  const lowerQuery = query.toLowerCase();

  return items
    .map((item) => {
      const label = item.label.toLowerCase();
      let score = 0;
      if (label.startsWith(lowerQuery)) score = 100;
      else if (label.split(/[\s\-_/]/).some((word) => word.startsWith(lowerQuery))) score = 50;
      else if (label.includes(lowerQuery)) score = 25;
      return { item, score };
    })
    .filter(({ score }) => score > 0)
    .sort((a, b) => b.score - a.score)
    .map(({ item }) => item);
}

/** Build a file mention item that adds file to context store instead of inserting text. */
function makeFileItem(
  filePath: string,
  onFileSelect?: (path: string, name: string) => void,
): MentionItem {
  return {
    id: filePath,
    kind: "file",
    label: filePath,
    description: "File",
    onSelect: (input, value, triggerStart, onChange) => {
      const cursorPos = input.getSelectionStart();
      onChange(value.substring(0, triggerStart) + value.substring(cursorPos));
      onFileSelect?.(filePath, getFileName(filePath));
      requestAnimationFrame(() => {
        input.setSelectionRange(triggerStart, triggerStart);
        input.focus();
      });
    },
  };
}

function makePlanItem(onPlanSelect: () => void): MentionItem {
  return {
    id: "__plan__",
    kind: "plan",
    label: "Plan",
    description: "Include the plan as context",
    onSelect: (input, value, triggerStart, onChange) => {
      const cursorPos = input.getSelectionStart();
      onChange(value.substring(0, triggerStart) + value.substring(cursorPos));
      onPlanSelect();
      requestAnimationFrame(() => {
        input.setSelectionRange(triggerStart, triggerStart);
        input.focus();
      });
    },
  };
}

/** Build a prompt mention item that adds prompt to context store instead of inserting content. */
function makePromptItem(
  prompt: { id: string; name: string; content: string },
  onPromptSelect?: (id: string, name: string) => void,
): MentionItem {
  return {
    id: prompt.id,
    kind: "prompt",
    label: prompt.name,
    description:
      prompt.content.length > 100 ? prompt.content.slice(0, 100) + "..." : prompt.content,
    onSelect: (input, value, triggerStart, onChange) => {
      const cursorPos = input.getSelectionStart();
      onChange(value.substring(0, triggerStart) + value.substring(cursorPos));
      onPromptSelect?.(prompt.id, prompt.name);
      requestAnimationFrame(() => {
        input.setSelectionRange(triggerStart, triggerStart);
        input.focus();
      });
    },
  };
}

function clearMentionState(
  setIsOpen: (open: boolean) => void,
  setTriggerStart: (start: number) => void,
  setQuery: (query: string) => void,
) {
  setIsOpen(false);
  setTriggerStart(-1);
  setQuery("");
}

/** Detect an @-trigger in text before cursor and return the query, or null if none. */
function detectMentionTrigger(
  text: string,
  cursorPos: number,
): { triggerStart: number; query: string } | null {
  const textBeforeCursor = text.substring(0, cursorPos);
  const lastAtIndex = textBeforeCursor.lastIndexOf("@");
  if (lastAtIndex < 0 || !isValidMentionTrigger(text, lastAtIndex)) return null;
  const textAfterAt = textBeforeCursor.substring(lastAtIndex + 1);
  if (/\s/.test(textAfterAt)) return null;
  return { triggerStart: lastAtIndex, query: textAfterAt };
}

export interface UseInlineMentionParams {
  inputRef: React.RefObject<RichTextInputHandle | null>;
  value: string;
  onChange: (value: string) => void;
  sessionId?: string | null;
  onPlanSelect?: () => void;
  onFileSelect?: (path: string, name: string) => void;
  onPromptSelect?: (id: string, name: string) => void;
}

function useFileSearch(sessionId: string | null | undefined, isOpen: boolean, query: string) {
  const [fileResults, setFileResults] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastSearchRef = useRef<{ query: string; results: string[] }>({ query: "", results: [] });

  const searchFiles = useCallback(
    async (searchQuery: string): Promise<string[]> => {
      if (!sessionId) return [];
      const cacheKey = searchQuery || "__empty__";
      if (lastSearchRef.current.query === cacheKey) return lastSearchRef.current.results;
      try {
        const client = getWebSocketClient();
        if (!client) return [];
        const response = await searchWorkspaceFiles(client, sessionId, searchQuery || "", 20);
        const results = response.files || [];
        lastSearchRef.current = { query: cacheKey, results };
        return results;
      } catch (error) {
        console.error("File search failed:", error);
        return [];
      }
    },
    [sessionId],
  );

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (!isOpen) {
      setIsLoading(false);
      return;
    }
    if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    const delay = query === "" ? 0 : FILE_SEARCH_DEBOUNCE;
    setIsLoading(true);
    searchTimeoutRef.current = setTimeout(async () => {
      const results = await searchFiles(query);
      setFileResults(results);
      setIsLoading(false);
    }, delay);
    return () => {
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    };
  }, [isOpen, query, searchFiles]);
  /* eslint-enable react-hooks/set-state-in-effect */

  return { fileResults, isLoading };
}

type MentionKeyboardParams = {
  isOpen: boolean;
  filteredItems: MentionItem[];
  selectedIndex: number;
  setSelectedIndex: (v: number | ((prev: number) => number)) => void;
  handleSelect: (item: MentionItem) => void;
  closeMenu: () => void;
};

function useMentionKeyboard({
  isOpen,
  filteredItems,
  selectedIndex,
  setSelectedIndex,
  handleSelect,
  closeMenu,
}: MentionKeyboardParams) {
  return useCallback(
    (event: React.KeyboardEvent) => {
      if (!isOpen) return;
      switch (event.key) {
        case "ArrowDown":
          event.preventDefault();
          setSelectedIndex((prev) => Math.min(prev + 1, filteredItems.length - 1));
          break;
        case "ArrowUp":
          event.preventDefault();
          setSelectedIndex((prev) => Math.max(prev - 1, 0));
          break;
        case "Enter":
        case "Tab":
          if (filteredItems.length > 0) {
            event.preventDefault();
            handleSelect(filteredItems[selectedIndex]);
          }
          break;
        case "Escape":
          event.preventDefault();
          closeMenu();
          break;
      }
    },
    [isOpen, filteredItems, selectedIndex, setSelectedIndex, handleSelect, closeMenu],
  );
}

type MentionItemsOptions = {
  query: string;
  fileResults: string[];
  prompts: Array<{ id: string; name: string; content: string }>;
  onPlanSelect?: () => void;
  onFileSelect?: (path: string, name: string) => void;
  onPromptSelect?: (id: string, name: string) => void;
};

function useMentionItems({ query, fileResults, prompts, onPlanSelect, onFileSelect, onPromptSelect }: MentionItemsOptions) {
  const planItem = useMemo((): MentionItem | null => {
    if (!onPlanSelect) return null;
    return makePlanItem(onPlanSelect);
  }, [onPlanSelect]);

  const promptItems = useMemo(
    () => prompts.map((prompt) => makePromptItem(prompt, onPromptSelect)),
    [prompts, onPromptSelect],
  );

  const fileItems = useMemo(
    () => fileResults.map((filePath) => makeFileItem(filePath, onFileSelect)),
    [fileResults, onFileSelect],
  );

  return useMemo(() => {
    const allItems: MentionItem[] = [];
    if (planItem) allItems.push(planItem);
    allItems.push(...promptItems, ...fileItems);
    return filterItems(allItems, query);
  }, [planItem, promptItems, fileItems, query]);
}

export function useInlineMention({
  inputRef,
  value,
  onChange,
  sessionId,
  onPlanSelect,
  onFileSelect,
  onPromptSelect,
}: UseInlineMentionParams) {
  const [isOpen, setIsOpen] = useState(false);
  const [position, setPosition] = useState<Position | null>(null);
  const [triggerStart, setTriggerStart] = useState<number>(-1);
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);

  const { prompts } = useCustomPrompts();
  const { fileResults, isLoading } = useFileSearch(sessionId, isOpen, query);
  const filteredItems = useMentionItems({
    query,
    fileResults,
    prompts,
    onPlanSelect,
    onFileSelect,
    onPromptSelect,
  });

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    setSelectedIndex(0);
  }, [filteredItems.length]);

  useEffect(() => {
    if (!isOpen || isLoading) return;
    if (filteredItems.length === 0 && query.length >= NO_RESULTS_CLOSE_THRESHOLD) {
      clearMentionState(setIsOpen, setTriggerStart, setQuery);
    }
  }, [isOpen, isLoading, filteredItems.length, query.length]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const handleChange = useCallback(
    (newValue: string) => {
      onChange(newValue);
      const input = inputRef.current;
      if (!input) return;
      requestAnimationFrame(() => {
        const cursorPos = input.getSelectionStart();
        const trigger = detectMentionTrigger(newValue, cursorPos);
        if (trigger) {
          const caretRect = input.getCaretRect();
          if (caretRect) {
            setPosition({ x: caretRect.x, y: caretRect.y });
            setTriggerStart(trigger.triggerStart);
            setQuery(trigger.query);
            setIsOpen(true);
            return;
          }
        }
        if (isOpen) {
          clearMentionState(setIsOpen, setTriggerStart, setQuery);
        }
      });
    },
    [inputRef, isOpen, onChange],
  );

  const closeMenu = useCallback(() => {
    clearMentionState(setIsOpen, setTriggerStart, setQuery);
  }, []);

  const handleSelect = useCallback(
    (item: MentionItem) => {
      const input = inputRef.current;
      if (!input || triggerStart < 0) return;
      item.onSelect(input, value, triggerStart, onChange);
      closeMenu();
    },
    [inputRef, triggerStart, value, onChange, closeMenu],
  );

  const handleKeyDown = useMentionKeyboard({
    isOpen,
    filteredItems,
    selectedIndex,
    setSelectedIndex,
    handleSelect,
    closeMenu,
  });

  return {
    isOpen,
    isLoading,
    position,
    query,
    items: filteredItems,
    selectedIndex,
    setSelectedIndex,
    handleChange,
    handleSelect,
    handleKeyDown,
    closeMenu,
  };
}
