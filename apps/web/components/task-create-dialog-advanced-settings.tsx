"use client";

import { useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useTranslation } from "react-i18next";
import { TaskCreateDependencies } from "@/components/task-create-dialog-dependencies";
import { cn } from "@/lib/utils";

type TaskCreateAdvancedSettingsProps = {
  isCreateMode: boolean;
  isTaskStarted: boolean;
  blockedBy: string[];
  onBlockedByChange: (next: string[]) => void;
  dependenciesDisabled?: boolean;
};

export function TaskCreateAdvancedSettings({
  isCreateMode,
  isTaskStarted,
  blockedBy,
  onBlockedByChange,
  dependenciesDisabled,
}: TaskCreateAdvancedSettingsProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  if (!isCreateMode || isTaskStarted) return null;

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="min-w-0"
      data-testid="task-create-advanced-settings"
    >
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="min-h-12 h-12 w-full justify-start gap-1 px-1 text-[11px] text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground cursor-pointer md:h-7 md:min-h-7"
          data-testid="task-create-advanced-settings-trigger"
        >
          <span>{t("task:advancedSettings")}</span>
          <IconChevronDown
            className={cn("h-3 w-3 transition-transform", open && "rotate-180")}
            aria-hidden="true"
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent
        className="min-w-0 pt-1"
        data-testid="task-create-advanced-settings-content"
      >
        <TaskCreateDependencies
          value={blockedBy}
          onChange={onBlockedByChange}
          disabled={dependenciesDisabled}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}
