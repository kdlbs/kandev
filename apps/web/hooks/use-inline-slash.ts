'use client';

import { useState, useCallback, useMemo, useEffect } from 'react';
import type { RichTextInputHandle } from '@/components/task/chat/rich-text-input';

export type SlashCommand = {
  id: string;
  label: string;
  description: string;
  action: 'plan' | 'todo' | 'review' | 'summarize';
};

type Position = {
  x: number;
  y: number;
};

const SLASH_COMMANDS: SlashCommand[] = [
  {
    id: 'plan',
    label: '/plan',
    description: 'Start a planning response',
    action: 'plan',
  },
  {
    id: 'todo',
    label: '/todo',
    description: 'Add a todo list',
    action: 'todo',
  },
  {
    id: 'review',
    label: '/review',
    description: 'Request a review',
    action: 'review',
  },
  {
    id: 'summarize',
    label: '/summarize',
    description: 'Summarize the task',
    action: 'summarize',
  },
];

function isValidSlashTrigger(text: string, pos: number): boolean {
  if (pos === 0) return true;
  const charBefore = text[pos - 1];
  return charBefore === ' ' || charBefore === '\n' || charBefore === '\t';
}

function filterCommands(query: string): SlashCommand[] {
  if (!query) return SLASH_COMMANDS;
  const lowerQuery = query.toLowerCase();

  return SLASH_COMMANDS.filter((cmd) => {
    const label = cmd.label.toLowerCase();
    return label.startsWith('/' + lowerQuery) || cmd.action.startsWith(lowerQuery);
  }).sort((a, b) => {
    // Prefer exact prefix matches
    const aStartsWithQuery = a.action.toLowerCase().startsWith(lowerQuery);
    const bStartsWithQuery = b.action.toLowerCase().startsWith(lowerQuery);
    if (aStartsWithQuery && !bStartsWithQuery) return -1;
    if (!aStartsWithQuery && bStartsWithQuery) return 1;
    return 0;
  });
}

export function useInlineSlash(
  inputRef: React.RefObject<RichTextInputHandle | null>,
  value: string,
  onChange: (value: string) => void,
  onPlanModeChange?: (enabled: boolean) => void
) {
  const [isOpen, setIsOpen] = useState(false);
  const [position, setPosition] = useState<Position | null>(null);
  const [triggerStart, setTriggerStart] = useState<number>(-1);
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);

  // Filter commands based on query
  const filteredCommands = useMemo(() => filterCommands(query), [query]);

  // Reset selected index when commands change
  /* eslint-disable react-hooks/set-state-in-effect -- resetting selection on items change is intentional */
  useEffect(() => {
    setSelectedIndex(0);
  }, [filteredCommands.length]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Handle text changes to detect / trigger
  const handleChange = useCallback(
    (newValue: string) => {
      onChange(newValue);

      const input = inputRef.current;
      if (!input) return;

      // Use requestAnimationFrame to ensure DOM is updated
      requestAnimationFrame(() => {
        const cursorPos = input.getSelectionStart();
        const textBeforeCursor = newValue.substring(0, cursorPos);

        // Find the last / before cursor
        const lastSlashIndex = textBeforeCursor.lastIndexOf('/');

        if (lastSlashIndex >= 0 && isValidSlashTrigger(newValue, lastSlashIndex)) {
          // Check if we're still within the command (no space after /)
          const textAfterSlash = textBeforeCursor.substring(lastSlashIndex + 1);
          if (!/\s/.test(textAfterSlash) && /^[\w-]*$/.test(textAfterSlash)) {
            // Open menu
            const caretRect = input.getCaretRect();
            if (caretRect) {
              setPosition({ x: caretRect.x, y: caretRect.y });
              setTriggerStart(lastSlashIndex);
              setQuery(textAfterSlash);
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

  // Handle command selection
  const handleSelect = useCallback(
    (command: SlashCommand) => {
      const input = inputRef.current;
      if (!input || triggerStart < 0) return;

      // Get current cursor position
      const cursorPos = input.getSelectionStart();

      // Remove the /command text
      const newValue = value.substring(0, triggerStart) + value.substring(cursorPos);
      onChange(newValue);

      // Execute the command action
      switch (command.action) {
        case 'plan':
          onPlanModeChange?.(true);
          break;
        case 'todo':
          // Insert todo template
          const todoText = '## TODO\n- [ ] ';
          const todoValue = value.substring(0, triggerStart) + todoText + value.substring(cursorPos);
          onChange(todoValue);
          requestAnimationFrame(() => {
            const newPos = triggerStart + todoText.length;
            input.setSelectionRange(newPos, newPos);
            input.focus();
          });
          break;
        case 'review':
          // Insert review request
          const reviewText = 'Please review ';
          const reviewValue = value.substring(0, triggerStart) + reviewText + value.substring(cursorPos);
          onChange(reviewValue);
          requestAnimationFrame(() => {
            const newPos = triggerStart + reviewText.length;
            input.setSelectionRange(newPos, newPos);
            input.focus();
          });
          break;
        case 'summarize':
          // Insert summarize request
          const summaryText = 'Please summarize ';
          const summaryValue =
            value.substring(0, triggerStart) + summaryText + value.substring(cursorPos);
          onChange(summaryValue);
          requestAnimationFrame(() => {
            const newPos = triggerStart + summaryText.length;
            input.setSelectionRange(newPos, newPos);
            input.focus();
          });
          break;
      }

      setIsOpen(false);
      setTriggerStart(-1);
      setQuery('');

      // Focus input
      requestAnimationFrame(() => {
        input.focus();
      });
    },
    [inputRef, triggerStart, value, onChange, onPlanModeChange]
  );

  // Handle keyboard navigation
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (!isOpen) return;

      switch (event.key) {
        case 'ArrowDown':
          event.preventDefault();
          setSelectedIndex((prev) => Math.min(prev + 1, filteredCommands.length - 1));
          break;
        case 'ArrowUp':
          event.preventDefault();
          setSelectedIndex((prev) => Math.max(prev - 1, 0));
          break;
        case 'Enter':
        case 'Tab':
          if (filteredCommands.length > 0) {
            event.preventDefault();
            handleSelect(filteredCommands[selectedIndex]);
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
    [isOpen, filteredCommands, selectedIndex, handleSelect]
  );

  // Close menu
  const closeMenu = useCallback(() => {
    setIsOpen(false);
    setTriggerStart(-1);
    setQuery('');
  }, []);

  return {
    isOpen,
    position,
    commands: filteredCommands,
    selectedIndex,
    setSelectedIndex,
    handleChange,
    handleSelect,
    handleKeyDown,
    closeMenu,
  };
}
