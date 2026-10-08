"use client";

import { useTranslation } from "react-i18next";
import type { WorkflowInspectorProps } from "./inspector";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { cn } from "@/lib/utils";
import { STEP_COLORS } from "@/components/settings/workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "@/components/settings/workflow-dirty-state";
import { settingsControlClassName } from "@/components/settings/settings-control";

export function InspectorHeader({
  step,
  savedStep,
  readOnly,
  onUpdate,
  onRemove,
}: Pick<WorkflowInspectorProps, "step" | "savedStep" | "readOnly" | "onUpdate" | "onRemove">) {
  const { t } = useTranslation();
  return (
    <div className="border-b border-border p-3">
      <div className="flex min-w-0 items-center gap-2">
        <span className={cn("h-3 w-3 shrink-0 rounded-full", step.color)} />
        <Input
          id={`${step.id}-name`}
          aria-label={t("workflows:stepName")}
          value={step.name}
          onChange={(event) => onUpdate({ name: event.target.value })}
          disabled={readOnly}
          placeholder={t("workflows:stepNamePlaceholder")}
          className={settingsControlClassName("min-w-0 flex-1 md:max-w-sm")}
          data-settings-dirty={!savedStep || step.name !== savedStep.name}
        />
        <Select
          value={step.color}
          onValueChange={(value) => onUpdate({ color: value })}
          disabled={readOnly}
        >
          <SelectTrigger
            id={`${step.id}-color`}
            aria-label={t("workflows:color")}
            className={settingsControlClassName("w-24 shrink-0")}
            data-settings-dirty={isWorkflowStepValueDirty(step, savedStep, (item) => item.color)}
          >
            <SelectValue placeholder={t("workflows:color")} />
          </SelectTrigger>
          <SelectContent position="popper" side="bottom" align="start">
            {STEP_COLORS.map((color) => (
              <SelectItem key={color.value} value={color.value}>
                <div className="flex items-center gap-2">
                  <span className={cn("h-3 w-3 rounded-full", color.value)} />
                  {t(color.labelKey)}
                </div>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {onRemove && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={controlSizingClassName(
              "icon",
              "shrink-0 cursor-pointer text-destructive hover:text-destructive",
            )}
            aria-label={t("workflows:deleteStep")}
            onClick={onRemove}
            disabled={readOnly}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  );
}
