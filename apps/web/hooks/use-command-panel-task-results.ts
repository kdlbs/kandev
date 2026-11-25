"use client";

import { useEffect, useMemo, useRef } from "react";
import { listTasksByWorkspace, searchTaskConversations } from "@/lib/api";
import type { CommandPanelMode } from "@/lib/commands/types";
import type {
  ConversationTask as Task,
  ConversationSearchState,
} from "@/lib/types/conversation-search";
import { MODE_COMMANDS, MODE_SEARCH_TASKS } from "@/components/command-panel-results";

/** Task rows previewed alongside the commands in the commands scope. */
const TASK_PREVIEW_SIZE = 5;
/** Task rows listed in the dedicated tasks scope. */
const TASK_SCOPE_PAGE_SIZE = 20;

function resolveVisibleStepIds(steps: { id: string; show_in_command_panel?: boolean }[]) {
  if (steps.length === 0) return null; // no steps loaded yet — don't filter
  return new Set(steps.filter((s) => s.show_in_command_panel !== false).map((s) => s.id));
}

type InlineTaskSearchOptions = {
  mode: CommandPanelMode;
  search: string;
  open: boolean;
  workspaceId: string | null;
  workflowId?: string | null;
  steps: { id: string; position: number; show_in_command_panel?: boolean }[];
  setTaskResults: (tasks: Task[]) => void;
  setIsSearching: (searching: boolean) => void;
  setConversationState?: (state: ConversationSearchState) => void;
};

function useStepMaps(steps: InlineTaskSearchOptions["steps"]) {
  const visibleStepIds = useMemo(() => resolveVisibleStepIds(steps), [steps]);
  const stepPositionMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const step of steps) map.set(step.id, step.position);
    return map;
  }, [steps]);
  return { visibleStepIds, stepPositionMap };
}

type ActiveTaskQuery = {
  workspaceId: string;
  workflowId: string | null | undefined;
  signal: AbortSignal;
  visibleStepIds: Set<string> | null;
  stepPositionMap: Map<string, number>;
  resultLimit: number;
};

/** Unarchived task preview for the idle palette, ordered by workflow step. */
async function fetchActiveTasks(query: ActiveTaskQuery): Promise<Task[]> {
  const { workspaceId, workflowId, signal, visibleStepIds, stepPositionMap, resultLimit } = query;
  const filterByStep = visibleStepIds !== null && Boolean(workflowId);
  const res = await listTasksByWorkspace(
    workspaceId,
    {
      page: 1,
      pageSize: TASK_SCOPE_PAGE_SIZE,
      ...(filterByStep ? { workflowId, commandPanelVisible: true } : {}),
    },
    { init: { signal } },
  );
  const tasks = (res.tasks ?? []).filter(
    (t) => (!visibleStepIds || visibleStepIds.has(t.workflow_step_id)) && t.archived_at == null,
  );
  tasks.sort(
    (a, b) =>
      (stepPositionMap.get(a.workflow_step_id) ?? 99) -
      (stepPositionMap.get(b.workflow_step_id) ?? 99),
  );
  return tasks.slice(0, resultLimit);
}

/** Task search results, with archived matches fetched only as a fallback. */
async function fetchMatchingTasks(
  workspaceId: string,
  query: string,
  resultLimit: number,
  signal: AbortSignal,
): Promise<Task[]> {
  const activeResponse = await listTasksByWorkspace(
    workspaceId,
    { query, page: 1, pageSize: resultLimit },
    { init: { signal } },
  );
  const activeTasks = (activeResponse.tasks ?? []).filter((task) => task.archived_at == null);
  if (activeTasks.length >= resultLimit) return activeTasks.slice(0, resultLimit);

  const archivedResponse = await listTasksByWorkspace(
    workspaceId,
    {
      query,
      page: 1,
      pageSize: resultLimit - activeTasks.length,
      onlyArchived: true,
    },
    { init: { signal } },
  );
  const archivedTasks = (archivedResponse.tasks ?? []).filter((task) => task.archived_at != null);
  return [...activeTasks, ...archivedTasks].slice(0, resultLimit);
}

export function useInlineTaskSearchEffect(opts: InlineTaskSearchOptions) {
  const { mode, search, open, workspaceId, workflowId, steps, setTaskResults, setIsSearching } =
    opts;
  const { setConversationState } = opts;
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const taskScopeRef = useRef(mode);
  const { visibleStepIds, stepPositionMap } = useStepMaps(steps);

  useEffect(() => {
    if (mode !== MODE_COMMANDS && mode !== MODE_SEARCH_TASKS) return;
    // The commands scope only previews tasks alongside the commands; the tasks
    // scope is the one that lists the full result set.
    const resultLimit = mode === MODE_SEARCH_TASKS ? TASK_SCOPE_PAGE_SIZE : TASK_PREVIEW_SIZE;
    // The two scopes size their result sets differently, so carrying one over
    // would present a five-row preview as the tasks scope's full result set
    // (and suppress its loading state) until the wider request lands.
    if (taskScopeRef.current !== mode) {
      taskScopeRef.current = mode;
      setTaskResults([]);
    }
    if (debounceRef.current) clearTimeout(debounceRef.current);
    abortRef.current?.abort();

    setConversationState?.("ready");
    if (!open || !workspaceId) {
      setTaskResults([]);
      setIsSearching(false);
      return;
    }

    // No search: load unarchived tasks in the visible workflow steps.
    if (!search.trim()) {
      setIsSearching(true);
      const controller = new AbortController();
      abortRef.current = controller;
      fetchActiveTasks({
        workspaceId,
        workflowId,
        signal: controller.signal,
        visibleStepIds,
        stepPositionMap,
        resultLimit,
      })
        .then((tasks) => {
          if (!controller.signal.aborted) setTaskResults(tasks);
        })
        .catch(() => {
          if (!controller.signal.aborted) setTaskResults([]);
        })
        .finally(() => {
          if (!controller.signal.aborted) setIsSearching(false);
        });
      return () => {
        controller.abort();
      };
    }

    // Search with < 2 chars: clear results
    if (search.trim().length < 2) {
      setConversationState?.("query_too_short");
      setTaskResults([]);
      setIsSearching(false);
      return;
    }

    setIsSearching(true);
    setTaskResults([]);
    const controller = new AbortController();
    abortRef.current = controller;
    debounceRef.current = setTimeout(() => {
      searchTaskSources(
        { workspaceId, search, setTaskResults, setIsSearching, setConversationState },
        resultLimit,
        controller.signal,
      );
    }, 300);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      abortRef.current?.abort();
    };
  }, [
    mode,
    search,
    open,
    workspaceId,
    workflowId,
    visibleStepIds,
    stepPositionMap,
    setTaskResults,
    setIsSearching,
    setConversationState,
  ]);
}

export function mergeTaskSearchResults(
  metadata: Task[],
  conversations: Task[],
  limit: number,
): Task[] {
  const ordered = [
    ...metadata.filter((task) => !task.archived_at),
    ...conversations.filter((task) => !task.archived_at),
    ...metadata.filter((task) => task.archived_at),
    ...conversations.filter((task) => task.archived_at),
  ];
  const seen = new Set<string>();
  return ordered
    .filter((task) => {
      if (seen.has(task.id)) return false;
      seen.add(task.id);
      return true;
    })
    .slice(0, limit);
}

function searchTaskSources(
  opts: Pick<
    InlineTaskSearchOptions,
    "search" | "setTaskResults" | "setIsSearching" | "setConversationState"
  > & { workspaceId: string },
  resultLimit: number,
  signal: AbortSignal,
) {
  const { workspaceId, search, setTaskResults, setIsSearching, setConversationState } = opts;
  let pending = 2;
  const settle = () => {
    pending -= 1;
    if (!signal.aborted && pending === 0) setIsSearching(false);
  };
  let metadata: Task[] = [];
  let conversations: Task[] = [];
  const publish = () => {
    if (!signal.aborted)
      setTaskResults(mergeTaskSearchResults(metadata, conversations, resultLimit));
  };
  void fetchMatchingTasks(workspaceId, search.trim(), resultLimit, signal)
    .then((tasks) => {
      metadata = tasks;
      publish();
    })
    .catch(() => {
      publish();
    })
    .finally(settle);
  setConversationState?.("searching");
  void searchTaskConversations(workspaceId, search.trim(), resultLimit, {
    init: { signal: signal },
  })
    .then((response) => {
      if (signal.aborted) return;
      conversations = response.tasks;
      setConversationState?.(response.has_more ? "limited" : response.state);
      publish();
    })
    .catch(() => {
      if (!signal.aborted) setConversationState?.("unavailable");
    })
    .finally(settle);
}
