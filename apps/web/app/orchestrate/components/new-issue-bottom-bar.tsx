"use client";

import {
  IconCircleDot,
  IconMinus,
  IconArrowUp,
  IconArrowDown,
  IconAlertTriangle,
  IconPaperclip,
  IconDotsVertical,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { IssueDraft } from "./new-issue-draft";

const STATUS_OPTIONS = [
  { value: "backlog", label: "Backlog", className: "text-muted-foreground" },
  { value: "todo", label: "Todo", className: "text-blue-600 dark:text-blue-400" },
  { value: "in_progress", label: "In Progress", className: "text-yellow-600 dark:text-yellow-400" },
] as const;

const PRIORITY_OPTIONS = [
  { value: "critical", label: "Critical", icon: IconAlertTriangle, className: "text-red-600" },
  { value: "high", label: "High", icon: IconArrowUp, className: "text-orange-600" },
  { value: "medium", label: "Medium", icon: IconMinus, className: "text-yellow-600" },
  { value: "low", label: "Low", icon: IconArrowDown, className: "text-blue-600" },
] as const;

type Props = {
  draft: IssueDraft;
  onUpdate: (patch: Partial<IssueDraft>) => void;
};

function StatusChip({ draft, onUpdate }: Props) {
  const current = STATUS_OPTIONS.find((s) => s.value === draft.status) ?? STATUS_OPTIONS[1];
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          <IconCircleDot className={`h-3.5 w-3.5 mr-1 ${current.className}`} />
          {current.label}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-40 p-1" align="start">
        {STATUS_OPTIONS.map((opt) => (
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

function PriorityChip({ draft, onUpdate }: Props) {
  const current = PRIORITY_OPTIONS.find((p) => p.value === draft.priority) ?? PRIORITY_OPTIONS[2];
  const PriorityIcon = current.icon;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          <PriorityIcon className={`h-3.5 w-3.5 mr-1 ${current.className}`} />
          {current.label}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-40 p-1" align="start">
        {PRIORITY_OPTIONS.map((opt) => {
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
        <IconPaperclip className="h-3.5 w-3.5 mr-1" />
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
