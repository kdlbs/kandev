"use client";

import { useState } from "react";
import {
  IconAlertTriangle,
  IconArrowDown,
  IconArrowUp,
  IconChevronDown,
  IconMinus,
} from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@/lib/utils";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import { updateTask } from "@/lib/api/domains/office-extended-api";
import type { Task, TaskPriority } from "@/app/office/tasks/[id]/types";

type PriorityMeta = {
  value: TaskPriority;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  iconClass: string;
};

export const PRIORITY_OPTIONS: PriorityMeta[] = [
  {
    value: "critical",
    label: "Critical",
    icon: IconAlertTriangle,
    iconClass: "text-red-600",
  },
  { value: "high", label: "High", icon: IconArrowUp, iconClass: "text-orange-600" },
  { value: "medium", label: "Medium", icon: IconMinus, iconClass: "text-orange-500" },
  { value: "low", label: "Low", icon: IconArrowDown, iconClass: "text-blue-500" },
];

const BY_VALUE: Record<TaskPriority, PriorityMeta> = PRIORITY_OPTIONS.reduce(
  (acc, opt) => {
    acc[opt.value] = opt;
    return acc;
  },
  {} as Record<TaskPriority, PriorityMeta>,
);

type PriorityPickerProps = {
  task: Task;
};

export function PriorityPicker({ task }: PriorityPickerProps) {
  const [open, setOpen] = useState(false);
  const mutate = useOptimisticTaskMutation();

  const current = BY_VALUE[task.priority] ?? BY_VALUE.medium;

  const handleSelect = async (value: TaskPriority) => {
    setOpen(false);
    if (value === task.priority) return;
    try {
      await mutate(task.id, { priority: value }, () => updateTask(task.id, { priority: value }));
    } catch {
      /* toast already raised by hook */
    }
  };

  const CurrentIcon = current.icon;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-haspopup="listbox"
          aria-expanded={open}
          data-testid="priority-picker-trigger"
          className="inline-flex items-center gap-1.5 cursor-pointer rounded px-2 py-1 hover:bg-accent/50 ml-auto"
        >
          <CurrentIcon className={cn("h-3.5 w-3.5", current.iconClass)} />
          <span>{current.label}</span>
          <IconChevronDown className="h-3 w-3 opacity-50" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-44 p-1" portal={false} role="listbox">
        {PRIORITY_OPTIONS.map((opt) => {
          const Icon = opt.icon;
          return (
            <button
              key={opt.value}
              type="button"
              role="option"
              aria-selected={task.priority === opt.value}
              data-testid={`priority-picker-option-${opt.value}`}
              className={cn(
                "flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm cursor-pointer hover:bg-accent",
                task.priority === opt.value && "bg-accent/40",
              )}
              onClick={() => handleSelect(opt.value)}
            >
              <Icon className={cn("h-3.5 w-3.5", opt.iconClass)} />
              <span>{opt.label}</span>
            </button>
          );
        })}
      </PopoverContent>
    </Popover>
  );
}
