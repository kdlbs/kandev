"use client";

import { useTranslation } from "react-i18next";
import type { WorkflowInspectorProps } from "./inspector";
import { WorkflowStepAgentProfileSelector } from "@/components/settings/workflow-step-agent-profile-selector";
import { hasOnEnterAction } from "@/components/settings/workflow-pipeline-editor-helpers";
import { useStepActions } from "@/components/settings/workflow-pipeline-editor-step-actions";
import { isWorkflowStepValueDirty } from "@/components/settings/workflow-dirty-state";
import { PolicyCheckbox } from "./step-policy-checkbox";

export function StepAgentFields({
  step,
  savedStep,
  steps,
  readOnly,
  onUpdate,
  onRestoreSource,
}: WorkflowInspectorProps) {
  const { t } = useTranslation();
  const { toggleOnEnterAction } = useStepActions({ step, onUpdate });
  return (
    <div className="max-w-3xl space-y-4">
      <WorkflowStepAgentProfileSelector
        step={step}
        savedStep={savedStep}
        steps={steps}
        onUpdate={onUpdate}
        onRestoreSource={onRestoreSource}
        readOnly={readOnly}
      />
      <PolicyCheckbox
        id={`${step.id}-auto-start`}
        checked={hasOnEnterAction(step, "auto_start_agent")}
        dirty={isWorkflowStepValueDirty(step, savedStep, (item) =>
          hasOnEnterAction(item, "auto_start_agent"),
        )}
        label={t("workflows:autoStartAgent")}
        help={t("workflows:autoStartAgentHelp")}
        disabled={readOnly}
        onChange={() => toggleOnEnterAction("auto_start_agent")}
      />
      <PolicyCheckbox
        id={`${step.id}-plan-mode`}
        checked={hasOnEnterAction(step, "enable_plan_mode")}
        dirty={isWorkflowStepValueDirty(step, savedStep, (item) =>
          hasOnEnterAction(item, "enable_plan_mode"),
        )}
        label={t("workflows:planMode")}
        help={t("workflows:planModeHelp")}
        disabled={readOnly}
        onChange={() => toggleOnEnterAction("enable_plan_mode")}
      />
    </div>
  );
}
