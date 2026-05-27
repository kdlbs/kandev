"use client";

type AppSidebarResizeHandleProps = {
  onMouseDown: (e: React.MouseEvent) => void;
};

/**
 * Hairline drag handle on the right edge of the expanded AppSidebar.
 * Hover-visible only; widens slightly when active for affordance.
 */
export function AppSidebarResizeHandle({ onMouseDown }: AppSidebarResizeHandleProps) {
  return (
    <button
      type="button"
      aria-label="Resize sidebar"
      onMouseDown={onMouseDown}
      tabIndex={-1}
      className="absolute right-0 top-0 h-full w-1.5 cursor-ew-resize group flex items-center justify-center"
    >
      <div className="h-full w-px bg-border group-hover:bg-primary/60 transition-colors" />
    </button>
  );
}
