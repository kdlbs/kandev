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

/** Detect a /-trigger in text before cursor and return the query, or null if none. */
function detectSlashTrigger(text: string, cursorPos: number): { triggerStart: number; query: string } | null {
  const textBeforeCursor = text.substring(0, cursorPos);
  const lastSlashIndex = textBeforeCursor.lastIndexOf('/');
  if (lastSlashIndex < 0 || !isValidSlashTrigger(text, lastSlashIndex)) return null;
  const textAfterSlash = textBeforeCursor.substring(lastSlashIndex + 1);
  if (/\s/.test(textAfterSlash) || !/^[\w-]*$/.test(textAfterSlash)) return null;
  return { triggerStart: lastSlashIndex, query: textAfterSlash };
}

function filterCommands(query: string, allCommands: SlashCommand[]): SlashCommand[] {
  if (!query) return allCommands;
  const lowerQuery = query.toLowerCase();

  return allCommands.filter((cmd) => {
    const label = cmd.label.toLowerCase();
    const cmdName = cmd.agentCommandName?.toLowerCase();
    return label.startsWith('/' + lowerQuery) || cmdName?.startsWith(lowerQuery);
  }).sort((a, b) => {
    const aName = a.agentCommandName?.toLowerCase();
    const bName = b.agentCommandName?.toLowerCase();
    const aStartsWithQuery = aName?.startsWith(lowerQuery) ?? false;
    const bStartsWithQuery = bName?.startsWith(lowerQuery) ?? false;
    if (aStartsWithQuery && !bStartsWithQuery) return -1;
    if (!aStartsWithQuery && bStartsWithQuery) return 1;
    return 0;
  });
}

type SlashKeyboardParams = {
  isOpen: boolean;
  filteredCommands: SlashCommand[];
  selectedIndex: number;
  setSelectedIndex: (v: number | ((prev: number) => number)) => void;
  handleSelect: (cmd: SlashCommand) => void;
  closeMenu: () => void;
};

function useSlashKeyboard({
  isOpen, filteredCommands, selectedIndex, setSelectedIndex, handleSelect, closeMenu,
}: SlashKeyboardParams) {
  return useCallback(
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
          closeMenu();
          break;
      }
    },
    [isOpen, filteredCommands, selectedIndex, setSelectedIndex, handleSelect, closeMenu]
  );
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

  const agentCommands = useAppStore((state) =>
    sessionId ? state.availableCommands.bySessionId[sessionId] : undefined
  );

  const allCommands = useMemo(() => {
    if (!agentCommands || agentCommands.length === 0) return [];
    return agentCommands
      .filter((cmd) => {
        const desc = cmd.description || '';
        return !desc.includes('(bundled)');
      })
      .map((cmd) => ({
        id: `agent-${cmd.name}`, label: `/${cmd.name}`,
        description: cmd.description || `Run /${cmd.name} command`,
        action: 'agent' as const, agentCommandName: cmd.name,
      }));
  }, [agentCommands]);

  const filteredCommands = useMemo(() => filterCommands(query, allCommands), [query, allCommands]);

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => { setSelectedIndex(0); }, [filteredCommands.length]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const closeMenu = useCallback(() => {
    setIsOpen(false);
    setTriggerStart(-1);
    setQuery('');
  }, []);

  const handleChange = useCallback(
    (newValue: string) => {
      onChange(newValue);
      const input = inputRef.current;
      if (!input) return;
      requestAnimationFrame(() => {
        const cursorPos = input.getSelectionStart();
        const trigger = detectSlashTrigger(newValue, cursorPos);
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
        if (isOpen) closeMenu();
      });
    },
    [inputRef, isOpen, onChange, closeMenu]
  );

  const handleSelect = useCallback(
    (command: SlashCommand) => {
      const input = inputRef.current;
      if (!input || triggerStart < 0) return;
      const cursorPos = input.getSelectionStart();
      onChange(value.substring(0, triggerStart) + value.substring(cursorPos));
      if (command.agentCommandName && onAgentCommand) {
        onAgentCommand(command.agentCommandName);
      }
      closeMenu();
      requestAnimationFrame(() => { input.focus(); });
    },
    [inputRef, triggerStart, value, onChange, onAgentCommand, closeMenu]
  );

  const handleKeyDown = useSlashKeyboard({ isOpen, filteredCommands, selectedIndex, setSelectedIndex, handleSelect, closeMenu });

  return {
    isOpen, position, commands: filteredCommands,
    selectedIndex, setSelectedIndex,
    handleChange, handleSelect, handleKeyDown, closeMenu,
  };
}
