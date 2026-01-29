'use client';

import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { IconCommand } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import type { SlashCommand } from '@/hooks/use-inline-slash';

type SlashCommandMenuProps = {
  isOpen: boolean;
  position: { x: number; y: number } | null;
  commands: SlashCommand[];
  selectedIndex: number;
  onSelect: (command: SlashCommand) => void;
  onClose: () => void;
  setSelectedIndex: (index: number) => void;
};

const MENU_HEIGHT = 280;
const MENU_WIDTH = 320;

export function SlashCommandMenu({
  isOpen,
  position,
  commands,
  selectedIndex,
  onSelect,
  onClose,
  setSelectedIndex,
}: SlashCommandMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<Map<number, HTMLButtonElement>>(new Map());

  // Scroll selected item into view
  useEffect(() => {
    const selectedItem = itemRefs.current.get(selectedIndex);
    if (selectedItem) {
      selectedItem.scrollIntoView({ block: 'nearest' });
    }
  }, [selectedIndex]);

  // Close on click outside
  useEffect(() => {
    if (!isOpen) return;

    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        onClose();
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen, onClose]);

  if (!isOpen || !position || commands.length === 0) {
    return null;
  }

  // Calculate position (above cursor)
  const menuStyle: React.CSSProperties = {
    position: 'fixed',
    left: Math.max(8, Math.min(position.x - 16, window.innerWidth - MENU_WIDTH - 8)),
    bottom: window.innerHeight - position.y + 8,
    width: MENU_WIDTH,
    maxHeight: MENU_HEIGHT,
    zIndex: 50,
  };

  const menu = (
    <div
      ref={menuRef}
      style={menuStyle}
      className="overflow-hidden rounded-lg bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10"
    >
      {/* Header */}
      <div className="px-2 py-1.5 border-b border-border/50">
        <span className="text-xs font-medium text-muted-foreground">Commands</span>
      </div>

      {/* Content */}
      <div className="overflow-y-auto py-1 scrollbar-thin" style={{ maxHeight: MENU_HEIGHT - 36 }}>
        {commands.map((command, index) => (
          <button
            key={command.id}
            ref={(el) => {
              if (el) itemRefs.current.set(index, el);
            }}
            type="button"
            className={cn(
              'flex w-full cursor-pointer select-none items-center gap-3 rounded-[6px] mx-1 px-2 py-1.5 text-[13px] text-left',
              'hover:bg-muted/50',
              selectedIndex === index && 'bg-muted/50'
            )}
            style={{ width: 'calc(100% - 8px)' }}
            onClick={() => onSelect(command)}
            onMouseEnter={() => setSelectedIndex(index)}
          >
            <IconCommand className="h-4 w-4 text-muted-foreground shrink-0" />
            <div className="min-w-0 flex-1">
              <div className="truncate">{command.label}</div>
              <div className="text-[11px] text-muted-foreground">{command.description}</div>
            </div>
          </button>
        ))}
      </div>
    </div>
  );

  // Render via portal to escape any overflow containers
  if (typeof document === 'undefined') return null;
  return createPortal(menu, document.body);
}
