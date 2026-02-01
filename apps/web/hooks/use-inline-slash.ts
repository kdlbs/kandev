'use client';

import { useState, useCallback, useMemo, useEffect } from 'react';
import type { RichTextInputHandle } from '@/components/task/chat/rich-text-input';
import { useAppStore } from '@/components/state-provider';

export type SlashCommandAction = 'agent';

export type SlashCommand = {
  id: string;
  label: string;
  description: string;
  action: SlashCommandAction;
  // For agent commands
  agentCommandName?: string;
};

type Position = {
  x: number;
  y: number;
};

function isValidSlashTrigger(text: string, pos: number): boolean {
  if (pos === 0) return true;
  const charBefore = text[pos - 1];
  return charBefore === ' ' || charBefore === '\n' || charBefore === '\t';
}

function filterCommands(query: string, allCommands: SlashCommand[]): SlashCommand[] {
  if (!query) return allCommands;
  const lowerQuery = query.toLowerCase();

  return allCommands.filter((cmd) => {
    const label = cmd.label.toLowerCase();
    const cmdName = cmd.agentCommandName?.toLowerCase();
    return label.startsWith('/' + lowerQuery) || cmdName?.startsWith(lowerQuery);
  }).sort((a, b) => {
    // Prefer exact prefix matches
    const aName = a.agentCommandName?.toLowerCase();
    const bName = b.agentCommandName?.toLowerCase();
    const aStartsWithQuery = aName?.startsWith(lowerQuery) ?? false;
    const bStartsWithQuery = bName?.startsWith(lowerQuery) ?? false;
    if (aStartsWithQuery && !bStartsWithQuery) return -1;
    if (!aStartsWithQuery && bStartsWithQuery) return 1;
    return 0;
  });
}

type UseInlineSlashOptions = {
  sessionId?: string | null;
  onAgentCommand?: (commandName: string) => void;
};

export function useInlineSlash(
  inputRef: React.RefObject<RichTextInputHandle | null>,
  value: string,
  onChange: (value: string) => void,
  options?: UseInlineSlashOptions
) {
  const { sessionId, onAgentCommand } = options ?? {};
  const [isOpen, setIsOpen] = useState(false);
  const [position, setPosition] = useState<Position | null>(null);
  const [triggerStart, setTriggerStart] = useState<number>(-1);
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);

  // Get agent commands from store
  const agentCommands = useAppStore((state) =>
    sessionId ? state.availableCommands.bySessionId[sessionId] : undefined
  );

  // Convert agent commands to slash commands
  // Filter out "bundled" commands (skills) that don't produce visible output
  const allCommands = useMemo(() => {
    if (!agentCommands || agentCommands.length === 0) {
      return [];
    }

    return agentCommands
      .filter((cmd) => {
        // Skip bundled skills - they don't produce <local-command-stdout> output
        const desc = cmd.description || '';
        return !desc.includes('(bundled)');
      })
      .map((cmd) => ({
        id: `agent-${cmd.name}`,
        label: `/${cmd.name}`,
        description: cmd.description || `Run /${cmd.name} command`,
        action: 'agent' as const,
        agentCommandName: cmd.name,
      }));
  }, [agentCommands]);

  // Filter commands based on query
  const filteredCommands = useMemo(() => filterCommands(query, allCommands), [query, allCommands]);

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

      // Execute the command action - only agent commands are supported now
      if (command.agentCommandName && onAgentCommand) {
        onAgentCommand(command.agentCommandName);
      }

      setIsOpen(false);
      setTriggerStart(-1);
      setQuery('');

      // Focus input
      requestAnimationFrame(() => {
        input.focus();
      });
    },
    [inputRef, triggerStart, value, onChange, onAgentCommand]
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
