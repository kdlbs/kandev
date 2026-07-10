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
      {/* Transparent at rest — the aside's own border-r is the visible edge.
          Colouring this too would double the hairline. Highlights on hover. */}
      <div className="h-full w-px bg-transparent group-hover:bg-primary/60 transition-colors" />
    </button>
  );
}
