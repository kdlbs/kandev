import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ChangeEvent, SetStateAction } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import type { FileTreeCacheBinding } from "./file-browser-tree-cache";

type SearchState = { active: boolean; query: string; results: string[] | null; searching: boolean };
const emptySearch = (): SearchState => ({
  active: false,
  query: "",
  results: null,
  searching: false,
});

export function useFileBrowserSearch(
  sessionId: string,
  binding?: FileTreeCacheBinding,
  resetKey?: string,
) {
  const owner = useMemo(
    () => ({
      generation: 0,
      timer: null as ReturnType<typeof setTimeout> | null,
    }),
    [sessionId, binding, resetKey],
  );
  const ownerRef = useRef(owner);
  ownerRef.current = owner;
  const [snapshot, setSnapshot] = useState(() => ({ owner, ...emptySearch() }));
  const state = snapshot.owner === owner ? snapshot : emptySearch();
  const stateRef = useRef(state);
  stateRef.current = state;
  const searchInputRef = useRef<HTMLInputElement>(null);
  const invalidate = useCallback(() => {
    owner.generation += 1;
    if (owner.timer) clearTimeout(owner.timer);
    owner.timer = null;
  }, [owner]);
  const publish = useCallback(
    (patch: Partial<SearchState>, generation?: number) => {
      const isCurrent = () =>
        ownerRef.current === owner &&
        (!binding || binding.isCurrent()) &&
        (generation === undefined || owner.generation === generation);
      if (!isCurrent()) return;
      setSnapshot((previous) => {
        if (!isCurrent()) return previous;
        return { ...(previous.owner === owner ? previous : emptySearch()), ...patch, owner };
      });
    },
    [owner, binding],
  );

  useEffect(() => () => invalidate(), [invalidate]);
  useEffect(() => {
    if (state.active) searchInputRef.current?.focus();
  }, [state.active]);

  const handleCloseSearch = useCallback(() => {
    invalidate();
    publish(emptySearch());
  }, [invalidate, publish]);
  const setIsSearchActive = useCallback(
    (update: SetStateAction<boolean>) => {
      const active = typeof update === "function" ? update(stateRef.current.active) : update;
      if (!active) handleCloseSearch();
      else publish({ active });
    },
    [handleCloseSearch, publish],
  );
  const handleSearchChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.target.value;
      invalidate();
      const generation = owner.generation;
      publish({
        query: value,
        searching: Boolean(value.trim()),
        ...(value.trim() ? {} : { results: null }),
      });
      if (!value.trim()) return;
      const isCurrent = () =>
        ownerRef.current === owner &&
        owner.generation === generation &&
        (!binding || binding.isCurrent());
      owner.timer = setTimeout(async () => {
        if (!isCurrent()) return;
        owner.timer = null;
        try {
          const client = getWebSocketClient();
          if (!client) return;
          const response = await searchWorkspaceFiles(client, sessionId, value, 50);
          publish({ results: response.files || [] }, generation);
        } catch (error) {
          if (!isCurrent()) return;
          console.error("Failed to search files:", error);
          publish({ results: [] }, generation);
        } finally {
          publish({ searching: false }, generation);
        }
      }, 300);
    },
    [invalidate, owner, publish, binding, sessionId],
  );

  return {
    isSearchActive: state.active,
    setIsSearchActive,
    localSearchQuery: state.query,
    searchResults: state.results,
    isSearching: state.searching,
    searchInputRef,
    handleSearchChange,
    handleCloseSearch,
  };
}
