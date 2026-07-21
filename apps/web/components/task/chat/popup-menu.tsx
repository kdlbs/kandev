"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";

const MENU_HEIGHT = 280;
const MENU_WIDTH = 420;
const MENU_HEADER_HEIGHT = 32;

type PopupViewport = {
  offsetLeft: number;
  offsetTop: number;
  width: number;
  height: number;
};

export function computePopupMenuStyle(args: {
  position: { x: number; y: number };
  placement: "above" | "below";
  viewport: PopupViewport;
}): React.CSSProperties {
  const margin = 8;
  const viewportRight = args.viewport.offsetLeft + args.viewport.width;
  const viewportBottom = args.viewport.offsetTop + args.viewport.height;
  const width = Math.max(0, Math.min(MENU_WIDTH, args.viewport.width - margin * 2));
  const minLeft = args.viewport.offsetLeft + margin;
  const maxLeft = Math.max(minLeft, viewportRight - width - margin);
  const left = Math.min(Math.max(minLeft, args.position.x), maxLeft);
  const availableHeight =
    args.placement === "above"
      ? Math.max(0, args.position.y - args.viewport.offsetTop - margin * 2)
      : Math.max(0, viewportBottom - args.position.y - margin * 2);
  const maxHeight = Math.min(MENU_HEIGHT, availableHeight);
  const top =
    args.placement === "above"
      ? Math.max(args.viewport.offsetTop + margin, args.position.y - margin - maxHeight)
      : args.position.y + margin;
  return {
    position: "fixed",
    left,
    top,
    width,
    maxWidth: width,
    maxHeight,
    zIndex: 60,
    pointerEvents: "auto",
  };
}

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
  placement = "above",
}: PopupMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const [, setViewportRevision] = useState(0);

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

  useEffect(() => {
    if (!isOpen) return;
    const update = () => setViewportRevision((revision) => revision + 1);
    const viewport = window.visualViewport;
    window.addEventListener("resize", update);
    viewport?.addEventListener("resize", update);
    viewport?.addEventListener("scroll", update);
    return () => {
      window.removeEventListener("resize", update);
      viewport?.removeEventListener("resize", update);
      viewport?.removeEventListener("scroll", update);
    };
  }, [isOpen]);

  // Resolve position from clientRect function or direct position
  const resolvedPosition = (() => {
    if (position) return position;
    if (clientRectFn) {
      const rect = clientRectFn();
      if (rect) return { x: rect.left, y: placement === "below" ? rect.bottom : rect.top };
    }
    return null;
  })();

  if (!isOpen || !resolvedPosition) {
    return null;
  }

  // z-index sits above Radix Dialog overlay/content (both z-50) so the popup
  // renders on top when it is invoked from within a modal (e.g. task-create).
  // pointer-events: auto restores click-handling on the popup itself when
  // Radix Dialog has set pointer-events: none on <body> for modal isolation;
  // without this, the popup is visible but unclickable inside a dialog.
  const visualViewport = window.visualViewport;
  const menuStyle = computePopupMenuStyle({
    position: resolvedPosition,
    placement,
    viewport: visualViewport
      ? {
          offsetLeft: visualViewport.offsetLeft,
          offsetTop: visualViewport.offsetTop,
          width: visualViewport.width,
          height: visualViewport.height,
        }
      : { offsetLeft: 0, offsetTop: 0, width: window.innerWidth, height: window.innerHeight },
  });
  const contentMaxHeight =
    typeof menuStyle.maxHeight === "number"
      ? Math.max(0, menuStyle.maxHeight - MENU_HEADER_HEIGHT)
      : MENU_HEIGHT - MENU_HEADER_HEIGHT;

  const menu = (
    <div
      ref={menuRef}
      data-testid={testId}
      style={menuStyle}
      className="overflow-hidden rounded-lg bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10"
    >
      {/* Header */}
      <div className="border-b border-border/50 px-2 py-1.5">
        <span id={titleId} className="text-xs font-medium text-muted-foreground">
          {title}
        </span>
      </div>

      {/* Content */}
      <div
        role="listbox"
        aria-labelledby={titleId}
        className="overflow-y-auto py-1 scrollbar-thin"
        style={{ maxHeight: contentMaxHeight }}
      >
        {hasItems ? children : emptyState}
      </div>
    </div>
  );

  // Render via portal to escape any overflow containers
  if (typeof document === "undefined") return null;
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
      role="option"
      aria-selected={isSelected}
      className={cn(
        "mx-1 flex min-h-11 w-full cursor-pointer select-none items-center gap-3 rounded-[6px] px-2 py-1.5 text-left text-xs",
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
      <div className="flex min-w-0 flex-1 items-baseline gap-2">
        <span className="max-w-[45%] shrink-0 truncate font-medium">{label}</span>
        {description && (
          <span className="min-w-0 truncate whitespace-nowrap text-[11px] text-muted-foreground">
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
