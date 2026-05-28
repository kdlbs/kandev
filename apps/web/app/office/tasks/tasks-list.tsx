"use client";

import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useState,
  useSyncExternalStore,
} from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import type {
  TaskFilterState,
  TaskSortField,
  TaskSortDir,
  TaskGroupBy,
  OfficeTask,
} from "@/lib/state/slices/office/types";
import { NewTaskDialog } from "../components/new-task-dialog";
import { TasksToolbar } from "./tasks-toolbar";
import { TasksContent } from "./tasks-content";
import { useIssuesTree } from "./use-tasks-tree";
import { useServerSearch } from "./use-server-search";
import { usePaginatedTasks } from "./use-paginated-tasks";

const STORAGE_KEY_PREFIX = "kandev-tasks-filters-";
const SHOW_SYSTEM_STORAGE_KEY = "kandev-tasks-show-system";
const SHOW_SYSTEM_EVENT = "kandev:tasks-show-system";

const DEFAULT_FILTERS: TaskFilterState = {
  statuses: [],
  priorities: [],
  assigneeIds: [],
  projectIds: [],
  search: "",
};

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

function useTasksListState(workspaceId: string | null) {
  // UI state lives in local React state (not Zustand) per TQ migration plan.
  // Lazily seed filters from localStorage so we don't need a setState-in-effect.
  const [filters, setFilters] = useState<TaskFilterState>(() => {
    const persisted = loadPersistedFilters(workspaceId);
    return Object.keys(persisted).length > 0
      ? { ...DEFAULT_FILTERS, ...persisted }
      : DEFAULT_FILTERS;
  });
  const [viewMode, setViewMode] = useState<"list" | "board">("list");
  const [sortField, setSortField] = useState<TaskSortField>("updated");
  const [sortDir, setSortDir] = useState<TaskSortDir>("desc");
  const [groupBy, setGroupBy] = useState<TaskGroupBy>("none");
  const [nestingEnabled, setNestingEnabled] = useState(true);
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [newTaskOpen, setNewTaskOpen] = useState(false);
  return {
    filters,
    setFilters,
    viewMode,
    setViewMode,
    sortField,
    setSortField,
    sortDir,
    setSortDir,
    groupBy,
    setGroupBy,
    nestingEnabled,
    setNestingEnabled,
    expandedIds,
    setExpandedIds,
    newTaskOpen,
    setNewTaskOpen,
  };
}

type TasksHandlersOptions = {
  workspaceId: string | null;
  tasks: OfficeTask[];
  filters: TaskFilterState;
  setFilters: Dispatch<SetStateAction<TaskFilterState>>;
  sortField: TaskSortField;
  sortDir: TaskSortDir;
  nestingEnabled: boolean;
  expandedIds: Set<string>;
  setExpandedIds: Dispatch<SetStateAction<Set<string>>>;
  triggerSearch: (search: string) => void;
  searchResults: OfficeTask[] | null;
};

type TasksHandlers = {
  handleFilterChange: (patch: Record<string, unknown>) => void;
  handleSearchChange: (search: string) => void;
  handleToggleExpand: (id: string) => void;
  flatNodes: ReturnType<typeof useIssuesTree>;
};

function useTasksHandlers(opts: TasksHandlersOptions): TasksHandlers {
  const {
    workspaceId,
    tasks,
    filters,
    setFilters,
    sortField,
    sortDir,
    nestingEnabled,
    expandedIds,
    setExpandedIds,
    triggerSearch,
    searchResults,
  } = opts;
  const handleFilterChange = useCallback(
    (patch: Record<string, unknown>) => {
      setFilters((prev) => {
        const next = { ...prev, ...patch } as TaskFilterState;
        persistFilters(workspaceId, next);
        return next;
      });
    },
    [workspaceId, setFilters],
  );

  const handleSearchChange = useCallback(
    (search: string) => {
      setFilters((prev) => ({ ...prev, search }));
      triggerSearch(search);
    },
    [triggerSearch, setFilters],
  );

  const handleToggleExpand = useCallback(
    (id: string) => {
      setExpandedIds((prev) => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      });
    },
    [setExpandedIds],
  );

  const treeFilters = searchResults ? { ...filters, search: "" } : filters;
  const flatNodes = useIssuesTree({
    tasks: searchResults ?? tasks,
    filters: treeFilters,
    sortField,
    sortDir,
    nestingEnabled,
    expandedIds,
  });

  return { handleFilterChange, handleSearchChange, handleToggleExpand, flatNodes };
}

export function TasksList() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const tasks = useAppStore((s) => s.office.tasks.items);
  const isLoading = useAppStore((s) => s.office.tasks.isLoading);

  const {
    filters,
    setFilters,
    viewMode,
    setViewMode,
    sortField,
    setSortField,
    sortDir,
    setSortDir,
    groupBy,
    setGroupBy,
    nestingEnabled,
    setNestingEnabled,
    expandedIds,
    setExpandedIds,
    newTaskOpen,
    setNewTaskOpen,
  } = useTasksListState(workspaceId);

  const { data: agents = [] } = useQuery({
    ...officeQueryOptions.agents(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  const [showSystem, setShowSystem] = useShowSystemPref();
  const { searchResults, triggerSearch } = useServerSearch(workspaceId);
  const agentMap = new Map(agents.map((a) => [a.id, a.name]));

  const { loadMore, hasMore, isLoadingMore, refetch } = usePaginatedTasks(
    workspaceId,
    showSystem,
    filters,
    sortField,
    sortDir,
  );
  // WS-driven invalidation: refetch the current filter/sort/page-1 on
  // task lifecycle events so the list stays current.
  useOfficeRefetch("tasks", refetch);

  const { handleFilterChange, handleSearchChange, handleToggleExpand, flatNodes } =
    useTasksHandlers({
      workspaceId,
      tasks,
      filters,
      setFilters,
      sortField,
      sortDir,
      nestingEnabled,
      expandedIds,
      setExpandedIds,
      triggerSearch,
      searchResults,
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
        onViewModeChange={setViewMode}
        onToggleNesting={() => setNestingEnabled((v) => !v)}
        onFilterChange={handleFilterChange}
        onSortFieldChange={setSortField}
        onSortDirChange={setSortDir}
        onGroupByChange={setGroupBy}
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
