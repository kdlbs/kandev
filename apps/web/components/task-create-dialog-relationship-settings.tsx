"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { TaskCreateDependencies } from "@/components/task-create-dialog-dependencies";
import {
  TaskCreateParentSelector,
  TaskCreateRelatedSelector,
  useRelationshipConflictHandlers,
} from "@/components/task-create-dialog-relationship-selectors";

export type RelationshipSettingsProps = {
  blockedBy: string[];
  onBlockedByChange: (next: string[]) => void;
  parentTaskId?: string;
  selectedParentTaskId?: string;
  onSelectedParentTaskIdChange?: (next: string | undefined) => void;
  relatedTaskIds: string[];
  onRelatedTaskIdsChange?: (next: string[]) => void;
  taskId?: string | null;
  contextStepId?: string | null;
  disabled?: boolean;
  showCreateRelationships?: boolean;
};

type ParentRelatedRowsProps = Pick<
  RelationshipSettingsProps,
  | "showCreateRelationships"
  | "parentTaskId"
  | "selectedParentTaskId"
  | "relatedTaskIds"
  | "onRelatedTaskIdsChange"
  | "blockedBy"
  | "taskId"
  | "contextStepId"
  | "disabled"
> & {
  onSelectedParentTaskIdChange: (next: string | undefined) => void;
};

function TaskCreateParentRelatedRows({
  showCreateRelationships,
  parentTaskId,
  selectedParentTaskId,
  onSelectedParentTaskIdChange,
  relatedTaskIds,
  onRelatedTaskIdsChange,
  blockedBy,
  taskId,
  contextStepId,
  disabled,
}: ParentRelatedRowsProps) {
  const { t } = useTranslation();
  return (
    <>
      {showCreateRelationships && !parentTaskId && (
        <div
          className="flex min-w-0 items-center gap-3"
          data-testid="task-create-parent-setting-row"
        >
          <span className="min-h-6 shrink-0 text-[11px] text-muted-foreground/70">
            {t("task:parent")}
          </span>
          <div className="min-w-0 flex-1">
            <TaskCreateParentSelector
              value={selectedParentTaskId}
              onChange={onSelectedParentTaskIdChange}
              excludeTaskId={taskId ?? undefined}
              contextStepId={contextStepId}
              disabled={disabled}
            />
          </div>
        </div>
      )}
      {showCreateRelationships && (
        <div
          className="flex min-w-0 items-center gap-3"
          data-testid="task-create-related-setting-row"
        >
          <span className="min-h-6 shrink-0 text-[11px] text-muted-foreground/70">
            {t("task:relatedTo")}
          </span>
          <div className="min-w-0 flex-1">
            <TaskCreateRelatedSelector
              value={relatedTaskIds}
              onChange={(value) => onRelatedTaskIdsChange?.(value)}
              blockedBy={blockedBy}
              parentTaskId={selectedParentTaskId ?? parentTaskId}
              excludeTaskId={taskId ?? undefined}
              contextStepId={contextStepId}
              disabled={disabled}
            />
          </div>
        </div>
      )}
    </>
  );
}

export function TaskCreateRelationshipSettings({
  blockedBy,
  onBlockedByChange,
  parentTaskId,
  selectedParentTaskId,
  onSelectedParentTaskIdChange,
  relatedTaskIds,
  onRelatedTaskIdsChange,
  taskId,
  contextStepId,
  disabled,
  showCreateRelationships = true,
}: RelationshipSettingsProps) {
  const { t } = useTranslation();
  const { handleBlockedByChange, handleSelectedParentTaskIdChange } =
    useRelationshipConflictHandlers({
      onBlockedByChange,
      onSelectedParentTaskIdChange,
      relatedTaskIds,
      onRelatedTaskIdsChange,
    });
  return (
    <>
      <div
        className="flex min-w-0 items-center gap-3"
        data-testid="task-create-dependency-setting-row"
      >
        <div
          className="flex min-h-11 shrink-0 items-center gap-1 text-[11px] text-muted-foreground/70 md:min-h-6"
          data-testid="task-create-dependency-setting-label"
        >
          <span>{t("task:dependsOn")}</span>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-11 min-h-11 w-11 min-w-11 cursor-pointer p-0 text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground md:h-6 md:min-h-6 md:w-6 md:min-w-6"
                aria-label={t("task:dependencyInfoLabel")}
                data-testid="task-create-dependency-setting-info"
              >
                <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top" className="z-[60] max-w-xs">
              {t("task:dependencyInfo")}
            </TooltipContent>
          </Tooltip>
        </div>
        <div className="min-w-0 flex-1" data-testid="task-create-dependency-selector-container">
          <TaskCreateDependencies
            value={blockedBy}
            onChange={handleBlockedByChange}
            disabled={disabled}
            contextStepId={contextStepId}
          />
        </div>
      </div>
      <TaskCreateParentRelatedRows
        showCreateRelationships={showCreateRelationships}
        parentTaskId={parentTaskId}
        selectedParentTaskId={selectedParentTaskId}
        onSelectedParentTaskIdChange={handleSelectedParentTaskIdChange}
        relatedTaskIds={relatedTaskIds}
        onRelatedTaskIdsChange={onRelatedTaskIdsChange}
        blockedBy={blockedBy}
        taskId={taskId}
        contextStepId={contextStepId}
        disabled={disabled}
      />
    </>
  );
}
