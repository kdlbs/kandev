"use client";

import { useEffect, useMemo, useRef } from "react";
import { listTasksByWorkspace } from "@/lib/api";
import type { CommandPanelMode } from "@/lib/commands/types";
import type { Task } from "@/lib/types/http";
import { MODE_COMMANDS, MODE_SEARCH_TASKS } from "@/components/command-panel-results";

const ARCHIVED_STATES = new Set(["COMPLETED", "CANCELLED", "FAILED"]);
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
  steps: { id: string; position: number; show_in_command_panel?: boolean }[];
  setTaskResults: (tasks: Task[]) => void;
  setIsSearching: (searching: boolean) => void;
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
  signal: AbortSignal;
  visibleStepIds: Set<string> | null;
  stepPositionMap: Map<string, number>;
  resultLimit: number;
};

/** Active tasks for the idle palette: no backlog or done steps, workflow order. */
async function fetchActiveTasks(query: ActiveTaskQuery): Promise<Task[]> {
  const { workspaceId, signal, visibleStepIds, stepPositionMap, resultLimit } = query;
  // Deliberately over-fetched: the step and archived filters below run on the
  // response, so requesting only `resultLimit` rows would show fewer active
  // tasks than exist whenever the first page is mostly backlog or done.
  const res = await listTasksByWorkspace(
    workspaceId,
    { page: 1, pageSize: TASK_SCOPE_PAGE_SIZE },
    { init: { signal } },
  );
  const tasks = (res.tasks ?? []).filter(
    (t) =>
      (!visibleStepIds || visibleStepIds.has(t.workflow_step_id)) && !ARCHIVED_STATES.has(t.state),
  );
  tasks.sort(
    (a, b) =>
      (stepPositionMap.get(a.workflow_step_id) ?? 99) -
      (stepPositionMap.get(b.workflow_step_id) ?? 99),
  );
  return tasks.slice(0, resultLimit);
}

/** Task search results, archived matches last. */
async function fetchMatchingTasks(
  workspaceId: string,
  query: string,
  resultLimit: number,
  signal: AbortSignal,
): Promise<Task[]> {
  const res = await listTasksByWorkspace(
    workspaceId,
    { query, page: 1, pageSize: resultLimit, includeArchived: true },
    { init: { signal } },
  );
  const tasks = res.tasks ?? [];
  tasks.sort(
    (a, b) => (ARCHIVED_STATES.has(a.state) ? 1 : 0) - (ARCHIVED_STATES.has(b.state) ? 1 : 0),
  );
  return tasks;
}

export function useInlineTaskSearchEffect(opts: InlineTaskSearchOptions) {
  const { mode, search, open, workspaceId, steps, setTaskResults, setIsSearching } = opts;
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

    // No search: load active tasks (excluding backlog + done steps)
    if (!search.trim()) {
      if (!open || !workspaceId) {
        setTaskResults([]);
        setIsSearching(false);
        return;
      }
      setIsSearching(true);
      const controller = new AbortController();
      abortRef.current = controller;
      fetchActiveTasks({
        workspaceId,
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
      setTaskResults([]);
      setIsSearching(false);
      return;
    }

    // Search: query API including archived
    setIsSearching(true);
    debounceRef.current = setTimeout(async () => {
      if (!workspaceId) {
        setIsSearching(false);
        return;
      }
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const tasks = await fetchMatchingTasks(
          workspaceId,
          search.trim(),
          resultLimit,
          controller.signal,
        );
        if (!controller.signal.aborted) setTaskResults(tasks);
      } catch {
        if (!controller.signal.aborted) setTaskResults([]);
      } finally {
        if (!controller.signal.aborted) setIsSearching(false);
      }
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
    visibleStepIds,
    stepPositionMap,
    setTaskResults,
    setIsSearching,
  ]);
}
