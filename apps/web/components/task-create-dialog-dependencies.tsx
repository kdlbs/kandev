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
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { cn } from "@/lib/utils";

// Keep stable empty fallbacks here. Creating a fresh array/object in a store
// selector makes Object.is report a change on every read and can cause a
// render loop, which previously blanked the dependency chip.
const NO_TASKS: KanbanState["tasks"] = [];
const NO_SNAPSHOTS: Record<string, { tasks?: KanbanState["tasks"] }> = {};

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

function DependencyTriggerLabel({
  value,
  selectedTitle,
  t,
}: {
  value: string[];
  selectedTitle?: string;
  t: (key: string, options?: { count?: number }) => string;
}) {
  if (value.length === 0) return t("task:noDependency");
  if (value.length === 1 && selectedTitle) return selectedTitle;
  return t("task:dependencyCount", { count: value.length });
}

function DependencyOption({
  task,
  selected,
  onSelect,
}: {
  task: KanbanState["tasks"][number];
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <CommandItem
      value={`${task.title} ${task.id}`}
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
      <span className="min-w-0 flex-1 truncate">{task.title}</span>
      <IconCheck
        className={cn("h-4 w-4 shrink-0", selected ? "opacity-100" : "opacity-0")}
        aria-hidden="true"
      />
    </CommandItem>
  );
}

type DependencyPickerContentProps = {
  candidates: KanbanState["tasks"];
  value: string[];
  onChange: (next: string[]) => void;
  setOpen: (open: boolean) => void;
  portalContainer: HTMLElement | null;
};

function DependencyPickerContent({
  candidates,
  value,
  onChange,
  setOpen,
  portalContainer,
}: DependencyPickerContentProps) {
  const { t } = useTranslation();
  return (
    <PopoverContent
      align="start"
      className="w-[var(--radix-popover-trigger-width)] min-w-[min(320px,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] p-0"
      data-testid="task-create-dependencies-popover"
      portalContainer={portalContainer}
      onWheel={(event) => event.stopPropagation()}
    >
      <Command>
        <div className="flex items-center gap-1">
          <div className="min-w-0 flex-1">
            <CommandInput placeholder={t("task:searchTasks")} />
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
  const tasks = useBoardTasks();

  const selectedTitle = useMemo(() => {
    if (value.length !== 1) return undefined;
    return tasks.find((task) => task.id === value[0])?.title;
  }, [tasks, value]);
  const candidates = useMemo(
    () => tasks.filter((task) => !task.isArchived).sort((a, b) => a.title.localeCompare(b.title)),
    [tasks],
  );
  const triggerLabel = <DependencyTriggerLabel value={value} selectedTitle={selectedTitle} t={t} />;

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
          value={value}
          onChange={onChange}
          setOpen={setOpen}
          portalContainer={portalContainer}
        />
      </Popover>
    </div>
  );
}
