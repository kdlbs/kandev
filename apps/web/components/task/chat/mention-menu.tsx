'use client';

import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { IconAt, IconFile, IconFolder } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import type { MentionItem } from '@/hooks/use-inline-mention';

type MentionMenuProps = {
  isOpen: boolean;
  isLoading: boolean;
  position: { x: number; y: number } | null;
  items: MentionItem[];
  query: string;
  selectedIndex: number;
  onSelect: (item: MentionItem) => void;
  onClose: () => void;
  setSelectedIndex: (index: number) => void;
};

const MENU_HEIGHT = 280;
const MENU_WIDTH = 320;

// Extract filename and parent path from a full path
function parseFilePath(filePath: string): { name: string; parent: string } {
  const parts = filePath.split('/');
  const name = parts.pop() || filePath;
  const parent = parts.length > 0 ? parts.join('/') + '/' : '';
  return { name, parent };
}

// Check if path looks like a directory (ends with / or has no extension)
function isDirectory(filePath: string): boolean {
  if (filePath.endsWith('/')) return true;
  const name = filePath.split('/').pop() || '';
  return !name.includes('.');
}

export function MentionMenu({
  isOpen,
  isLoading,
  position,
  items,
  query,
  selectedIndex,
  onSelect,
  onClose,
  setSelectedIndex,
}: MentionMenuProps) {
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

  if (!isOpen || !position) {
    return null;
  }

  // Calculate position (above cursor)
  const menuStyle: React.CSSProperties = {
    position: 'fixed',
    left: Math.max(8, Math.min(position.x, window.innerWidth - MENU_WIDTH - 8)),
    bottom: window.innerHeight - position.y + 8,
    width: MENU_WIDTH,
    maxHeight: MENU_HEIGHT,
    zIndex: 50,
  };

  // Group items by type
  const prompts = items.filter((item) => item.type === 'prompt');
  const files = items.filter((item) => item.type === 'file');

  // Calculate indices for sections
  let currentIndex = 0;

  const menu = (
    <div
      ref={menuRef}
      style={menuStyle}
      className="overflow-hidden rounded-lg bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10 text-xs"
    >
      {/* Header */}
      <div className="px-2 py-1.5 border-b border-border/50">
        <span className="text-xs font-medium text-muted-foreground">Mention files, prompts</span>
      </div>

      {/* Content */}
      <div className="overflow-y-auto py-1 scrollbar-thin" style={{ maxHeight: MENU_HEIGHT - 36 }}>
        {items.length === 0 ? (
          <div className="px-3 py-1 text-center text-xs text-muted-foreground">
            {isLoading ? 'Loading...' : query ? 'No results found' : 'Type to search...'}
          </div>
        ) : (
          <>
            {prompts.length > 0 && (
              <div>
                {prompts.map((item) => {
                  const index = currentIndex++;
                  return (
                    <button
                      key={item.id}
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
                      onClick={() => onSelect(item)}
                      onMouseEnter={() => setSelectedIndex(index)}
                    >
                      <IconAt className="h-4 w-4 text-muted-foreground shrink-0" />
                      <div className="flex items-baseline gap-2 min-w-0 flex-1">
                        <span className="shrink-0">{item.label}</span>
                        {item.description && (
                          <span className="text-[11px] text-muted-foreground truncate">
                            {item.description}
                          </span>
                        )}
                      </div>
                    </button>
                  );
                })}
              </div>
            )}

            {files.length > 0 && (
              <div>
                {files.map((item) => {
                  const index = currentIndex++;
                  const { name, parent } = parseFilePath(item.label);
                  const isDir = isDirectory(item.label);

                  return (
                    <button
                      key={item.id}
                      ref={(el) => {
                        if (el) itemRefs.current.set(index, el);
                      }}
                      type="button"
                      className={cn(
                        'flex w-full cursor-pointer select-none items-center gap-3 rounded-[6px] mx-1 px-2 py-1.5 text-xs text-left',
                        'hover:bg-muted/50',
                        selectedIndex === index && 'bg-muted/50'
                      )}
                      style={{ width: 'calc(100% - 8px)' }}
                      onClick={() => onSelect(item)}
                      onMouseEnter={() => setSelectedIndex(index)}
                    >
                      {isDir ? (
                        <IconFolder className="h-4 w-4 text-muted-foreground shrink-0" />
                      ) : (
                        <IconFile className="h-4 w-4 text-muted-foreground shrink-0" />
                      )}
                      <div className="flex items-baseline gap-2 min-w-0 flex-1">
                        <span className="shrink-0">{name}</span>
                        {parent && (
                          <span className="text-[11px] text-muted-foreground truncate">{parent}</span>
                        )}
                      </div>
                    </button>
                  );
                })}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );

  // Render via portal to escape any overflow containers
  if (typeof document === 'undefined') return null;
  return createPortal(menu, document.body);
}
