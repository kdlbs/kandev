"use client";

import { useCallback, useEffect, useState } from "react";
import type { Editor } from "@tiptap/core";
import { usePanelSearch } from "@/hooks/use-panel-search";
import { planSearchPluginKey } from "@/components/editors/tiptap/search-highlight-extension";

/** Ctrl+F find-in-plan shortcut + editor wiring. Returns the search-bar state
 * the panel renders into `<PanelSearchBar>`. */
export function usePlanFindShortcut(
  wrapperRef: React.RefObject<HTMLDivElement | null>,
  editor: Editor | null,
) {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [matchInfo, setMatchInfo] = useState({ current: 0, total: 0 });

  const readMatchInfo = useCallback(() => {
    if (!editor) return;
    const s = planSearchPluginKey.getState(editor.state);
    if (!s) return;
    const next = {
      current: s.matches.length ? s.current + 1 : 0,
      total: s.matches.length,
    };
    // Bail out when nothing changed — TipTap fires `transaction` on every cursor
    // move, and allocating a new object identity would re-render this hook on
    // every keystroke / click in the editor (and can cascade into update loops).
    setMatchInfo((prev) =>
      prev.current === next.current && prev.total === next.total ? prev : next,
    );
  }, [editor]);

  useEffect(() => {
    if (!editor) return;
    const handler = () => readMatchInfo();
    editor.on("transaction", handler);
    return () => {
      editor.off("transaction", handler);
    };
  }, [editor, readMatchInfo]);

  useEffect(() => {
    if (!editor) return;
    if (!isOpen) return;
    editor.commands.setPlanSearchQuery(query);
  }, [query, isOpen, editor]);

  useEffect(() => {
    if (isOpen) return;
    if (!editor) return;
    editor.commands.clearPlanSearch();
  }, [isOpen, editor]);

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => setIsOpen(false), []);
  const findNext = useCallback(() => {
    editor?.commands.planSearchNext();
  }, [editor]);
  const findPrev = useCallback(() => {
    editor?.commands.planSearchPrev();
  }, [editor]);

  usePanelSearch({ containerRef: wrapperRef, isOpen, onOpen: open, onClose: close });

  return { isOpen, open, close, query, setQuery, findNext, findPrev, matchInfo };
}
