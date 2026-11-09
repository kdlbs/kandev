"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconChevronDown, IconLink, IconListCheck } from "@tabler/icons-react";
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
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import {
  changeRequestNumbers,
  dependencyOptionValue,
  rankCreateDialogCandidates,
} from "@/components/task-create-dialog-dependency-search";
import { useBoardTasks } from "@/components/task-create-dialog-dependencies";
import { useMRsByTaskId, usePRsByTaskId } from "@/components/task-create-dialog-dependencies";
import { getRecentTasks } from "@/lib/recent-tasks";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskDependencyCandidate } from "@/lib/types/task-dependencies";
import { cn } from "@/lib/utils";

type BoardTask = KanbanState["tasks"][number];
const PICKER_CONTENT_CLASS =
  "w-[var(--radix-popover-trigger-width)] min-w-[min(420px,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] p-0";
const PICKER_OPTION_CLASS = "cursor-pointer gap-2 max-md:min-h-11 pointer-coarse:min-h-11";
const PICKER_CHECK_CLASS = "h-4 w-4 shrink-0";
const CHECKED_CLASS = "opacity-100";
const UNCHECKED_CLASS = "opacity-0";

export function filterCreateParentCandidates(
  tasks: BoardTask[],
  excludeTaskId?: string,
): BoardTask[] {
  const byId = new Map(tasks.map((task) => [task.id, task]));
  const excluded = new Set<string>();
  if (excludeTaskId) {
    excluded.add(excludeTaskId);
    for (const task of tasks) {
      let parentId = task.parentTaskId ?? undefined;
      const visited = new Set<string>();
      while (parentId && !visited.has(parentId)) {
        if (parentId === excludeTaskId) {
          excluded.add(task.id);
          break;
        }
        visited.add(parentId);
        parentId = byId.get(parentId)?.parentTaskId ?? undefined;
      }
    }
  }
  return tasks.filter(
    (task) =>
      !task.isArchived &&
      !excluded.has(task.id) &&
      // Mirrors the backend's ValidateCreationParent: a task that is already a
      // subtask cannot become a parent, except an Office task's own subtasks,
      // which intentionally allow arbitrary nesting depth.
      (!task.parentTaskId || task.isFromOffice),
  );
}

export function filterCreateRelatedCandidates(
  tasks: BoardTask[],
  excludeTaskId: string | undefined,
  blockedBy: string[],
  parentTaskId?: string,
): BoardTask[] {
  const excluded = new Set([...(excludeTaskId ? [excludeTaskId] : []), ...blockedBy]);
  if (parentTaskId) excluded.add(parentTaskId);
  return tasks.filter((task) => !task.isArchived && !excluded.has(task.id));
}

export function toggleRelatedTaskSelection(selected: string[], taskId: string): string[] {
  return selected.includes(taskId) ? selected.filter((id) => id !== taskId) : [...selected, taskId];
}

// Drops any related-task selection that a later parent/blocked-by choice has
// made mutually exclusive, so the three pickers never hold a task in two
// conflicting relationships at once.
export function reconcileRelatedTaskIds(
  relatedTaskIds: string[],
  conflictingIds: string[],
): string[] {
  if (conflictingIds.length === 0) return relatedTaskIds;
  const conflicts = new Set(conflictingIds);
  return relatedTaskIds.filter((id) => !conflicts.has(id));
}

export function useRelationshipConflictHandlers({
  onBlockedByChange,
  onSelectedParentTaskIdChange,
  relatedTaskIds,
  onRelatedTaskIdsChange,
}: {
  onBlockedByChange: (next: string[]) => void;
  onSelectedParentTaskIdChange?: (next: string | undefined) => void;
  relatedTaskIds: string[];
  onRelatedTaskIdsChange?: (next: string[]) => void;
}): {
  handleBlockedByChange: (next: string[]) => void;
  handleSelectedParentTaskIdChange: (next: string | undefined) => void;
} {
  const handleBlockedByChange = (next: string[]) => {
    onBlockedByChange(next);
    if (!onRelatedTaskIdsChange) return;
    const reconciled = reconcileRelatedTaskIds(relatedTaskIds, next);
    if (reconciled.length !== relatedTaskIds.length) onRelatedTaskIdsChange(reconciled);
  };
  const handleSelectedParentTaskIdChange = (next: string | undefined) => {
    onSelectedParentTaskIdChange?.(next);
    if (!next || !onRelatedTaskIdsChange) return;
    const reconciled = reconcileRelatedTaskIds(relatedTaskIds, [next]);
    if (reconciled.length !== relatedTaskIds.length) onRelatedTaskIdsChange(reconciled);
  };
  return { handleBlockedByChange, handleSelectedParentTaskIdChange };
}

export function filterCreateDialogCandidates(
  tasks: BoardTask[],
  workspaceId?: string | null,
): BoardTask[] {
  return tasks.filter(
    (task) => !workspaceId || !task.workspaceId || task.workspaceId === workspaceId,
  );
}

function useCreateDialogCandidates(
  contextStepId: string | null | undefined,
  searchText: string,
  titleMatchFirst: boolean,
): { candidates: BoardTask[]; numbers: ReadonlyMap<string, number[]> } {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const boardTasks = useBoardTasks();
  const mrsByTaskId = useMRsByTaskId();
  const prsByTaskId = usePRsByTaskId();
  const recentTaskIds = useMemo(
    () =>
      new Set(
        getRecentTasks()
          .filter((entry) => !entry.workspaceId || entry.workspaceId === workspaceId)
          .map((entry) => entry.taskId),
      ),
    [workspaceId],
  );
  const tasks = useMemo(
    () => filterCreateDialogCandidates(boardTasks, workspaceId),
    [boardTasks, workspaceId],
  );
  const numbers = useMemo(
    () =>
      new Map(
        tasks.map(
          (task) =>
            [
              task.id,
              changeRequestNumbers(task, mrsByTaskId[task.id] ?? [], prsByTaskId[task.id] ?? []),
            ] as const,
        ),
      ),
    [mrsByTaskId, prsByTaskId, tasks],
  );
  const candidates = useMemo(
    () =>
      [...tasks].sort((a, b) =>
        rankCreateDialogCandidates(a, b, {
          contextStepId: contextStepId ?? undefined,
          recentTaskIds,
          searchText,
          titleMatchFirst,
        }),
      ),
    [contextStepId, recentTaskIds, searchText, tasks, titleMatchFirst],
  );
  return { candidates, numbers };
}

function optionValue(task: TaskDependencyCandidate, numbers: number[]): string {
  return dependencyOptionValue(task, numbers);
}

type SelectorProps = {
  disabled?: boolean;
  contextStepId?: string | null;
  excludeTaskId?: string;
};

function CreateParentOption({
  task,
  numbers,
  selected,
  onSelect,
}: {
  task: BoardTask;
  numbers: number[];
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <CommandItem
      key={task.id}
      value={optionValue(task, numbers)}
      onSelect={onSelect}
      className={PICKER_OPTION_CLASS}
      data-testid={`task-create-parent-option-${task.id}`}
    >
      <IconListCheck className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      <span className="min-w-0 flex-1 line-clamp-2">{task.title}</span>
      <IconCheck
        className={cn(PICKER_CHECK_CLASS, selected ? CHECKED_CLASS : UNCHECKED_CLASS)}
        aria-hidden="true"
      />
    </CommandItem>
  );
}

function CreateRelatedOption({
  task,
  numbers,
  selected,
  onSelect,
}: {
  task: BoardTask;
  numbers: number[];
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <CommandItem
      key={task.id}
      value={optionValue(task, numbers)}
      onSelect={onSelect}
      className={PICKER_OPTION_CLASS}
      data-testid={`task-create-related-option-${task.id}`}
      aria-selected={selected}
    >
      <IconListCheck className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      <span className="min-w-0 flex-1 line-clamp-2">{task.title}</span>
      <IconCheck
        className={cn(PICKER_CHECK_CLASS, selected ? CHECKED_CLASS : UNCHECKED_CLASS)}
        aria-hidden="true"
      />
    </CommandItem>
  );
}

export function TaskCreateParentSelector({
  value,
  onChange,
  disabled,
  contextStepId,
  excludeTaskId,
}: SelectorProps & { value?: string; onChange: (value: string | undefined) => void }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const { candidates, numbers } = useCreateDialogCandidates(contextStepId, search, false);
  const eligible = useMemo(
    () => filterCreateParentCandidates(candidates, excludeTaskId),
    [candidates, excludeTaskId],
  );
  const selected = candidates.find((task) => task.id === value);
  return (
    <Popover open={open} onOpenChange={(next) => !disabled && setOpen(next)}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          role="combobox"
          aria-label={t("task:parent")}
          aria-expanded={open}
          className={cn(
            "h-7 min-h-7 w-full min-w-0 justify-between max-md:h-11 max-md:min-h-11 pointer-coarse:h-11 pointer-coarse:min-h-11",
            !disabled && "cursor-pointer",
          )}
          disabled={disabled}
          data-testid="task-create-parent-trigger"
        >
          <span className={cn("truncate", !selected && "text-muted-foreground")}>
            {selected?.title ?? t("task:noParent")}
          </span>
          <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        portalContainer={portalContainer}
        className={PICKER_CONTENT_CLASS}
        data-testid="task-create-parent-popover"
      >
        <Command>
          <CommandInput
            value={search}
            onValueChange={setSearch}
            placeholder={t("task:searchTasks")}
          />
          <CommandList>
            <CommandEmpty>{t("task:noTasksFound")}</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value="__no_parent__"
                forceMount
                onSelect={() => {
                  onChange(undefined);
                  setOpen(false);
                }}
                className={PICKER_OPTION_CLASS}
                data-testid="task-create-no-parent"
              >
                <span className="flex-1">{t("task:noParent")}</span>
                <IconCheck
                  className={cn("h-4 w-4", !value ? CHECKED_CLASS : UNCHECKED_CLASS)}
                  aria-hidden="true"
                />
              </CommandItem>
              {eligible.map((task) => (
                <CreateParentOption
                  key={task.id}
                  task={task}
                  numbers={numbers.get(task.id) ?? []}
                  selected={value === task.id}
                  onSelect={() => {
                    onChange(task.id);
                    setOpen(false);
                  }}
                />
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

export function TaskCreateRelatedSelector({
  value,
  onChange,
  blockedBy,
  parentTaskId,
  disabled,
  contextStepId,
  excludeTaskId,
}: SelectorProps & {
  value: string[];
  onChange: (value: string[]) => void;
  blockedBy: string[];
  parentTaskId?: string;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const { candidates, numbers } = useCreateDialogCandidates(contextStepId, search, true);
  const eligible = useMemo(
    () => filterCreateRelatedCandidates(candidates, excludeTaskId, blockedBy, parentTaskId),
    [blockedBy, candidates, excludeTaskId, parentTaskId],
  );
  const label = value.length
    ? t("task:relatedTaskCount", { count: value.length })
    : t("task:noRelatedTasks");
  return (
    <Popover open={open} onOpenChange={(next) => !disabled && setOpen(next)}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          role="combobox"
          aria-label={t("task:relatedTo")}
          aria-expanded={open}
          className={cn(
            "h-7 min-h-7 w-full min-w-0 justify-between max-md:h-11 max-md:min-h-11 pointer-coarse:h-11 pointer-coarse:min-h-11",
            !disabled && "cursor-pointer",
          )}
          disabled={disabled}
          data-testid="task-create-related-trigger"
        >
          <span className={cn("truncate", !value.length && "text-muted-foreground")}>{label}</span>
          <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        portalContainer={portalContainer}
        className={PICKER_CONTENT_CLASS}
        data-testid="task-create-related-popover"
      >
        <Command>
          <CommandInput
            value={search}
            onValueChange={setSearch}
            placeholder={t("task:searchTasks")}
          />
          <CommandList>
            <CommandEmpty>{t("task:noTasksFound")}</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value="__no_related_tasks__"
                forceMount
                onSelect={() => {
                  onChange([]);
                  setOpen(false);
                }}
                className={PICKER_OPTION_CLASS}
                data-testid="task-create-no-related"
              >
                <IconLink className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                <span className="flex-1">{t("task:noRelatedTasks")}</span>
                <IconCheck
                  className={cn("h-4 w-4", !value.length ? CHECKED_CLASS : UNCHECKED_CLASS)}
                  aria-hidden="true"
                />
              </CommandItem>
              {eligible.map((task) => (
                <CreateRelatedOption
                  key={task.id}
                  task={task}
                  numbers={numbers.get(task.id) ?? []}
                  selected={value.includes(task.id)}
                  onSelect={() => onChange(toggleRelatedTaskSelection(value, task.id))}
                />
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
