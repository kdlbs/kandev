"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useEntityReferenceSearch } from "@/hooks/use-entity-reference-search";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  createEntityReferenceSuggestion,
  handleEntityReferenceMenuKeyDown,
} from "./tiptap-entity-reference-suggestion";
import type { MenuState } from "./tiptap-suggestion";
import { selectableEntityReferences } from "./entity-reference-groups";

const EMPTY_REFERENCE_MENU: MenuState<EntityReference> = {
  isOpen: false,
  items: [],
  query: "",
  clientRect: null,
  command: null,
};

type UseEntityReferenceComposerOptions = {
  enabled: boolean;
  workspaceId: string | null;
  sessionId: string | null;
};

export function useEntityReferenceComposer({
  enabled,
  workspaceId,
  sessionId,
}: UseEntityReferenceComposerOptions) {
  const [menu, setMenu] = useState<MenuState<EntityReference>>(EMPTY_REFERENCE_MENU);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const searchEnabled =
    enabled && Boolean(workspaceId) && menu.isOpen && Boolean(menu.query.trim());
  const search = useEntityReferenceSearch({
    workspaceId,
    sessionId,
    query: menu.query,
    enabled: searchEnabled,
  });
  const items = useMemo(() => selectableEntityReferences(search.groups), [search.groups]);
  const clampedSelectedIndex = Math.min(selectedIndex, Math.max(0, items.length - 1));
  const itemsRef = useRef(items);
  const selectedIndexRef = useRef(clampedSelectedIndex);
  const commandRef = useRef(menu.command);

  useLayoutEffect(() => {
    itemsRef.current = items;
    selectedIndexRef.current = clampedSelectedIndex;
    commandRef.current = menu.command;
  });

  useEffect(() => {
    void Promise.resolve().then(() => setSelectedIndex(0));
  }, [items]);

  useEffect(() => {
    void Promise.resolve().then(() => setMenu(EMPTY_REFERENCE_MENU));
  }, [enabled, sessionId, workspaceId]);

  const selectReference = useCallback((reference: EntityReference) => {
    commandRef.current?.(reference);
    setMenu(EMPTY_REFERENCE_MENU);
  }, []);
  const onKeyDown = useCallback(
    (event: KeyboardEvent) =>
      handleEntityReferenceMenuKeyDown({
        event,
        items: itemsRef.current,
        selectedIndex: selectedIndexRef.current,
        setSelectedIndex,
        onSelect: selectReference,
      }),
    [selectReference],
  );
  const suggestion = useMemo(
    () => createEntityReferenceSuggestion(setMenu, onKeyDown),
    [onKeyDown],
  );
  const close = useCallback(() => setMenu(EMPTY_REFERENCE_MENU), []);
  const hasWorkspace = Boolean(workspaceId);

  return {
    suggestion: enabled ? suggestion : undefined,
    isOpen: enabled && hasWorkspace && menu.isOpen,
    clientRect: menu.clientRect,
    query: menu.query,
    groups: search.groups,
    isSearching: search.isSearching,
    error: search.error,
    retry: search.retry,
    selectedIndex: clampedSelectedIndex,
    setSelectedIndex,
    selectReference,
    close,
  };
}
