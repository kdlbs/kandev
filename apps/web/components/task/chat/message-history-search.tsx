"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { searchHistory, type MessageHistoryEntry, type SearchHit } from "./message-history";
import { HitRow } from "./message-history-search-row";
import { useTranslation } from "react-i18next";

type MessageHistorySearchProps = {
  /** Newest-first list of the user's previous messages for this session. */
  history: readonly MessageHistoryEntry[];
  /** True while older messages are still being paginated in from the backend
   *  (overlay-driven drain). The list updates live as more arrive. */
  isLoadingOlder?: boolean;
  /** Bottom-anchor rect (typically the chat input's bounding rect). The
   *  overlay positions itself directly above this rect. */
  anchorRect: DOMRect | null;
  /** DOM node to portal the overlay into. On the main task chat panel this is
   *  `document.body`; on Quick Chat it must be a node inside the Dialog's
   *  FocusScope (e.g. `[data-slot="dialog-content"]`) or the Dialog's focus
   *  trap reverts focus away from this overlay's input on every render. */
  container: Element;
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

function useHits(history: readonly MessageHistoryEntry[], query: string): SearchHit[] {
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
    // Claim the key here: once the reverse-search overlay is closed, nothing
    // further up the tree (e.g. a clarification panel's own
    // Escape-collapses handler) should also react to the same keypress.
    event.stopPropagation();
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

export type ContainerPaddingBox = {
  /** Padding-box left edge, in viewport coordinates. */
  left: number;
  /** Padding-box width (border-box width minus both border widths). */
  width: number;
  /** Padding-box bottom edge, in viewport coordinates. */
  bottom: number;
};

type PaddingBoxSource = {
  getBoundingClientRect: () => DOMRect;
  clientLeft: number;
  clientTop: number;
  clientWidth: number;
  clientHeight: number;
};

/**
 * The containing block a `position: fixed` descendant of a transformed
 * ancestor resolves against is that ancestor's PADDING box, not its border
 * box -- `getBoundingClientRect()` alone returns the border box. Quick Chat's
 * `DialogContent` has a real border (`dialog.tsx`), so using the border box
 * directly offset the overlay by the border width and left `maxWidth` short
 * by twice it. `clientLeft`/`clientTop` are exactly the container's own
 * border widths, so subtracting them (via the border-box rect) recovers the
 * padding box. Takes a minimal structural source rather than `Element` so the
 * arithmetic is unit-testable without a real layout engine (jsdom always
 * reports 0 for client* metrics, which would silently hide this bug).
 */
export function containerPaddingBox(container: PaddingBoxSource): ContainerPaddingBox {
  const rect = container.getBoundingClientRect();
  return {
    left: rect.left + container.clientLeft,
    width: container.clientWidth,
    bottom: rect.top + container.clientTop + container.clientHeight,
  };
}

/**
 * Position math is relative to `containerBox`, the padding box of the portal
 * target, not the viewport. A `document.body` target has no transform of its
 * own, so `containerBox` is null there and the origin collapses to (0, 0,
 * window.innerWidth, window.innerHeight) -- identical to the previous
 * viewport-relative formula. Quick Chat's DialogContent carries a permanent
 * `translate` for centering, which establishes a new containing block for
 * `position: fixed` descendants portaled inside it, so once the overlay
 * renders there its rect must be computed relative to that ancestor's padding
 * box instead (see `containerPaddingBox` above).
 */
export function computeStyle(
  anchorRect: DOMRect | null,
  containerBox: ContainerPaddingBox | null,
): React.CSSProperties | null {
  if (!anchorRect) return null;
  const originLeft = containerBox?.left ?? 0;
  const originWidth = containerBox?.width ?? window.innerWidth;
  const originBottom = containerBox?.bottom ?? window.innerHeight;
  const left = Math.max(8, anchorRect.left - originLeft);
  const maxWidth = Math.min(OVERLAY_MAX_WIDTH, Math.max(200, originWidth - left - 8));
  return {
    position: "fixed",
    left,
    width: Math.min(maxWidth, anchorRect.width || maxWidth),
    bottom: originBottom - anchorRect.top + 8,
    maxHeight: OVERLAY_HEIGHT,
    zIndex: 60,
    pointerEvents: "auto",
  };
}

// Fallback close, independent of exact focus. The input's own onKeyDown
// (below, in MessageHistorySearch) handles the common case, but nothing else
// closes the overlay if focus ever leaves the input while staying inside the
// overlay (there is no blur handler on this popup). Without this, Quick
// Chat's `CLAIM_ANY_ESCAPE` registration (tiptap-input.tsx's
// useReverseSearchOverlay) still suppresses Radix's auto-dismiss, but nothing
// then closes the overlay, so Escape goes silently inert. `document`-level
// listeners run before `window`-level ones in native bubble order, so this
// also runs ahead of the clarification carousel's `window` listener --
// fixing the "collapses the clarification instead of closing the overlay"
// case that could otherwise occur depending on escape-guard registration
// order.
//
// Ownership is checked against this popup's own `containerRef` first (the
// precise case), then against `container` (the portal target) as a fallback
// -- but ONLY when `container` is a modal's `[data-slot="dialog-content"]`,
// not `document.body`. On Quick Chat that ancestor is unique to this one
// composer's dialog, so it still catches focus that tabs out of the input to
// elsewhere in the same dialog (the literal "tab out of the input" repro --
// the reverse-search input is the overlay's only focusable element, so
// tabbing away always leaves `containerRef`). Using `container` when it's
// `document.body` would instead claim the entire document for whichever
// composer's listener happens to be registered first, mishandling Escape for
// a second, unrelated overlay -- the same cross-composer bug F10 fixed for
// suggestion popups (see use-suggestion-escape-fallback.ts).
function useReverseSearchEscapeFallback(
  containerRef: React.RefObject<HTMLDivElement | null>,
  container: Element,
  onClose: () => void,
) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      const el = containerRef.current;
      if (!el) return;
      const target = event.target as Node | null;
      const active = document.activeElement;
      const ownedByPopup = (target && el.contains(target)) || (active && el.contains(active));
      const ownedByDialog =
        container !== document.body &&
        ((target && container.contains(target)) || (active && container.contains(active)));
      if (!ownedByPopup && !ownedByDialog) return;
      event.preventDefault();
      event.stopPropagation();
      onClose();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose, container, containerRef]);
}

export function MessageHistorySearch({
  history,
  isLoadingOlder = false,
  anchorRect,
  container,
  onClose,
  onSelect,
}: MessageHistorySearchProps) {
  const { t } = useTranslation();
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

  useReverseSearchEscapeFallback(containerRef, container, onClose);
  useScrollSelectedIntoView(selectedIndex, listRef);

  if (typeof document === "undefined") return null;
  const containerBox = container === document.body ? null : containerPaddingBox(container);
  const style = computeStyle(anchorRect, containerBox);
  if (!style) return null;

  const overlay = (
    <div
      ref={containerRef}
      style={style}
      className="overflow-hidden rounded-lg bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10"
      data-testid="history-search-overlay"
    >
      <div className="flex items-center gap-2 px-2 py-1.5 border-b border-border/50">
        <span className="text-xs font-medium text-muted-foreground shrink-0">
          {t("task:reverseISearch")}
        </span>
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) =>
            handleOverlayKeyDown(e, { hits, selectedIndex, setSelectedIndex, onSelect, onClose })
          }
          placeholder={t("task:typeToSearchPreviousMessages")}
          className="flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground"
          data-testid="history-search-input"
        />
        {isLoadingOlder && (
          <span
            className="text-[10px] text-muted-foreground shrink-0"
            data-testid="history-search-loading-older"
          >
            {t("task:loadingOlder")}
          </span>
        )}
      </div>
      <div
        ref={listRef}
        className="overflow-y-auto py-1 scrollbar-thin"
        style={{ maxHeight: OVERLAY_HEIGHT - 36 }}
      >
        {hits.length === 0 ? (
          <div className="px-3 py-2 text-xs text-muted-foreground">{t("task:noMatches")}</div>
        ) : (
          hits.map((hit, rowIndex) => (
            <HitRow
              key={hit.index}
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

  return createPortal(overlay, container);
}
