"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";
import { searchHistory, type SearchHit } from "./message-history";

type MessageHistorySearchProps = {
  isOpen: boolean;
  /** Newest-first list of the user's previous messages for this session. */
  history: readonly string[];
  /** True while older messages are still being paginated in from the backend
   *  (overlay-driven drain). The list updates live as more arrive. */
  isLoadingOlder?: boolean;
  /** Bottom-anchor rect (typically the chat input's bounding rect). The
   *  overlay positions itself directly above this rect. */
  anchorRect: DOMRect | null;
  onClose: () => void;
  /** Invoked when the user picks a result. `index` is the position in
   *  `history` so the editor's history navigation can resume from there. */
  onSelect: (index: number) => void;
};

const OVERLAY_HEIGHT = 280;
const OVERLAY_MAX_WIDTH = 700;

function clampSelectedIndex(prev: number, hitCount: number): number {
  if (hitCount === 0) return 0;
  if (prev >= hitCount) return hitCount - 1;
  if (prev < 0) return 0;
  return prev;
}

function useHits(history: readonly string[], query: string): SearchHit[] {
  return useMemo(() => searchHistory(history, query), [history, query]);
}

/** Track the selected index without an effect: reset to 0 when the query
 *  changes (state-during-render pattern), then clamp against the live hit
 *  count when reading. The setter remains stable so handlers can move it. */
function useSelectedIndex(hitCount: number, query: string) {
  const [rawSelectedIndex, setSelectedIndex] = useState(0);
  const [trackedQuery, setTrackedQuery] = useState(query);
  if (trackedQuery !== query) {
    setTrackedQuery(query);
    setSelectedIndex(0);
  }
  const selectedIndex = clampSelectedIndex(rawSelectedIndex, hitCount);
  return [selectedIndex, setSelectedIndex] as const;
}

type OverlayKeyArgs = {
  hits: SearchHit[];
  selectedIndex: number;
  setSelectedIndex: React.Dispatch<React.SetStateAction<number>>;
  onSelect: (index: number) => void;
  onClose: () => void;
};

function handleOverlayKeyDown(event: React.KeyboardEvent, args: OverlayKeyArgs) {
  const { hits, selectedIndex, setSelectedIndex, onSelect, onClose } = args;
  if (event.key === "ArrowDown") {
    event.preventDefault();
    setSelectedIndex((i) => Math.min(i + 1, hits.length - 1));
    return;
  }
  if (event.key === "ArrowUp") {
    event.preventDefault();
    setSelectedIndex((i) => Math.max(i - 1, 0));
    return;
  }
  if (event.key === "Enter") {
    event.preventDefault();
    const hit = hits[selectedIndex];
    if (hit) onSelect(hit.index);
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    onClose();
  }
}

function useScrollSelectedIntoView(
  selectedIndex: number,
  listRef: React.RefObject<HTMLDivElement | null>,
) {
  useEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const item = list.querySelector<HTMLElement>(`[data-hit-index="${selectedIndex}"]`);
    if (item) item.scrollIntoView({ block: "nearest" });
  }, [selectedIndex, listRef]);
}

function computeStyle(anchorRect: DOMRect | null): React.CSSProperties | null {
  if (!anchorRect) return null;
  const left = Math.max(8, anchorRect.left);
  const maxWidth = Math.min(OVERLAY_MAX_WIDTH, Math.max(200, window.innerWidth - left - 8));
  return {
    position: "fixed",
    left,
    width: Math.min(maxWidth, anchorRect.width || maxWidth),
    bottom: window.innerHeight - anchorRect.top + 8,
    maxHeight: OVERLAY_HEIGHT,
    zIndex: 60,
    pointerEvents: "auto",
  };
}

type HighlightedRowProps = {
  hit: SearchHit;
  isSelected: boolean;
  rowIndex: number;
  onMouseEnter: (rowIndex: number) => void;
  onClick: (historyIndex: number) => void;
};

function HitRow({ hit, isSelected, rowIndex, onMouseEnter, onClick }: HighlightedRowProps) {
  const firstLine = hit.content.split("\n", 1)[0];
  return (
    <button
      type="button"
      data-hit-index={rowIndex}
      data-testid="history-search-row"
      onMouseEnter={() => onMouseEnter(rowIndex)}
      onClick={() => onClick(hit.index)}
      className={cn(
        "flex w-full cursor-pointer select-none items-center gap-2 rounded-[6px] mx-1 px-2 py-1.5 text-xs text-left",
        "hover:bg-muted/50",
        isSelected && "bg-muted/50",
      )}
      style={{ width: "calc(100% - 8px)" }}
    >
      <span className="truncate flex-1">{firstLine || "(empty)"}</span>
    </button>
  );
}

export function MessageHistorySearch({
  isOpen,
  history,
  isLoadingOlder = false,
  anchorRect,
  onClose,
  onSelect,
}: MessageHistorySearchProps) {
  const [query, setQuery] = useState("");
  const hits = useHits(history, query);
  const [selectedIndex, setSelectedIndex] = useSelectedIndex(hits.length, query);
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    requestAnimationFrame(() => inputRef.current?.focus());
  }, []);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        onClose();
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [onClose]);

  useScrollSelectedIntoView(selectedIndex, listRef);

  const style = computeStyle(anchorRect);
  if (!isOpen || !style) return null;
  if (typeof document === "undefined") return null;

  const overlay = (
    <div
      ref={containerRef}
      style={style}
      className="overflow-hidden rounded-lg bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10"
      data-testid="history-search-overlay"
    >
      <div className="flex items-center gap-2 px-2 py-1.5 border-b border-border/50">
        <span className="text-xs font-medium text-muted-foreground shrink-0">
          (reverse-i-search)
        </span>
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) =>
            handleOverlayKeyDown(e, { hits, selectedIndex, setSelectedIndex, onSelect, onClose })
          }
          placeholder="Type to search previous messages..."
          className="flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground"
          data-testid="history-search-input"
        />
        {isLoadingOlder && (
          <span
            className="text-[10px] text-muted-foreground shrink-0"
            data-testid="history-search-loading-older"
          >
            loading older…
          </span>
        )}
      </div>
      <div
        ref={listRef}
        className="overflow-y-auto py-1 scrollbar-thin"
        style={{ maxHeight: OVERLAY_HEIGHT - 36 }}
      >
        {hits.length === 0 ? (
          <div className="px-3 py-2 text-xs text-muted-foreground">No matches</div>
        ) : (
          hits.map((hit, rowIndex) => (
            <HitRow
              key={`${hit.index}-${rowIndex}`}
              hit={hit}
              isSelected={rowIndex === selectedIndex}
              rowIndex={rowIndex}
              onMouseEnter={setSelectedIndex}
              onClick={onSelect}
            />
          ))
        )}
      </div>
    </div>
  );

  return createPortal(overlay, document.body);
}
