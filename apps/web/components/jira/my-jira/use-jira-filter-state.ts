"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { DEFAULT_FILTERS, filtersToJql, type FilterState } from "./filter-model";
import { resolveInitialJiraView, initialFilters } from "./jira-default-view";
import { DEFAULT_VIEW, useSavedViews } from "./use-saved-views";

type InitialViewSetters = {
  setFilters: (filters: FilterState) => void;
  setActiveViewId: (viewId: string | null) => void;
  setCustomJql: (jql: string | null) => void;
  setShowJqlEditor: (show: boolean) => void;
};

function useInitialDefaultSelection(
  defaultProjectKey: string,
  savedViews: ReturnType<typeof useSavedViews>,
  manualInteraction: { current: boolean },
  setters: InitialViewSetters,
) {
  const [resolved, setResolved] = useState(false);

  useEffect(() => {
    if (!savedViews.ready || resolved) return;
    if (!manualInteraction.current) {
      const initial = resolveInitialJiraView(
        savedViews.defaultViewId,
        savedViews.views,
        defaultProjectKey,
      );
      setters.setFilters(initial.filters);
      setters.setActiveViewId(initial.activeViewId);
      setters.setCustomJql(initial.customJql);
      setters.setShowJqlEditor(initial.showJqlEditor);
    }
    setResolved(true);
  }, [defaultProjectKey, manualInteraction, resolved, savedViews, setters]);

  return resolved;
}

export function useJiraFilterState(defaultProjectKey: string) {
  const savedViews = useSavedViews();
  const [filters, setFilters] = useState<FilterState>(() => initialFilters(defaultProjectKey));
  const [activeViewId, setActiveViewId] = useState<string | null>(DEFAULT_VIEW.id);
  const [customJql, setCustomJql] = useState<string | null>(null);
  const [showJqlEditor, setShowJqlEditor] = useState(false);
  const manualInteraction = useRef(false);
  const selectionRevision = useRef(0);
  const initialSelectionResolved = useInitialDefaultSelection(
    defaultProjectKey,
    savedViews,
    manualInteraction,
    { setFilters, setActiveViewId, setCustomJql, setShowJqlEditor },
  );

  const composedJql = useMemo(() => filtersToJql(filters), [filters]);
  const effectiveJql = customJql ?? composedJql;
  const setDefaultView = savedViews.setDefaultView;

  const updateFilters = useCallback((next: FilterState) => {
    manualInteraction.current = true;
    selectionRevision.current += 1;
    setFilters(next);
    setActiveViewId(null);
    setCustomJql(null);
  }, []);

  const selectView = useCallback(
    (id: string) => {
      const view = savedViews.views.find((candidate) => candidate.id === id);
      if (!view) return;
      manualInteraction.current = true;
      selectionRevision.current += 1;
      setFilters(view.filters);
      setActiveViewId(id);
      const savedCustomJql = view.customJql ?? null;
      setCustomJql(savedCustomJql);
      if (savedCustomJql !== null) setShowJqlEditor(true);
    },
    [savedViews.views],
  );

  const saveCurrentAsView = useCallback(
    async (name: string) => {
      const revision = selectionRevision.current;
      manualInteraction.current = true;
      const view = await savedViews.save(name, filters, customJql);
      if (revision === selectionRevision.current) setActiveViewId(view.id);
      return view;
    },
    [customJql, filters, savedViews],
  );

  const deleteView = useCallback(
    async (id: string) => {
      const revision = selectionRevision.current;
      const deletesActiveDefault = activeViewId === id && savedViews.defaultViewId === id;
      manualInteraction.current = true;
      const removed = await savedViews.remove(id);
      if (removed && deletesActiveDefault && revision === selectionRevision.current) {
        const fallback = resolveInitialJiraView("", [], defaultProjectKey);
        setFilters(fallback.filters);
        setActiveViewId(fallback.activeViewId);
        setCustomJql(fallback.customJql);
        setShowJqlEditor(fallback.showJqlEditor);
      }
      return removed;
    },
    [activeViewId, defaultProjectKey, savedViews.defaultViewId, savedViews.remove],
  );

  const applyCustomJql = useCallback((value: string | null) => {
    manualInteraction.current = true;
    selectionRevision.current += 1;
    setCustomJql(value);
  }, []);

  const resetCustomJql = useCallback(() => {
    manualInteraction.current = true;
    selectionRevision.current += 1;
    setCustomJql(null);
  }, []);

  return {
    filters,
    updateFilters,
    views: savedViews.views,
    activeViewId,
    selectView,
    deleteView,
    saveCurrentAsView,
    defaultViewId: savedViews.defaultViewId,
    viewsReady: savedViews.ready,
    defaultMutationPending: savedViews.defaultMutationPending,
    viewMutationPending: savedViews.viewMutationPending,
    setDefaultView,
    initialSelectionResolved,
    composedJql,
    customJql,
    effectiveJql,
    applyCustomJql,
    resetCustomJql,
    showJqlEditor,
    setShowJqlEditor,
  };
}

export { DEFAULT_FILTERS };
