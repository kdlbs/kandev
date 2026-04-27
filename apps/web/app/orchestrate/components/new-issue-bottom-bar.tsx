"use client";

import {
  IconCircleDot,
  IconMinus,
  IconArrowUp,
  IconArrowDown,
  IconAlertTriangle,
  IconUpload,
  IconDotsVertical,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type { IssueDraft } from "./new-issue-draft";

type StatusOption = { value: string; label: string; className: string };
type PriorityOption = { value: string; label: string; icon: typeof IconMinus; className: string };

const FALLBACK_STATUS_OPTIONS: StatusOption[] = [
  { value: "backlog", label: "Backlog", className: "text-muted-foreground" },
  { value: "todo", label: "Todo", className: "text-blue-600 dark:text-blue-400" },
  { value: "in_progress", label: "In Progress", className: "text-yellow-600 dark:text-yellow-400" },
];

const PRIORITY_ICONS: Record<string, typeof IconMinus> = {
  critical: IconAlertTriangle,
  high: IconArrowUp,
  medium: IconMinus,
  low: IconArrowDown,
};

const FALLBACK_PRIORITY_OPTIONS: PriorityOption[] = [
  { value: "critical", label: "Critical", icon: IconAlertTriangle, className: "text-red-600" },
  { value: "high", label: "High", icon: IconArrowUp, className: "text-orange-600" },
  { value: "medium", label: "Medium", icon: IconMinus, className: "text-yellow-600" },
  { value: "low", label: "Low", icon: IconArrowDown, className: "text-blue-600" },
];

type Props = {
  draft: IssueDraft;
  onUpdate: (patch: Partial<IssueDraft>) => void;
};

function useStatusOptions(): StatusOption[] {
  const meta = useAppStore((s) => s.orchestrate.meta);
  if (!meta) return FALLBACK_STATUS_OPTIONS;
  // Only show creation-relevant statuses (backlog, todo, in_progress)
  const creationStatuses = ["backlog", "todo", "in_progress"];
  return meta.statuses
    .filter((s) => creationStatuses.includes(s.id))
    .map((s) => ({ value: s.id, label: s.label, className: s.color }));
}

function StatusChip({ draft, onUpdate }: Props) {
  const options = useStatusOptions();
  const current = options.find((s) => s.value === draft.status) ?? options[1] ?? options[0];
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          <IconCircleDot className={`h-3.5 w-3.5 mr-1 ${current?.className ?? ""}`} />
          {current?.label ?? draft.status}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-40 p-1" align="start">
        {options.map((opt) => (
          <button
            key={opt.value}
            type="button"
            className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-accent cursor-pointer flex items-center gap-2"
            onClick={() => onUpdate({ status: opt.value })}
          >
            <IconCircleDot className={`h-3.5 w-3.5 ${opt.className}`} />
            {opt.label}
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}

function usePriorityOptions(): PriorityOption[] {
  const meta = useAppStore((s) => s.orchestrate.meta);
  if (!meta) return FALLBACK_PRIORITY_OPTIONS;
  // Exclude "none" from the creation picker
  return meta.priorities
    .filter((p) => p.id !== "none")
    .map((p) => ({
      value: p.id,
      label: p.label,
      icon: PRIORITY_ICONS[p.id] ?? IconMinus,
      className: p.color,
    }));
}

function PriorityChip({ draft, onUpdate }: Props) {
  const options = usePriorityOptions();
  const current = options.find((p) => p.value === draft.priority) ?? options[2] ?? options[0];
  const PriorityIcon = current?.icon ?? IconMinus;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          <PriorityIcon className={`h-3.5 w-3.5 mr-1 ${current?.className ?? ""}`} />
          {current?.label ?? draft.priority}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-40 p-1" align="start">
        {options.map((opt) => {
          const Icon = opt.icon;
          return (
            <button
              key={opt.value}
              type="button"
              className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-accent cursor-pointer flex items-center gap-2"
              onClick={() => onUpdate({ priority: opt.value })}
            >
              <Icon className={`h-3.5 w-3.5 ${opt.className}`} />
              {opt.label}
            </button>
          );
        })}
      </PopoverContent>
    </Popover>
  );
}

export function NewIssueBottomBar({ draft, onUpdate }: Props) {
  return (
    <div className="flex items-center gap-2 pt-2 border-t border-border">
      <StatusChip draft={draft} onUpdate={onUpdate} />
      <PriorityChip draft={draft} onUpdate={onUpdate} />
      <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
        <IconUpload className="h-3.5 w-3.5 mr-1" />
        Upload
      </Button>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" className="h-7 w-7 cursor-pointer">
            <IconDotsVertical className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>More options</TooltipContent>
      </Tooltip>
    </div>
  );
}
