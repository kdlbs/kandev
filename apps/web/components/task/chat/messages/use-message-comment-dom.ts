"use client";

import { useEffect, useLayoutEffect, useState, type RefObject } from "react";
import {
  getMessageCommentDecorations,
  getMessageSelection,
  type MessageCommentDecoration,
  type MessageSelection,
} from "@/lib/chat/agent-message-comments";
import type { AgentMessageComment } from "@/lib/state/slices/comments";

type ShortcutHandler = (selection: MessageSelection) => void;
type ShortcutManager = {
  handlers: Map<HTMLElement, ShortcutHandler>;
  listener: (event: KeyboardEvent) => void;
};

const shortcutManagers = new WeakMap<Document, ShortcutManager>();

function registerMessageCommentShortcut(root: HTMLElement, handler: ShortcutHandler) {
  const ownerDocument = root.ownerDocument;
  let manager = shortcutManagers.get(ownerDocument);
  if (!manager) {
    const handlers = new Map<HTMLElement, ShortcutHandler>();
    const listener = (event: KeyboardEvent) => {
      if (!((event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === "c"))
        return;
      const selection = ownerDocument.getSelection();
      const anchorElement =
        selection?.anchorNode instanceof Element
          ? selection.anchorNode
          : selection?.anchorNode?.parentElement;
      const selectedRoot = anchorElement?.closest<HTMLElement>("[data-agent-message-body]");
      if (!selectedRoot) return;
      const selectedHandler = handlers.get(selectedRoot);
      if (!selectedHandler) return;
      const messageSelection = getMessageSelection(selectedRoot, selection);
      if (!messageSelection) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      selectedHandler(messageSelection);
    };
    manager = { handlers, listener };
    shortcutManagers.set(ownerDocument, manager);
    ownerDocument.addEventListener("keydown", listener, true);
  }
  manager.handlers.set(root, handler);
  return () => {
    const current = shortcutManagers.get(ownerDocument);
    if (!current) return;
    current.handlers.delete(root);
    if (current.handlers.size > 0) return;
    ownerDocument.removeEventListener("keydown", current.listener, true);
    shortcutManagers.delete(ownerDocument);
  };
}

export function useMessageCommentShortcut(
  rootRef: RefObject<HTMLDivElement | null>,
  isSelectable: boolean,
  onSelect: ShortcutHandler,
) {
  useEffect(() => {
    const root = rootRef.current;
    if (!isSelectable || !root) return;
    return registerMessageCommentShortcut(root, onSelect);
  }, [isSelectable, onSelect, rootRef]);
}

type HighlightRegistry = {
  set: (name: string, highlight: unknown) => void;
  delete: (name: string) => void;
};

type HighlightConstructor = new (...ranges: Range[]) => unknown;

function highlightRegistry() {
  return (
    globalThis as typeof globalThis & {
      CSS?: typeof CSS & { highlights?: HighlightRegistry };
    }
  ).CSS?.highlights;
}

function updateCustomHighlight(name: string, ranges: Range[]) {
  const registry = highlightRegistry();
  const HighlightClass = (globalThis as typeof globalThis & { Highlight?: HighlightConstructor })
    .Highlight;
  if (!registry || !HighlightClass) return;
  if (ranges.length === 0) registry.delete(name);
  else registry.set(name, new HighlightClass(...ranges));
}

export function useMessageCommentDecorations(
  rootRef: RefObject<HTMLDivElement | null>,
  comments: AgentMessageComment[],
  messageContent: string,
  highlightName: string,
): MessageCommentDecoration[] {
  const [decorations, setDecorations] = useState<MessageCommentDecoration[]>([]);

  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    if (comments.length === 0) {
      highlightRegistry()?.delete(highlightName);
      setDecorations((current) => (current.length === 0 ? current : []));
      return;
    }
    let disposed = false;
    const refresh = () => {
      if (disposed) return;
      const next = getMessageCommentDecorations(root, comments);
      updateCustomHighlight(
        highlightName,
        next.map((decoration) => decoration.range),
      );
      setDecorations(next);
    };
    refresh();
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(refresh);
    observer?.observe(root);
    return () => {
      disposed = true;
      observer?.disconnect();
      highlightRegistry()?.delete(highlightName);
    };
  }, [comments, highlightName, messageContent, rootRef]);

  return decorations;
}

export function messageCommentDecorationAtPoint(
  decorations: MessageCommentDecoration[],
  x: number,
  y: number,
) {
  return decorations.find((decoration) =>
    Array.from(decoration.range.getClientRects()).some(
      (rect) => x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom,
    ),
  );
}
