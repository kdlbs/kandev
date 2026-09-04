"use client";

import { useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { KANBAN_PRIORITY_LABEL_KEYS, KANBAN_PRIORITY_TOKENS } from "@/lib/kanban/task-priority";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { TaskPriority } from "@/lib/types/http";

function PriorityInfo({ label, description }: { label: string; description: string }) {
  const usesTouchDrawer = useTouchDrawer();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const trigger = (
    <button
      type="button"
      className="inline-flex h-11 min-h-11 w-11 min-w-11 cursor-pointer items-center justify-center rounded-md border-0 bg-transparent p-0 text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:h-6 md:min-h-6 md:w-6 md:min-w-6"
      aria-label={label}
      aria-expanded={usesTouchDrawer ? drawerOpen : undefined}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      data-testid="task-create-priority-setting-info"
    >
      <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
    </button>
  );

  if (usesTouchDrawer) {
    return (
      <Drawer open={drawerOpen} onOpenChange={setDrawerOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{label}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent side="top" className="z-[60] max-w-xs">
        {description}
      </TooltipContent>
    </Tooltip>
  );
}

export function TaskCreatePrioritySelect({
  value,
  onChange,
  disabled,
}: {
  value: TaskPriority;
  onChange: (value: TaskPriority) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const priorityLabel = t("kanban:priority");
  const priorityInfoLabel = t("task:priorityInfoLabel");
  const priorityInfo = t("task:priorityInfo");
  return (
    <div className="flex items-center gap-2">
      <div className="flex min-h-11 shrink-0 items-center gap-1 text-xs text-muted-foreground md:min-h-6">
        <label htmlFor="task-create-priority-select">{priorityLabel}</label>
        <PriorityInfo label={priorityInfoLabel} description={priorityInfo} />
      </div>
      <Select
        value={value}
        onValueChange={(next) => onChange(next as TaskPriority)}
        disabled={disabled}
      >
        <SelectTrigger
          id="task-create-priority-select"
          data-testid="task-create-priority-select"
          aria-label={priorityLabel}
          className="h-11 min-h-11 w-32 border-border/60 bg-muted/30 hover:bg-muted/60 sm:h-8 sm:min-h-0"
          size="sm"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {KANBAN_PRIORITY_TOKENS.map((token) => (
            <SelectItem
              key={token}
              value={token}
              data-testid={`task-create-priority-option-${token}`}
              className="min-h-11 sm:min-h-7"
            >
              {t(KANBAN_PRIORITY_LABEL_KEYS[token])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
