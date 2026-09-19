"use client";

import { IconFolderOpen, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { useTaskFolderAction } from "@/hooks/use-task-folder-action";
import { TaskFolderPicker } from "./task-folder-picker";

export function OpenTaskFolderButton({ sessionId }: { sessionId: string | null }) {
  const { t } = useTranslation();
  const action = useTaskFolderAction(sessionId);
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={action.disabled ? 0 : -1} className="inline-flex">
            <Button
              variant="outline"
              size="icon"
              className="size-7 cursor-pointer"
              aria-label={t("editors:openFolder")}
              aria-busy={action.isLoading}
              data-testid="open-task-folder"
              disabled={action.disabled}
              onClick={(event) => action.open(event.currentTarget)}
            >
              {action.isLoading ? (
                <IconLoader2 className="size-4 animate-spin" aria-hidden />
              ) : (
                <IconFolderOpen className="size-4" aria-hidden />
              )}
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>
          {sessionId ? t("editors:openFolderOnHost") : t("task:selectASessionToOpenIts")}
        </TooltipContent>
      </Tooltip>
      <span role="status" className="sr-only">
        {action.isLoading ? t("editors:openingFolder") : ""}
      </span>
      <TaskFolderPicker action={action} />
    </>
  );
}
