"use client";

/**
 * "Depends on" selector for the task-create dialog.
 *
 * Dependencies are declared when the task is created (or later through MCP), so
 * this is the only place the UI writes an edge. Selecting one or more
 * predecessors also changes what "Start agent" means: the backend records the
 * start as a start-when-unblocked intent, and the task launches on its own once
 * every predecessor completes successfully.
 */

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLock, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useAppStore } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

/**
 * Stable empty fallbacks. A selector must not build a fresh array/object: under
 * Object.is that reads as a change on every store read and re-renders until React
 * bails out (which is what silently blanked the dependency chip once already).
 */
const NO_TASKS: KanbanState["tasks"] = [];
const NO_SNAPSHOTS: Record<string, { tasks?: KanbanState["tasks"] }> = {};

export type TaskCreateDependenciesProps = {
  /** Selected predecessor task IDs. */
  value: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
};

/**
 * Every task the user could depend on, from BOTH board slices.
 *
 * The home route hydrates its swimlanes into kanbanMulti.snapshots and can leave
 * kanban.tasks empty, so reading only the latter left this picker with nothing to
 * offer on the very route where tasks are created. Both selections are stable
 * references; the dedupe happens in the memo.
 */
function useBoardTasks(): KanbanState["tasks"] {
  const boardTasks = useAppStore((state) => state.kanban?.tasks ?? NO_TASKS);
  const snapshots = useAppStore((state) => state.kanbanMulti?.snapshots ?? NO_SNAPSHOTS);
  return useMemo(() => {
    const byId = new Map<string, KanbanState["tasks"][number]>();
    for (const task of boardTasks) byId.set(task.id, task);
    for (const snapshot of Object.values(snapshots)) {
      for (const task of snapshot.tasks ?? []) {
        if (!byId.has(task.id)) byId.set(task.id, task);
      }
    }
    return [...byId.values()];
  }, [boardTasks, snapshots]);
}

export function TaskCreateDependencies({ value, onChange, disabled }: TaskCreateDependenciesProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  // Select the stable slice reference and derive below: a selector that builds a
  // new array compares unequal on every store read and re-renders until React
  // gives up.
  // Candidates come from BOTH board slices. The home route hydrates its
  // swimlanes into kanbanMulti.snapshots and can leave kanban.tasks empty, so
  // reading only the latter left this picker with nothing to offer on the very
  // route where tasks are created. Both selections are stable references; the
  // dedupe happens in the memo below.
  const tasks = useBoardTasks();

  const selected = useMemo(
    () => value.map((id) => tasks.find((task) => task.id === id)).filter(Boolean),
    [value, tasks],
  );
  const candidates = useMemo(
    () =>
      tasks
        .filter((task) => !value.includes(task.id) && !task.isArchived)
        .sort((a, b) => a.title.localeCompare(b.title)),
    [tasks, value],
  );

  return (
    <div className="flex flex-col gap-1.5" data-testid="task-create-dependencies">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">{t("task:dependsOn")}</span>
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-xs"
              disabled={disabled}
              data-testid="task-create-dependencies-trigger"
            >
              <IconLock className="h-3.5 w-3.5" />
              {t("task:addDependency")}
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-72 p-0">
            <Command>
              <CommandInput placeholder={t("task:dependsOn")} />
              <CommandList>
                <CommandEmpty>{t("task:noDependencies")}</CommandEmpty>
                <CommandGroup>
                  {candidates.map((task) => (
                    <CommandItem
                      key={task.id}
                      value={`${task.title} ${task.id}`}
                      onSelect={() => {
                        setOpen(false);
                        onChange([...value, task.id]);
                      }}
                    >
                      <span className="truncate">{task.title}</span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
      </div>
      {selected.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {selected.map((task) => (
            <Badge
              key={task!.id}
              variant="secondary"
              className="h-5 max-w-[16rem] gap-1 text-xs"
              data-testid="task-create-dependency-chip"
            >
              <span className="truncate">{task!.title}</span>
              <button
                type="button"
                aria-label={t("task:removeDependency")}
                className="cursor-pointer"
                onClick={() => onChange(value.filter((id) => id !== task!.id))}
              >
                <IconX className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}
      {selected.length > 0 && (
        <p className="text-[11px] text-muted-foreground">{t("task:dependsOnWillAutoStart")}</p>
      )}
    </div>
  );
}
