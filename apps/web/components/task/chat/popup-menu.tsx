"use client";

import { useEffect, useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";
import { POPUP_MENU_SIZE, positionPopupMenu } from "./popup-menu-position";

export type PopupMenuProps = {
  isOpen: boolean;
  testId?: string;
  position: { x: number; y: number } | null;
  /** Alternative to position: a function returning a DOMRect (from TipTap suggestion) */
  clientRect?: (() => DOMRect | null) | null;
  title: string;
  selectedIndex: number;
  onClose: () => void;
  children: ReactNode;
  emptyState?: ReactNode;
  hasItems?: boolean;
  listId?: string;
  inline?: boolean;
  /** 'above' (default) positions bottom edge above cursor; 'below' positions top edge below cursor. */
  placement?: "above" | "below";
};

export function PopupMenu({
  isOpen,
  testId,
  position,
  clientRect: clientRectFn,
  title,
  onClose,
  children,
  emptyState,
  hasItems = true,
  listId,
  inline,
  placement = "above",
}: PopupMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const hasAnchor = isOpen && Boolean(inline || position || clientRectFn?.());

  // Close on click outside
  useEffect(() => {
    if (!isOpen) return;

    const handlePointerOutside = (event: PointerEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        onClose();
      }
    };

    document.addEventListener("pointerdown", handlePointerOutside);
    return () => document.removeEventListener("pointerdown", handlePointerOutside);
  }, [isOpen, onClose]);

  // A virtual caret can move between result renders without an element resize.
  useLayoutEffect(() => {
    const menu = menuRef.current;
    if (inline || !hasAnchor || !menu) return;
    return positionPopupMenu(
      menu,
      () => (position ? new DOMRect(position.x, position.y) : (clientRectFn?.() ?? null)),
      placement,
    );
  }, [inline, hasAnchor, position, clientRectFn, placement, children, hasItems, emptyState]);

  if (!hasAnchor) {
    return null;
  }

  // z-index sits above Radix Dialog overlay/content (both z-50) so the popup
  // renders on top when it is invoked from within a modal (e.g. task-create).
  // pointer-events: auto restores click-handling on the popup itself when
  // Radix Dialog has set pointer-events: none on <body> for modal isolation;
  // without this, the popup is visible but unclickable inside a dialog.
  const menu = (
    <div
      ref={menuRef}
      data-testid={testId}
      style={
        inline
          ? undefined
          : {
              ...POPUP_MENU_SIZE,
              position: "fixed",
              visibility: "hidden",
              zIndex: 60,
              pointerEvents: "auto",
            }
      }
      className={cn(
        "flex min-w-0 flex-col overflow-hidden rounded-lg bg-popover text-popover-foreground",
        inline ? "mt-1 border border-border" : "shadow-md ring-1 ring-foreground/10",
      )}
    >
      {/* Header */}
      <div className="shrink-0 border-b border-border/50 px-2 py-1.5">
        <span id={titleId} className="text-xs font-medium text-muted-foreground">
          {title}
        </span>
      </div>

      {/* Content */}
      <div
        role="listbox"
        id={listId}
        aria-labelledby={titleId}
        className={cn("min-h-0 py-1", !inline && "overflow-y-auto scrollbar-thin")}
      >
        {hasItems ? children : emptyState}
      </div>
    </div>
  );

  // Render via portal to escape any overflow containers
  if (inline) return menu;
  return typeof document === "undefined" ? null : createPortal(menu, document.body);
}

export type PopupMenuItemProps = {
  icon: ReactNode;
  label: string;
  description?: string;
  badges?: ReactNode;
  hint?: string;
  accessibleLabel?: string;
  id?: string;
  compact?: boolean;
  isSelected: boolean;
  onClick: () => void;
  onMouseEnter: () => void;
  itemRef?: (el: HTMLButtonElement | null) => void;
};

export function PopupMenuItem({
  icon,
  label,
  description,
  badges,
  hint,
  accessibleLabel,
  id,
  compact,
  isSelected,
  onClick,
  onMouseEnter,
  itemRef,
}: PopupMenuItemProps) {
  return (
    <button
      ref={itemRef}
      type="button"
      role="option"
      id={id}
      tabIndex={id ? -1 : undefined}
      aria-selected={isSelected}
      aria-label={accessibleLabel}
      className={cn(
        "mx-1 flex w-full cursor-pointer select-none items-center gap-3 rounded-[6px] px-2 py-1.5 text-left text-xs",
        compact ? "min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11" : "min-h-11",
        "hover:bg-muted/50",
        isSelected && "bg-muted/50",
      )}
      style={{ width: "calc(100% - 8px)" }}
      onPointerDown={(event) => event.preventDefault()}
      onClick={onClick}
      onMouseEnter={onMouseEnter}
    >
      <div className="flex h-4 w-4 shrink-0 items-center justify-center text-muted-foreground">
        {icon}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex min-w-0 items-center gap-2">
          <span className="max-w-[45%] shrink-0 truncate font-medium">{label}</span>
          {badges}
          {description && (
            <span className="min-w-0 flex-1 truncate whitespace-nowrap text-[11px] text-muted-foreground">
              {description}
            </span>
          )}
        </div>
        {hint && (
          <span className="min-w-0 truncate text-[10px] leading-tight text-muted-foreground">
            {hint}
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
      selectedItem.scrollIntoView({ block: "nearest" });
    }
  }, [selectedIndex]);

  const setItemRef = (index: number) => (el: HTMLButtonElement | null) => {
    if (el) {
      itemRefs.current.set(index, el);
    }
  };

  return { setItemRef };
}
