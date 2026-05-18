"use client";

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { NewTaskDialog } from "../components/new-task-dialog";
import { TasksToolbar } from "./tasks-toolbar";
import { TasksContent } from "./tasks-content";
import { useIssuesTree } from "./use-tasks-tree";
import { useServerSearch } from "./use-server-search";
import { usePaginatedTasks } from "./use-paginated-tasks";

const STORAGE_KEY_PREFIX = "kandev-tasks-filters-";
const SHOW_SYSTEM_STORAGE_KEY = "kandev-tasks-show-system";
const SHOW_SYSTEM_EVENT = "kandev:tasks-show-system";

function readShowSystemPref(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return localStorage.getItem(SHOW_SYSTEM_STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

// Subscribes to localStorage changes (cross-tab) plus a same-tab
// custom event so toggling in one component refreshes the snapshot
// for any other consumer mounted in the same tab.
function subscribeShowSystem(cb: () => void): () => void {
  const onStorage = (e: StorageEvent) => {
    if (e.key === SHOW_SYSTEM_STORAGE_KEY) cb();
  };
  const onCustom = () => cb();
  window.addEventListener("storage", onStorage);
  window.addEventListener(SHOW_SYSTEM_EVENT, onCustom);
  return () => {
    window.removeEventListener("storage", onStorage);
    window.removeEventListener(SHOW_SYSTEM_EVENT, onCustom);
  };
}

function useShowSystemPref(): [boolean, (next: boolean) => void] {
  const value = useSyncExternalStore(
    subscribeShowSystem,
    readShowSystemPref,
    () => false, // SSR snapshot — toggle defaults to off pre-hydration.
  );
  const set = useCallback((next: boolean) => {
    if (typeof window === "undefined") return;
    try {
      localStorage.setItem(SHOW_SYSTEM_STORAGE_KEY, next ? "true" : "false");
    } catch {
      // ignore storage errors
    }
    window.dispatchEvent(new Event(SHOW_SYSTEM_EVENT));
  }, []);
  return [value, set];
}

function loadPersistedFilters(workspaceId: string | null) {
  if (!workspaceId || typeof window === "undefined") return {};
  try {
    const raw = localStorage.getItem(`${STORAGE_KEY_PREFIX}${workspaceId}`);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function persistFilters(workspaceId: string | null, filters: Record<string, unknown>) {
  if (!workspaceId || typeof window === "undefined") return;
  try {
    localStorage.setItem(`${STORAGE_KEY_PREFIX}${workspaceId}`, JSON.stringify(filters));
  } catch {
    // ignore storage errors
  }
}

function useRehydratePersistedFilters(workspaceId: string | null) {
  const setTaskFilters = useAppStore((s) => s.setTaskFilters);
  useEffect(() => {
    const persisted = loadPersistedFilters(workspaceId);
    if (Object.keys(persisted).length > 0) setTaskFilters(persisted);
  }, [workspaceId, setTaskFilters]);
}

export function TasksList() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const tasks = useAppStore((s) => s.office.tasks.items);
  const filters = useAppStore((s) => s.office.tasks.filters);
  const viewMode = useAppStore((s) => s.office.tasks.viewMode);
  const sortField = useAppStore((s) => s.office.tasks.sortField);
  const sortDir = useAppStore((s) => s.office.tasks.sortDir);
  const groupBy = useAppStore((s) => s.office.tasks.groupBy);
  const nestingEnabled = useAppStore((s) => s.office.tasks.nestingEnabled);
  const isLoading = useAppStore((s) => s.office.tasks.isLoading);
  const agents = useAppStore((s) => s.office.agentProfiles);

  const setTaskFilters = useAppStore((s) => s.setTaskFilters);
  const setTaskViewMode = useAppStore((s) => s.setTaskViewMode);
  const setTaskSortField = useAppStore((s) => s.setTaskSortField);
  const setTaskSortDir = useAppStore((s) => s.setTaskSortDir);
  const setTaskGroupBy = useAppStore((s) => s.setTaskGroupBy);
  const toggleNesting = useAppStore((s) => s.toggleNesting);

  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [newTaskOpen, setNewTaskOpen] = useState(false);
  const [showSystem, setShowSystem] = useShowSystemPref();
  const { searchResults, triggerSearch } = useServerSearch(workspaceId);

  const agentMap = new Map(agents.map((a) => [a.id, a.name]));

  useRehydratePersistedFilters(workspaceId);
  const { loadMore, hasMore, isLoadingMore, refetch } = usePaginatedTasks(workspaceId, showSystem);
  // WS-driven invalidation: refetch the current filter/sort/page-1 on
  // task lifecycle events (task created, etc.) — moved from the page
  // client so the refetch preserves the user's active filters.
  useOfficeRefetch("tasks", refetch);

  const handleFilterChange = useCallback(
    (patch: Record<string, unknown>) => {
      setTaskFilters(patch);
      persistFilters(workspaceId, { ...filters, ...patch });
    },
    [setTaskFilters, filters, workspaceId],
  );

  const handleSearchChange = useCallback(
    (search: string) => {
      setTaskFilters({ search });
      triggerSearch(search);
    },
    [setTaskFilters, triggerSearch],
  );

  const handleToggleExpand = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const activeIssues = searchResults ?? tasks;
  // Skip local search filter when using server results to avoid
  // rejecting matches on description or identifier.
  const treeFilters = searchResults ? { ...filters, search: "" } : filters;

  const flatNodes = useIssuesTree({
    tasks: activeIssues,
    filters: treeFilters,
    sortField,
    sortDir,
    nestingEnabled,
    expandedIds,
  });

  return (
    <div className="space-y-4 p-6">
      <TasksToolbar
        viewMode={viewMode}
        nestingEnabled={nestingEnabled}
        filters={filters}
        sortField={sortField}
        sortDir={sortDir}
        groupBy={groupBy}
        showSystem={showSystem}
        onViewModeChange={setTaskViewMode}
        onToggleNesting={toggleNesting}
        onFilterChange={handleFilterChange}
        onSortFieldChange={setTaskSortField}
        onSortDirChange={setTaskSortDir}
        onGroupByChange={setTaskGroupBy}
        onSearchChange={handleSearchChange}
        onShowSystemChange={setShowSystem}
        onNewIssue={() => setNewTaskOpen(true)}
      />

      <TasksContent
        viewMode={viewMode}
        isLoading={isLoading}
        flatNodes={flatNodes}
        expandedIds={expandedIds}
        onToggleExpand={handleToggleExpand}
        agentMap={agentMap}
      />

      <LoadMoreButton
        visible={hasMore && !searchResults}
        loading={isLoadingMore}
        onClick={loadMore}
      />
      <NewTaskDialog open={newTaskOpen} onOpenChange={setNewTaskOpen} />
    </div>
  );
}

function LoadMoreButton({
  visible,
  loading,
  onClick,
}: {
  visible: boolean;
  loading: boolean;
  onClick: () => void;
}) {
  if (!visible) return null;
  return (
    <div className="flex justify-center pt-2">
      <Button
        variant="outline"
        size="sm"
        onClick={onClick}
        disabled={loading}
        className="cursor-pointer"
      >
        {loading ? "Loading…" : "Load more"}
      </Button>
    </div>
  );
}
