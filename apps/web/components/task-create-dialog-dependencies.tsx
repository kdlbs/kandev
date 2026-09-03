"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconCheck,
  IconChevronDown,
  IconInfoCircle,
  IconListCheck,
  IconLock,
} from "@tabler/icons-react";
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useWorkspacePRs } from "@/hooks/domains/github/use-task-pr";
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskMR } from "@/lib/types/gitlab";
import type { TaskPR } from "@/lib/types/github";
import { cn } from "@/lib/utils";
import {
  changeRequestNumbers,
  compareDependencyCandidates,
  dependencyOptionValue,
} from "./task-create-dialog-dependency-search";

// Keep stable empty fallbacks here. Creating a fresh array/object in a store
// selector makes Object.is report a change on every read and can cause a
// render loop, which previously blanked the dependency chip.
const NO_TASKS: KanbanState["tasks"] = [];
const NO_SNAPSHOTS: Record<string, { tasks?: KanbanState["tasks"] }> = {};
const NO_MRS: TaskMR[] = [];
const NO_MRS_BY_TASK_ID: Record<string, TaskMR[]> = {};
const NO_PRS: TaskPR[] = [];
const NO_PRS_BY_TASK_ID: Record<string, TaskPR[]> = {};

export type TaskCreateDependenciesProps = {
  /** Selected predecessor task IDs. */
  value: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
};

function useBoardTasks(): KanbanState["tasks"] {
  const boardTasks = useAppStore((state) => state.kanban?.tasks ?? NO_TASKS);
  const snapshots = useAppStore((state) => state.kanbanMulti?.snapshots ?? NO_SNAPSHOTS);

  // The home route hydrates its swimlanes into kanbanMulti.snapshots and can
  // leave kanban.tasks empty, so both slices must supply picker candidates.
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

function useMRsByTaskId(): Record<string, TaskMR[]> {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const byWorkspaceId = useAppStore((state) => state.taskMRs.byWorkspaceId);
  return (activeWorkspaceId && byWorkspaceId[activeWorkspaceId]) || NO_MRS_BY_TASK_ID;
}

function usePRsByTaskId(): Record<string, TaskPR[]> {
  return useAppStore((state) => {
    const activeWorkspaceId = state.workspaces.activeId;
    if (
      !activeWorkspaceId ||
      state.taskPRs.workspaceId !== activeWorkspaceId ||
      state.taskPRs.workspaceContextGeneration !== state.workspaceContextGeneration
    ) {
      return NO_PRS_BY_TASK_ID;
    }
    return state.taskPRs.byTaskId;
  });
}

function DependencyTriggerLabel({
  value,
  selectedTitle,
  selectedNumber,
  t,
}: {
  value: string[];
  selectedTitle?: string;
  selectedNumber?: number;
  t: (key: string, options?: { count?: number; number?: number }) => string;
}) {
  if (value.length === 0) return t("task:noDependency");
  if (value.length === 1 && selectedTitle) {
    if (selectedNumber === undefined) return selectedTitle;
    return `${t("task:dependencyChangeRequestLabel", { number: selectedNumber })} · ${selectedTitle}`;
  }
  return t("task:dependencyCount", { count: value.length });
}

function DependencyOption({
  task,
  numbers,
  selected,
  onSelect,
}: {
  task: KanbanState["tasks"][number];
  numbers: number[];
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  return (
    <CommandItem
      value={dependencyOptionValue(task, numbers)}
      onSelect={onSelect}
      className="min-h-12 cursor-pointer gap-2 md:min-h-11"
      data-testid={`task-create-dependency-option-${task.id}`}
      aria-selected={selected}
    >
      <IconListCheck
        className="h-4 w-4 shrink-0 text-muted-foreground"
        data-testid="task-create-dependency-task-icon"
        aria-hidden="true"
      />
      <span className="min-w-0 flex-1 line-clamp-2">{task.title}</span>
      {numbers.map((number) => (
        <span
          key={number}
          className="shrink-0 text-xs text-muted-foreground"
          aria-label={t("task:dependencyChangeRequestAria", { number })}
        >
          {t("task:dependencyChangeRequestLabel", { number })}
        </span>
      ))}
      <IconCheck
        className={cn("h-4 w-4 shrink-0", selected ? "opacity-100" : "opacity-0")}
        aria-hidden="true"
      />
    </CommandItem>
  );
}

type DependencyPickerContentProps = {
  candidates: KanbanState["tasks"];
  mrsByTaskId: Record<string, TaskMR[]>;
  prsByTaskId: Record<string, TaskPR[]>;
  value: string[];
  onChange: (next: string[]) => void;
  setOpen: (open: boolean) => void;
  portalContainer: HTMLElement | null;
};

function DependencyPickerContent({
  candidates,
  mrsByTaskId,
  prsByTaskId,
  value,
  onChange,
  setOpen,
  portalContainer,
}: DependencyPickerContentProps) {
  const { t } = useTranslation();
  return (
    <PopoverContent
      align="start"
      className="w-[var(--radix-popover-trigger-width)] min-w-[min(420px,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] p-0"
      data-testid="task-create-dependencies-popover"
      portalContainer={portalContainer}
      onWheel={(event) => event.stopPropagation()}
    >
      <Command>
        <div className="flex items-center gap-1">
          <div className="min-w-0 flex-1">
            <CommandInput placeholder={t("task:searchTasksOrChangeRequest")} />
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="mr-1 h-12 w-12 min-h-12 min-w-12 cursor-pointer md:h-9 md:w-9 md:min-h-9 md:min-w-9"
                aria-label={t("task:dependencyInfoLabel")}
                data-testid="task-create-dependency-info"
              >
                <IconInfoCircle className="h-4 w-4" aria-hidden="true" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top" className="z-[60] max-w-xs">
              {t("task:dependencyInfo")}
            </TooltipContent>
          </Tooltip>
        </div>
        <CommandList>
          <CommandEmpty>{t("task:noTasksFound")}</CommandEmpty>
          <CommandGroup>
            <CommandItem
              value="__no_dependency__"
              forceMount
              onSelect={() => {
                onChange([]);
                setOpen(false);
              }}
              className="min-h-12 cursor-pointer gap-2 md:min-h-11"
              data-testid="task-create-no-dependency"
              aria-selected={value.length === 0}
            >
              <IconLock className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              <span className="flex-1">{t("task:noDependency")}</span>
              <IconCheck
                className={cn("h-4 w-4 shrink-0", value.length === 0 ? "opacity-100" : "opacity-0")}
                aria-hidden="true"
              />
            </CommandItem>
            {candidates.map((task) => (
              <DependencyOption
                key={task.id}
                task={task}
                numbers={changeRequestNumbers(
                  task,
                  mrsByTaskId[task.id] ?? NO_MRS,
                  prsByTaskId[task.id] ?? NO_PRS,
                )}
                selected={value.includes(task.id)}
                onSelect={() => {
                  onChange(
                    value.includes(task.id)
                      ? value.filter((id) => id !== task.id)
                      : [...value, task.id],
                  );
                }}
              />
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
    </PopoverContent>
  );
}

export function TaskCreateDependencies({ value, onChange, disabled }: TaskCreateDependenciesProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  // The global new-task dialog is available outside the Kanban and Tasks
  // routes, which normally hydrate these association stores.
  useWorkspacePRs(activeWorkspaceId);
  useWorkspaceMRs(activeWorkspaceId);
  const tasks = useBoardTasks();
  const mrsByTaskId = useMRsByTaskId();
  const prsByTaskId = usePRsByTaskId();

  const selectedTask = useMemo(() => {
    if (value.length !== 1) return undefined;
    return tasks.find((task) => task.id === value[0]);
  }, [tasks, value]);
  const selectedNumber = useMemo(() => {
    if (!selectedTask) return undefined;
    return changeRequestNumbers(
      selectedTask,
      mrsByTaskId[selectedTask.id] ?? NO_MRS,
      prsByTaskId[selectedTask.id] ?? NO_PRS,
    )[0];
  }, [selectedTask, mrsByTaskId, prsByTaskId]);
  const candidates = useMemo(
    () => tasks.filter((task) => !task.isArchived).sort(compareDependencyCandidates),
    [tasks],
  );
  const triggerLabel = (
    <DependencyTriggerLabel
      value={value}
      selectedTitle={selectedTask?.title}
      selectedNumber={selectedNumber}
      t={t}
    />
  );

  return (
    <div className="min-w-0" data-testid="task-create-dependencies">
      <Popover
        open={open}
        onOpenChange={(next) => {
          if (!disabled) setOpen(next);
        }}
      >
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            role="combobox"
            aria-label={t("task:dependsOn")}
            aria-expanded={open}
            className={cn(
              "h-12 min-h-12 w-full min-w-0 justify-between md:h-7 md:min-h-7",
              !disabled && "cursor-pointer",
            )}
            disabled={disabled}
            data-testid="task-create-dependencies-trigger"
          >
            <span className="flex min-w-0 flex-1 items-center gap-2 text-left">
              <IconLock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
              <span className={cn("truncate", value.length === 0 && "text-muted-foreground")}>
                {triggerLabel}
              </span>
            </span>
            <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden="true" />
          </Button>
        </PopoverTrigger>
        <DependencyPickerContent
          candidates={candidates}
          mrsByTaskId={mrsByTaskId}
          prsByTaskId={prsByTaskId}
          value={value}
          onChange={onChange}
          setOpen={setOpen}
          portalContainer={portalContainer}
        />
      </Popover>
    </div>
  );
}
