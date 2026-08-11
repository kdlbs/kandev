"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";
import {
  DEFAULT_ONBOARDING_TASK_DESCRIPTION,
  DEFAULT_ONBOARDING_TASK_TITLE,
} from "./setup-task-defaults";
import { useTranslation } from "react-i18next";

type StepTaskProps = {
  agentName: string;
  taskTitle: string;
  taskDescription: string;
  onChange: (patch: { taskTitle?: string; taskDescription?: string }) => void;
};

export function StepTask({ agentName, taskTitle, taskDescription, onChange }: StepTaskProps) {
  const { t } = useTranslation();
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(taskTitle);
  // The coordinator step requires a non-empty agentName before advancing,
  // so by the time this step renders we always have a real value.
  const name = agentName.trim() || "coordinator";
  return (
    <div className="space-y-6">
      <div>
        {/* `name` is the coordinator's own name — user data, interpolated, never translated. */}
        <h2 className="text-xl font-semibold">{t("office:giveAgentSomethingToDo", { name })}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("office:agentWillUseThisStarterTask", { name })}
        </p>
      </div>
      <div className="space-y-4">
        <div>
          <Label htmlFor="task-title">{t("office:taskTitle")}</Label>
          <Input
            ref={inputRef}
            id="task-title"
            value={taskTitle}
            onChange={(e) => onChange({ taskTitle: clampChange(e) })}
            placeholder={DEFAULT_ONBOARDING_TASK_TITLE}
            className="mt-1"
            autoFocus
          />
        </div>
        <div>
          <Label htmlFor="task-desc">{t("office:description")}</Label>
          <Textarea
            id="task-desc"
            value={taskDescription}
            onChange={(e) => onChange({ taskDescription: e.target.value })}
            placeholder={DEFAULT_ONBOARDING_TASK_DESCRIPTION}
            className="mt-1 min-h-[220px]"
          />
        </div>
      </div>
    </div>
  );
}
