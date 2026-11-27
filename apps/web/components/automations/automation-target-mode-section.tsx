"use client";

import { useTranslation } from "react-i18next";
import type { TaskMode } from "@/lib/types/automation";
import type { FormState } from "./automation-payload";
import { ManagedConversationDestinationSelector } from "./managed-conversation-destination-selector";
import { SegmentedChoice, type SegmentedOption } from "./segmented-choice";
import { clearPRContinuation } from "./automation-continuation-section";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

/** Where each run's task appears: automation history, a normal task, or a plugin conversation. */
export function TargetModeSection({
  form,
  savedForm,
  workspaceId,
  updateField,
}: {
  form: FormState;
  savedForm: FormState;
  workspaceId: string;
  updateField: UpdateField;
}) {
  const { t } = useTranslation();
  const options: SegmentedOption<TaskMode>[] = [
    {
      value: "automation_run",
      label: t("automations:taskModeAutomationRun"),
      description: t("automations:taskModeAutomationRunDescription"),
    },
    {
      value: "normal_task",
      label: t("automations:taskModeNormalTask"),
      description: t("automations:taskModeNormalTaskDescription"),
    },
    {
      value: "managed_conversation",
      label: t("automations:managedConversationMode"),
      description: t("automations:managedConversationModeDescription"),
    },
  ];
  return (
    <div className="space-y-3">
      <SegmentedChoice
        id="automation-task-mode"
        label={t("automations:taskModeTitle")}
        value={form.taskMode}
        options={options}
        onValueChange={(mode) => {
          updateField("taskMode", mode);
          if (mode !== "normal_task") updateField("prContinuationStepId", "");
          // A managed conversation has no repeat-run choice, so none may stay hidden.
          if (mode === "managed_conversation") {
            updateField("continuationPolicy", "new_task");
            clearPRContinuation(updateField);
          }
        }}
        isDirty={form.taskMode !== savedForm.taskMode}
        testId="automation-task-mode"
      />
      {form.taskMode === "managed_conversation" ? (
        <ManagedConversationDestinationSelector
          workspaceId={workspaceId}
          value={form.managedDestination}
          isDirty={
            JSON.stringify(form.managedDestination) !== JSON.stringify(savedForm.managedDestination)
          }
          onChange={(destination) => updateField("managedDestination", destination)}
        />
      ) : null}
    </div>
  );
}
