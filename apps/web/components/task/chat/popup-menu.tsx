'use client';

import { useEffect, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { cn } from '@/lib/utils';

const MENU_HEIGHT = 280;
const MENU_MAX_WIDTH = 700;

export type PopupMenuProps = {
  isOpen: boolean;
  position: { x: number; y: number } | null;
  /** Alternative to position: a function returning a DOMRect (from TipTap suggestion) */
  clientRect?: (() => DOMRect | null) | null;
  title: string;
  selectedIndex: number;
  onClose: () => void;
  children: ReactNode;
  emptyState?: ReactNode;
  hasItems?: boolean;
  /** 'above' (default) positions bottom edge above cursor; 'below' positions top edge below cursor. */
  placement?: 'above' | 'below';
};

export function PopupMenu({
  isOpen,
  position,
  clientRect: clientRectFn,
  title,
  onClose,
  children,
  emptyState,
  hasItems = true,
  placement = 'above',
}: PopupMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

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

  // Resolve position from clientRect function or direct position
  const resolvedPosition = (() => {
    if (position) return position;
    if (clientRectFn) {
      const rect = clientRectFn();
      if (rect) return { x: rect.left, y: placement === 'below' ? rect.bottom : rect.top };
    }
    return null;
  })();

  if (!isOpen || !resolvedPosition) {
    return null;
  }

  // Calculate position with content-based width
  const menuStyle: React.CSSProperties = {
    position: 'fixed',
    left: Math.max(8, resolvedPosition.x),
    maxWidth: Math.min(MENU_MAX_WIDTH, window.innerWidth - resolvedPosition.x - 8),
    maxHeight: MENU_HEIGHT,
    zIndex: 50,
    ...(placement === 'below'
      ? { top: resolvedPosition.y + 8 }
      : { bottom: window.innerHeight - resolvedPosition.y + 8 }),
  };

  const menu = (
    <div
      ref={menuRef}
      style={menuStyle}
      className="overflow-hidden rounded-lg bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10"
    >
      {/* Header */}
      <div className="px-2 py-1.5 border-b border-border/50">
        <span className="text-xs font-medium text-muted-foreground">{title}</span>
      </div>

      {/* Content */}
      <div className="overflow-y-auto py-1 scrollbar-thin" style={{ maxHeight: MENU_HEIGHT - 36 }}>
        {hasItems ? children : emptyState}
      </div>
    </div>
  );

  // Render via portal to escape any overflow containers
  if (typeof document === 'undefined') return null;
  return createPortal(menu, document.body);
}

export type PopupMenuItemProps = {
  icon: ReactNode;
  label: string;
  description?: string;
  isSelected: boolean;
  onClick: () => void;
  onMouseEnter: () => void;
  itemRef?: (el: HTMLButtonElement | null) => void;
};

export function PopupMenuItem({
  icon,
  label,
  description,
  isSelected,
  onClick,
  onMouseEnter,
  itemRef,
}: PopupMenuItemProps) {
  return (
    <button
      ref={itemRef}
      type="button"
      className={cn(
        'flex w-full cursor-pointer select-none items-center gap-3 rounded-[6px] mx-1 px-2 py-1.5 text-xs text-left',
        'hover:bg-muted/50',
        isSelected && 'bg-muted/50'
      )}
      style={{ width: 'calc(100% - 8px)' }}
      onClick={onClick}
      onMouseEnter={onMouseEnter}
    >
      <div className="h-4 w-4 text-muted-foreground shrink-0 flex items-center justify-center">
        {icon}
      </div>
      <div className="flex items-baseline gap-2 min-w-0 flex-1">
        <span className="shrink-0 font-medium">{label}</span>
        {description && (
          <span className="text-[11px] text-muted-foreground whitespace-nowrap">
            {description}
          </span>
        )}
      </div>
    </button>
  );
}

// Hook for scroll-into-view behavior
export function useMenuItemRefs(selectedIndex: number) {
  const itemRefs = useRef<Map<number, HTMLButtonElement>>(new Map());

  useEffect(() => {
    const selectedItem = itemRefs.current.get(selectedIndex);
    if (selectedItem) {
      selectedItem.scrollIntoView({ block: 'nearest' });
    }
  }, [selectedIndex]);

  const setItemRef = (index: number) => (el: HTMLButtonElement | null) => {
    if (el) {
      itemRefs.current.set(index, el);
    }
  };

  return { setItemRef };
}
