'use client';

import { IconAt, IconFile, IconFolder } from '@tabler/icons-react';
import type { MentionItem } from '@/hooks/use-inline-mention';
import { PopupMenu, PopupMenuItem, useMenuItemRefs } from './popup-menu';

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

// Get the appropriate icon for an item
function getItemIcon(item: MentionItem) {
  if (item.type === 'prompt') {
    return <IconAt className="h-4 w-4" />;
  }
  const isDir = isDirectory(item.label);
  return isDir ? <IconFolder className="h-4 w-4" /> : <IconFile className="h-4 w-4" />;
}

// Get the label and description for an item
function getItemDisplay(item: MentionItem): { label: string; description?: string } {
  if (item.type === 'prompt') {
    return { label: item.label, description: item.description };
  }
  const { name, parent } = parseFilePath(item.label);
  return { label: name, description: parent || undefined };
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
  const { setItemRef } = useMenuItemRefs(selectedIndex);

  const emptyState = (
    <div className="px-3 py-1 text-center text-xs text-muted-foreground">
      {isLoading ? 'Loading...' : query ? 'No results found' : 'Type to search...'}
    </div>
  );

  return (
    <PopupMenu
      isOpen={isOpen}
      position={position}
      title="Mention files, prompts"
      selectedIndex={selectedIndex}
      onClose={onClose}
      hasItems={items.length > 0}
      emptyState={emptyState}
    >
      {items.map((item, index) => {
        const { label, description } = getItemDisplay(item);
        return (
          <PopupMenuItem
            key={item.id}
            icon={getItemIcon(item)}
            label={label}
            description={description}
            isSelected={selectedIndex === index}
            onClick={() => onSelect(item)}
            onMouseEnter={() => setSelectedIndex(index)}
            itemRef={setItemRef(index)}
          />
        );
      })}
    </PopupMenu>
  );
}
