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
import type { TaskPriority } from "@/lib/types/http";
import type { PriorityMeta } from "@/lib/state/slices/office/types";
import type { IssueDraft } from "./new-task-draft";
import { PRIORITY_LABEL_KEYS, STATUS_LABEL_KEYS } from "../lib/label-keys";
import { useTranslation } from "react-i18next";

type StatusOption = { value: string; label: string; className: string };
type PriorityOption = {
  value: TaskPriority;
  label: string;
  icon: typeof IconMinus;
  className: string;
};

type FallbackPriorityOption = Omit<PriorityOption, "label"> & { labelKey: string };

// The fallback tables carry `labelKey`, not `label`: they are module scope, so a
// `t()` here would resolve once at import and freeze at the boot locale. The
// hooks below resolve the key at render. `value` stays untranslated — it is the
// persisted status/priority id.
const FALLBACK_STATUS_OPTIONS = [
  { value: "backlog", labelKey: STATUS_LABEL_KEYS.backlog, className: "text-muted-foreground" },
  {
    value: "todo",
    labelKey: STATUS_LABEL_KEYS.todo,
    className: "text-blue-600 dark:text-blue-400",
  },
  {
    value: "in_progress",
    labelKey: STATUS_LABEL_KEYS.in_progress,
    className: "text-yellow-600 dark:text-yellow-400",
  },
];

const PRIORITY_ICONS: Record<string, typeof IconMinus> = {
  critical: IconAlertTriangle,
  high: IconArrowUp,
  medium: IconMinus,
  low: IconArrowDown,
};

const FALLBACK_PRIORITY_OPTIONS: FallbackPriorityOption[] = [
  {
    value: "critical",
    labelKey: PRIORITY_LABEL_KEYS.critical,
    icon: IconAlertTriangle,
    className: "text-red-600",
  },
  {
    value: "high",
    labelKey: PRIORITY_LABEL_KEYS.high,
    icon: IconArrowUp,
    className: "text-orange-600",
  },
  {
    value: "medium",
    labelKey: PRIORITY_LABEL_KEYS.medium,
    icon: IconMinus,
    className: "text-yellow-600",
  },
  {
    value: "low",
    labelKey: PRIORITY_LABEL_KEYS.low,
    icon: IconArrowDown,
    className: "text-blue-600",
  },
];

function isTaskPriority(value: string): value is TaskPriority {
  return value === "critical" || value === "high" || value === "medium" || value === "low";
}

function isTaskPriorityMeta(value: PriorityMeta): value is PriorityMeta & { id: TaskPriority } {
  return isTaskPriority(value.id);
}

export function buildPriorityOptions(
  priorities: PriorityMeta[] | null | undefined,
  translate: (key: string) => string,
): PriorityOption[] {
  if (!priorities) {
    return FALLBACK_PRIORITY_OPTIONS.map((o) => ({
      value: o.value,
      label: translate(o.labelKey),
      icon: o.icon,
      className: o.className,
    }));
  }

  // Exclude "none" from the creation picker. Labels come from the workspace's
  // own priority metadata, so they travel as-is.
  return priorities.filter(isTaskPriorityMeta).map((p) => ({
    value: p.id,
    label: p.label,
    icon: PRIORITY_ICONS[p.id] ?? IconMinus,
    className: p.color,
  }));
}

type Props = {
  draft: IssueDraft;
  onUpdate: (patch: Partial<IssueDraft>) => void;
};

function useStatusOptions(): StatusOption[] {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  if (!meta) {
    return FALLBACK_STATUS_OPTIONS.map((o) => ({
      value: o.value,
      label: t(o.labelKey),
      className: o.className,
    }));
  }
  // Only show creation-relevant statuses (backlog, todo, in_progress).
  // Labels come from the workspace's own status metadata, so they travel as-is.
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
            className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer flex items-center gap-2"
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
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  return buildPriorityOptions(meta?.priorities, t);
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
              className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer flex items-center gap-2"
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

export function NewTaskBottomBar({ draft, onUpdate }: Props) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 pt-2 border-t border-border">
      <StatusChip draft={draft} onUpdate={onUpdate} />
      <PriorityChip draft={draft} onUpdate={onUpdate} />
      <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
        <IconUpload className="h-3.5 w-3.5 mr-1" />
        {t("office:upload")}
      </Button>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" className="h-7 w-7 cursor-pointer">
            <IconDotsVertical className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("office:moreOptions")}</TooltipContent>
      </Tooltip>
    </div>
  );
}
