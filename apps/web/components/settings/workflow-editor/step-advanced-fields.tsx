"use client";

import { useTranslation } from "react-i18next";
import type { WorkflowInspectorProps } from "./inspector";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { hasOnEnterAction, HelpTip } from "@/components/settings/workflow-pipeline-editor-helpers";
import {
  CompleteTaskOnEnterToggle,
  useStepActions,
} from "@/components/settings/workflow-pipeline-editor-step-actions";
import { isWorkflowStepValueDirty } from "@/components/settings/workflow-dirty-state";
import {
  SessionConfigToggle,
  SessionConfigEditor,
} from "@/components/settings/workflow-session-config-editor";
import { settingsControlClassName } from "@/components/settings/settings-control";
import { PolicyCheckbox } from "./step-policy-checkbox";

export function StepAdvancedFields({
  step,
  savedStep,
  steps,
  readOnly,
  onUpdate,
  onSessionConfigResolutionPendingChange,
}: WorkflowInspectorProps) {
  const { t } = useTranslation();
  const { toggleOnEnterAction } = useStepActions({ step, onUpdate });
  return (
    <div className="max-w-3xl space-y-4">
      <PolicyCheckbox
        id={`${step.id}-reset-context`}
        checked={hasOnEnterAction(step, "reset_agent_context")}
        dirty={isWorkflowStepValueDirty(step, savedStep, (item) =>
          hasOnEnterAction(item, "reset_agent_context"),
        )}
        label={t("workflows:resetAgentContext")}
        help={
          step.agent_profile_id
            ? t("workflows:resetAgentContextHelpWithProfile")
            : t("workflows:resetAgentContextHelp")
        }
        disabled={readOnly || !!step.agent_profile_id}
        onChange={() => toggleOnEnterAction("reset_agent_context")}
      />
      <SessionConfigToggle
        step={step}
        savedStep={savedStep}
        onUpdate={onUpdate}
        readOnly={readOnly}
      />
      <SessionConfigEditor
        step={step}
        savedStep={savedStep}
        steps={steps}
        onUpdate={onUpdate}
        readOnly={readOnly}
        onResolutionPendingChange={onSessionConfigResolutionPendingChange}
      />
      <PolicyCheckbox
        id={`workflow-signal-gate-${step.id}`}
        checked={step.auto_advance_requires_signal === true}
        dirty={
          !savedStep || step.auto_advance_requires_signal !== savedStep.auto_advance_requires_signal
        }
        label={t("workflows:requireCompletionSignal")}
        help={t("workflows:requireCompletionSignalHelp")}
        disabled={readOnly}
        onChange={(checked) => onUpdate({ auto_advance_requires_signal: checked })}
        rowTestId={`${step.id}-require-signal-row`}
      />
      <PolicyCheckbox
        id={`workflow-cancel-completion-${step.id}`}
        checked={step.cancel_triggers_turn_complete === true}
        dirty={
          !savedStep ||
          step.cancel_triggers_turn_complete !== savedStep.cancel_triggers_turn_complete
        }
        label={t("workflows:runCompletionActionsWhenTurnCancelled")}
        help={t("workflows:runCompletionActionsWhenTurnCancelledHelp")}
        disabled={readOnly}
        onChange={(checked) => onUpdate({ cancel_triggers_turn_complete: checked })}
        rowTestId={`${step.id}-cancel-completion-row`}
        labelTestId={`${step.id}-cancel-completion-label`}
        helpTestId={`${step.id}-cancel-completion-help`}
      />
      <CompleteTaskOnEnterToggle
        step={step}
        savedStep={savedStep}
        onUpdate={onUpdate}
        readOnly={readOnly}
        isFinalStep={steps[steps.length - 1]?.id === step.id}
      />
      <AutoArchivePolicy
        step={step}
        savedStep={savedStep}
        readOnly={readOnly}
        onUpdate={onUpdate}
      />
    </div>
  );
}

function AutoArchivePolicy({
  step,
  savedStep,
  readOnly,
  onUpdate,
}: Pick<WorkflowInspectorProps, "step" | "savedStep" | "readOnly" | "onUpdate">) {
  const { t } = useTranslation();
  const enabled = (step.auto_archive_after_hours ?? 0) > 0;
  const dirty = isWorkflowStepValueDirty(
    step,
    savedStep,
    (item) => item.auto_archive_after_hours ?? 0,
  );
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-3">
        <Checkbox
          id={`${step.id}-auto-archive`}
          checked={enabled}
          onCheckedChange={(checked) => onUpdate({ auto_archive_after_hours: checked ? 24 : 0 })}
          disabled={readOnly}
          data-settings-dirty={dirty}
        />
        <Label
          htmlFor={`${step.id}-auto-archive`}
          className="flex min-h-11 items-center text-sm md:min-h-0 [@media(pointer:coarse)]:min-h-11"
        >
          {t("workflows:autoArchive")}
        </Label>
        <HelpTip text={t("workflows:autoArchiveHelp")} />
      </div>
      {enabled && (
        <div className="flex items-center gap-2 pl-7">
          <span className="text-sm text-muted-foreground">{t("workflows:autoArchiveAfter")}</span>
          <Input
            id={`${step.id}-auto-archive-hours`}
            type="number"
            min={1}
            value={step.auto_archive_after_hours ?? 24}
            onChange={(event) => {
              const value = Number.parseInt(event.target.value, 10);
              onUpdate({
                auto_archive_after_hours: Number.isFinite(value) && value > 0 ? value : 1,
              });
            }}
            disabled={readOnly}
            className={settingsControlClassName("w-20")}
            data-settings-dirty={dirty}
          />
          <span className="text-sm text-muted-foreground">{t("workflows:autoArchiveHours")}</span>
        </div>
      )}
    </div>
  );
}
