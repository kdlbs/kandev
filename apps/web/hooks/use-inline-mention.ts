'use client';

import { useState, useCallback, useRef, useMemo, useEffect } from 'react';
import { useCustomPrompts } from '@/hooks/domains/settings/use-custom-prompts';
import { getWebSocketClient } from '@/lib/ws/connection';
import { searchWorkspaceFiles } from '@/lib/ws/workspace-files';
import { getFileName } from '@/lib/utils/file-path';
import type { RichTextInputHandle } from '@/components/task/chat/rich-text-input';

export type MentionItem = {
  id: string;
  kind: 'prompt' | 'file' | 'plan';
  label: string;
  description?: string;
  /** What happens on selection. Each kind provides its own. */
  onSelect: (input: RichTextInputHandle, value: string, triggerStart: number, onChange: (v: string) => void) => void;
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
  return charBefore === ' ' || charBefore === '\n' || charBefore === '\t';
}

function filterItems(items: MentionItem[], query: string): MentionItem[] {
  if (!query) return items;
  const lowerQuery = query.toLowerCase();

  return items
    .map((item) => {
      const label = item.label.toLowerCase();
      let score = 0;

      // Exact prefix match (highest)
      if (label.startsWith(lowerQuery)) {
        score = 100;
      }
      // Word boundary match
      else if (label.split(/[\s\-_/]/).some((word) => word.startsWith(lowerQuery))) {
        score = 50;
      }
      // Contains match
      else if (label.includes(lowerQuery)) {
        score = 25;
      }

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
    kind: 'file',
    label: filePath,
    description: 'File',
    onSelect: (input, value, triggerStart, onChange) => {
      // Remove the @trigger text
      const cursorPos = input.getSelectionStart();
      onChange(value.substring(0, triggerStart) + value.substring(cursorPos));

      // Add to context store
      const name = getFileName(filePath);
      onFileSelect?.(filePath, name);

      requestAnimationFrame(() => {
        input.setSelectionRange(triggerStart, triggerStart);
        input.focus();
      });
    },
  };
}

function makePlanItem(onPlanSelect: () => void): MentionItem {
  return {
    id: '__plan__',
    kind: 'plan',
    label: 'Plan',
    description: 'Include the plan as context',
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
    kind: 'prompt',
    label: prompt.name,
    description: prompt.content.length > 100 ? prompt.content.slice(0, 100) + '...' : prompt.content,
    onSelect: (input, value, triggerStart, onChange) => {
      // Remove the @trigger text
      const cursorPos = input.getSelectionStart();
      onChange(value.substring(0, triggerStart) + value.substring(cursorPos));

      // Add to context store
      onPromptSelect?.(prompt.id, prompt.name);

      requestAnimationFrame(() => {
        input.setSelectionRange(triggerStart, triggerStart);
        input.focus();
      });
    },
  };
}

export function useInlineMention(
  inputRef: React.RefObject<RichTextInputHandle | null>,
  value: string,
  onChange: (value: string) => void,
  sessionId?: string | null,
  onPlanSelect?: () => void,
  onFileSelect?: (path: string, name: string) => void,
  onPromptSelect?: (id: string, name: string) => void,
) {
  const [isOpen, setIsOpen] = useState(false);
  const [position, setPosition] = useState<Position | null>(null);
  const [triggerStart, setTriggerStart] = useState<number>(-1);
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [fileResults, setFileResults] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(false);

  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastSearchRef = useRef<{ query: string; results: string[] }>({
    query: '',
    results: [],
  });

  const { prompts } = useCustomPrompts();

  // File search function with caching
  const searchFiles = useCallback(
    async (searchQuery: string): Promise<string[]> => {
      if (!sessionId) return [];

      // Use special key for empty query
      const cacheKey = searchQuery || '__empty__';

      // Return cached results if query matches
      if (lastSearchRef.current.query === cacheKey) {
        return lastSearchRef.current.results;
      }

      try {
        const client = getWebSocketClient();
        if (!client) return [];

        // For empty query, search with wildcard or empty to get initial files
        const response = await searchWorkspaceFiles(client, sessionId, searchQuery || '', 20);
        const results = response.files || [];

        // Cache results
        lastSearchRef.current = { query: cacheKey, results };
        return results;
      } catch (error) {
        console.error('File search failed:', error);
        return [];
      }
    },
    [sessionId]
  );

  // Debounced file search - search immediately when menu opens, debounce subsequent queries
  /* eslint-disable react-hooks/set-state-in-effect -- loading state sync is intentional for UX */
  useEffect(() => {
    if (!isOpen) {
      setIsLoading(false);
      return;
    }

    // Cancel previous timeout
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }

    // Search immediately for empty query (menu just opened), debounce for typed queries
    const delay = query === '' ? 0 : FILE_SEARCH_DEBOUNCE;

    // Mark as loading and start search
    setIsLoading(true);
    searchTimeoutRef.current = setTimeout(async () => {
      const results = await searchFiles(query);
      setFileResults(results);
      setIsLoading(false);
    }, delay);

    return () => {
      if (searchTimeoutRef.current) {
        clearTimeout(searchTimeoutRef.current);
      }
    };
  }, [isOpen, query, searchFiles]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Build plan item
  const planItem = useMemo((): MentionItem | null => {
    if (!onPlanSelect) return null;
    return makePlanItem(onPlanSelect);
  }, [onPlanSelect]);

  // Build prompt items
  const promptItems = useMemo((): MentionItem[] => {
    return prompts.map((prompt) => makePromptItem(prompt, onPromptSelect));
  }, [prompts, onPromptSelect]);

  // Build file items from search results
  const fileItems = useMemo((): MentionItem[] => {
    return fileResults.map((filePath) => makeFileItem(filePath, onFileSelect));
  }, [fileResults, onFileSelect]);

  // Combine and filter items
  const filteredItems = useMemo(() => {
    // Plan first, then prompts, then files
    const allItems: MentionItem[] = [];
    if (planItem) allItems.push(planItem);
    allItems.push(...promptItems);
    allItems.push(...fileItems);
    return filterItems(allItems, query);
  }, [planItem, promptItems, fileItems, query]);

  // Reset selected index when items change
  /* eslint-disable react-hooks/set-state-in-effect -- resetting selection on items change is intentional */
  useEffect(() => {
    setSelectedIndex(0);
  }, [filteredItems.length]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Auto-close menu when no results and query is long enough
  // This prevents showing "No results found" when user is just typing normally
  /* eslint-disable react-hooks/set-state-in-effect -- auto-close logic is intentional */
  useEffect(() => {
    if (!isOpen || isLoading) return;

    if (filteredItems.length === 0 && query.length >= NO_RESULTS_CLOSE_THRESHOLD) {
      setIsOpen(false);
      setTriggerStart(-1);
      setQuery('');
    }
  }, [isOpen, isLoading, filteredItems.length, query.length]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Handle text changes to detect @ trigger
  const handleChange = useCallback(
    (newValue: string) => {
      onChange(newValue);

      const input = inputRef.current;
      if (!input) return;

      // Use requestAnimationFrame to ensure DOM is updated
      requestAnimationFrame(() => {
        const cursorPos = input.getSelectionStart();
        const textBeforeCursor = newValue.substring(0, cursorPos);

        // Find the last @ before cursor
        const lastAtIndex = textBeforeCursor.lastIndexOf('@');

        if (lastAtIndex >= 0 && isValidMentionTrigger(newValue, lastAtIndex)) {
          // Check if we're still within the mention (no space after @)
          const textAfterAt = textBeforeCursor.substring(lastAtIndex + 1);
          if (!/\s/.test(textAfterAt)) {
            // Open menu
            const caretRect = input.getCaretRect();
            if (caretRect) {
              setPosition({ x: caretRect.x, y: caretRect.y });
              setTriggerStart(lastAtIndex);
              setQuery(textAfterAt);
              setIsOpen(true);
              return;
            }
          }
        }

        // Close menu if no valid trigger
        if (isOpen) {
          setIsOpen(false);
          setTriggerStart(-1);
          setQuery('');
        }
      });
    },
    [inputRef, isOpen, onChange]
  );

  // Handle item selection
  const handleSelect = useCallback(
    (item: MentionItem) => {
      const input = inputRef.current;
      if (!input || triggerStart < 0) return;

      item.onSelect(input, value, triggerStart, onChange);
      setIsOpen(false);
      setTriggerStart(-1);
      setQuery('');
    },
    [inputRef, triggerStart, value, onChange]
  );

  // Handle keyboard navigation
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (!isOpen) return;

      switch (event.key) {
        case 'ArrowDown':
          event.preventDefault();
          setSelectedIndex((prev) => Math.min(prev + 1, filteredItems.length - 1));
          break;
        case 'ArrowUp':
          event.preventDefault();
          setSelectedIndex((prev) => Math.max(prev - 1, 0));
          break;
        case 'Enter':
        case 'Tab':
          if (filteredItems.length > 0) {
            event.preventDefault();
            handleSelect(filteredItems[selectedIndex]);
          }
          break;
        case 'Escape':
          event.preventDefault();
          setIsOpen(false);
          setTriggerStart(-1);
          setQuery('');
          break;
      }
    },
    [isOpen, filteredItems, selectedIndex, handleSelect]
  );

  // Close menu
  const closeMenu = useCallback(() => {
    setIsOpen(false);
    setTriggerStart(-1);
    setQuery('');
  }, []);

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
