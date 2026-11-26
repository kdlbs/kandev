"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle } from "@tabler/icons-react";
import { TaskCreateDependencies } from "@/components/task-create-dialog-dependencies";
import {
  TASK_CREATE_SETTING_TRIGGER,
  TaskCreateSettingField,
} from "@/components/task-create-dialog-setting-field";
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
        <TaskCreateSettingField label={t("task:parent")} testId="task-create-parent-setting-row">
          <TaskCreateParentSelector
            value={selectedParentTaskId}
            onChange={onSelectedParentTaskIdChange}
            excludeTaskId={taskId ?? undefined}
            contextStepId={contextStepId}
            disabled={disabled}
            triggerClassName={TASK_CREATE_SETTING_TRIGGER}
          />
        </TaskCreateSettingField>
      )}
      {showCreateRelationships && (
        <TaskCreateSettingField
          label={t("task:relatedTo")}
          testId="task-create-related-setting-row"
        >
          <TaskCreateRelatedSelector
            value={relatedTaskIds}
            onChange={(value) => onRelatedTaskIdsChange?.(value)}
            blockedBy={blockedBy}
            parentTaskId={selectedParentTaskId ?? parentTaskId}
            excludeTaskId={taskId ?? undefined}
            contextStepId={contextStepId}
            disabled={disabled}
            triggerClassName={TASK_CREATE_SETTING_TRIGGER}
          />
        </TaskCreateSettingField>
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
      <TaskCreateSettingField
        testId="task-create-dependency-setting-row"
        labelTestId="task-create-dependency-setting-label"
        controlTestId="task-create-dependency-selector-container"
        label={
          <>
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
          </>
        }
      >
        <TaskCreateDependencies
          value={blockedBy}
          onChange={handleBlockedByChange}
          disabled={disabled}
          contextStepId={contextStepId}
          triggerClassName={TASK_CREATE_SETTING_TRIGGER}
        />
      </TaskCreateSettingField>
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
