"use client";

import { useTranslation } from "react-i18next";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { WorkflowStep } from "@/lib/types/http";
import { cn } from "@/lib/utils";
import { STEP_COLORS } from "./workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "./workflow-dirty-state";
import { WorkflowStepAgentProfileSelector } from "./workflow-step-agent-profile-selector";
import { SessionConfigToggle } from "./workflow-session-config-editor";
import { settingsActionClassName, settingsControlClassName } from "./settings-control";

type StepConfigHeaderProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  localName: string;
  onLocalNameChange: (name: string) => void;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRestoreSource?: () => void;
  onRemove: () => void;
  readOnly: boolean;
  debouncedUpdateName: (name: string) => void;
};

export function StepConfigHeader({
  step,
  savedStep,
  steps,
  localName,
  onLocalNameChange,
  onUpdate,
  onRestoreSource,
  onRemove,
  readOnly,
  debouncedUpdateName,
}: StepConfigHeaderProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 flex-1 flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
        <Input
          id={`${step.id}-name`}
          value={localName}
          onChange={(e) => {
            if (readOnly) return;
            onLocalNameChange(e.target.value);
            debouncedUpdateName(e.target.value);
          }}
          placeholder={t("workflows:stepNamePlaceholder")}
          disabled={readOnly}
          className={settingsControlClassName("w-full sm:max-w-[240px]")}
          data-settings-dirty={!savedStep || localName !== savedStep.name}
        />
        <Select
          value={step.color}
          onValueChange={(value) => {
            if (readOnly) return;
            onUpdate({ color: value });
          }}
          disabled={readOnly}
        >
          <SelectTrigger
            className={settingsControlClassName("w-full sm:w-[120px]")}
            data-settings-dirty={isWorkflowStepValueDirty(step, savedStep, (item) => item.color)}
          >
            <SelectValue placeholder={t("workflows:color")} />
          </SelectTrigger>
          <SelectContent position="popper" side="bottom" align="start">
            {STEP_COLORS.map((color) => (
              <SelectItem key={color.value} value={color.value}>
                <div className="flex items-center gap-2">
                  <div className={cn("w-3 h-3 rounded-full", color.value)} />
                  {t(color.labelKey)}
                </div>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <WorkflowStepAgentProfileSelector
          step={step}
          savedStep={savedStep}
          steps={steps}
          onUpdate={onUpdate}
          onRestoreSource={onRestoreSource}
          readOnly={readOnly}
        />
        <SessionConfigToggle
          step={step}
          savedStep={savedStep}
          onUpdate={onUpdate}
          readOnly={readOnly}
        />
      </div>
      <Button
        type="button"
        variant="ghost"
        onClick={onRemove}
        disabled={readOnly}
        className={settingsActionClassName(
          "self-end cursor-pointer text-destructive hover:text-destructive sm:self-auto",
        )}
      >
        <IconTrash className="h-3.5 w-3.5 mr-1" />
        {t("workflows:delete")}
      </Button>
    </div>
  );
}
