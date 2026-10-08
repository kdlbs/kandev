"use client";

import { useTranslation } from "react-i18next";
import type { WorkflowInspectorProps } from "./inspector";
import { StepWipControls } from "@/components/settings/workflow-pipeline-editor-wip-controls";
import { PolicyCheckbox } from "./step-policy-checkbox";

export function StepBoardFields({
  step,
  savedStep,
  steps,
  readOnly,
  onUpdate,
}: WorkflowInspectorProps) {
  const { t } = useTranslation();
  return (
    <div className="max-w-3xl space-y-4">
      <PolicyCheckbox
        id={`workflow-start-step-${step.id}`}
        checked={step.is_start_step === true}
        dirty={!savedStep || step.is_start_step !== savedStep.is_start_step}
        label={t("workflows:startStep")}
        help={t("workflows:startStepHelp")}
        disabled={readOnly}
        onChange={(checked) => onUpdate({ is_start_step: checked })}
      />
      <PolicyCheckbox
        id={`workflow-manual-move-${step.id}`}
        checked={step.allow_manual_move !== false}
        dirty={!savedStep || step.allow_manual_move !== savedStep.allow_manual_move}
        label={t("workflows:allowManualMove")}
        help={t("workflows:allowManualMoveHelp")}
        disabled={readOnly}
        onChange={(checked) => onUpdate({ allow_manual_move: checked })}
      />
      <PolicyCheckbox
        id={`workflow-command-panel-${step.id}`}
        checked={step.show_in_command_panel !== false}
        dirty={!savedStep || step.show_in_command_panel !== savedStep.show_in_command_panel}
        label={t("workflows:showInCommandPanel")}
        help={t("workflows:showInCommandPanelHelp")}
        disabled={readOnly}
        onChange={(checked) => onUpdate({ show_in_command_panel: checked })}
      />
      <StepWipControls
        step={step}
        savedStep={savedStep}
        steps={steps}
        onUpdate={onUpdate}
        readOnly={readOnly}
      />
    </div>
  );
}
