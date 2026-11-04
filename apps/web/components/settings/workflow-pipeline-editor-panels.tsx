"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import type { WorkflowStep } from "@/lib/types/http";
import { useDebouncedCallback } from "@/hooks/use-debounce";
import {
  HelpTip,
  hasOnEnterAction,
  hasOnExitAction,
  stageTypeOnHoldToggle,
  type TransitionEventKey,
} from "./workflow-pipeline-editor-helpers";
import {
  useStepActions,
  CompleteTaskOnEnterToggle,
  TurnStartSelect,
  TurnCompleteSelect,
  EventTransitionSelect,
} from "./workflow-pipeline-editor-step-actions";
import { StepWipControls } from "./workflow-pipeline-editor-wip-controls";
import { SessionConfigEditor } from "./workflow-session-config-editor";
import { StepPromptSection } from "./workflow-step-prompt-section";
import { isWorkflowStepDirty, isWorkflowStepValueDirty } from "./workflow-dirty-state";
import { settingsControlClassName } from "./settings-control";
import { StepConfigHeader } from "./workflow-step-config-header";
import { SandboxSelectionSettings } from "./sandbox-selection-settings";

type StepAutoArchiveRowProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

function StepAutoArchiveRow({ step, savedStep, onUpdate, readOnly }: StepAutoArchiveRowProps) {
  const { t } = useTranslation();
  const isDirty = isWorkflowStepValueDirty(
    step,
    savedStep,
    (item) => item.auto_archive_after_hours ?? 0,
  );
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Checkbox
        id={`${step.id}-auto-archive`}
        checked={(step.auto_archive_after_hours ?? 0) > 0}
        onCheckedChange={(checked) => {
          if (readOnly) return;
          onUpdate({ auto_archive_after_hours: checked ? 24 : 0 });
        }}
        disabled={readOnly}
        data-settings-dirty={isDirty}
      />
      <Label htmlFor={`${step.id}-auto-archive`} className="text-sm">
        {t("workflows:autoArchive")}
      </Label>
      <HelpTip text={t("workflows:autoArchiveHelp")} />
      {(step.auto_archive_after_hours ?? 0) > 0 && (
        <>
          <span className="text-sm text-muted-foreground">{t("workflows:autoArchiveAfter")}</span>
          <Input
            id={`${step.id}-auto-archive-hours`}
            type="number"
            min={1}
            className={settingsControlClassName("w-20 text-sm")}
            value={step.auto_archive_after_hours ?? 24}
            onChange={(e) => {
              if (readOnly) return;
              const val = parseInt(e.target.value, 10);
              onUpdate({ auto_archive_after_hours: isNaN(val) || val < 1 ? 1 : val });
            }}
            disabled={readOnly}
            data-settings-dirty={isDirty}
          />
          <span className="text-sm text-muted-foreground">{t("workflows:autoArchiveHours")}</span>
        </>
      )}
    </div>
  );
}

type StepCheckboxRowProps = {
  id: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled: boolean;
  label: string;
  helpText: string;
  isDirty: boolean;
};

function StepCheckboxRow({
  id,
  checked,
  onCheckedChange,
  disabled,
  label,
  helpText,
  isDirty,
}: StepCheckboxRowProps) {
  return (
    <div className="flex items-center gap-2">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onCheckedChange(v === true)}
        disabled={disabled}
        data-settings-dirty={isDirty}
      />
      <Label htmlFor={id} className="text-sm">
        {label}
      </Label>
      <HelpTip text={helpText} />
    </div>
  );
}

// --- StepBehaviorSection ---

type StepBehaviorSectionProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  toggleOnEnterAction: (type: string) => void;
  isFinalStep: boolean;
  readOnly: boolean;
};

type StepToggleRowsProps = Omit<StepBehaviorSectionProps, "steps" | "isFinalStep">;

// The six on-enter / board-behavior toggles. Split out of StepBehaviorSection
// so that component stays under the 100-line function cap.
function StepToggleRows({
  step,
  savedStep,
  onUpdate,
  toggleOnEnterAction,
  readOnly,
}: StepToggleRowsProps) {
  const { t } = useTranslation();
  return (
    <>
      <StepCheckboxRow
        id={`${step.id}-start-step`}
        checked={step.is_start_step === true}
        onCheckedChange={(c) => !readOnly && onUpdate({ is_start_step: c })}
        disabled={readOnly}
        label={t("workflows:startStep")}
        helpText={t("workflows:startStepHelp")}
        isDirty={isWorkflowStepValueDirty(step, savedStep, (item) => item.is_start_step ?? false)}
      />
      <StepCheckboxRow
        id={`${step.id}-auto-start`}
        checked={hasOnEnterAction(step, "auto_start_agent")}
        onCheckedChange={() => !readOnly && toggleOnEnterAction("auto_start_agent")}
        disabled={readOnly}
        label={t("workflows:autoStartAgent")}
        helpText={t("workflows:autoStartAgentHelp")}
        isDirty={isWorkflowStepValueDirty(step, savedStep, (item) =>
          hasOnEnterAction(item, "auto_start_agent"),
        )}
      />
      <StepCheckboxRow
        id={`${step.id}-plan-mode`}
        checked={hasOnEnterAction(step, "enable_plan_mode")}
        onCheckedChange={() => !readOnly && toggleOnEnterAction("enable_plan_mode")}
        disabled={readOnly}
        label={t("workflows:planMode")}
        helpText={t("workflows:planModeHelp")}
        isDirty={isWorkflowStepValueDirty(step, savedStep, (item) =>
          hasOnEnterAction(item, "enable_plan_mode"),
        )}
      />
      <StepCheckboxRow
        id={`${step.id}-reset-context`}
        checked={hasOnEnterAction(step, "reset_agent_context")}
        onCheckedChange={() => !readOnly && toggleOnEnterAction("reset_agent_context")}
        disabled={readOnly || !!step.agent_profile_id}
        label={t("workflows:resetAgentContext")}
        helpText={
          step.agent_profile_id
            ? t("workflows:resetAgentContextHelpWithProfile")
            : t("workflows:resetAgentContextHelp")
        }
        isDirty={isWorkflowStepValueDirty(step, savedStep, (item) =>
          hasOnEnterAction(item, "reset_agent_context"),
        )}
      />
      <StepCheckboxRow
        id={`${step.id}-manual-move`}
        checked={step.allow_manual_move !== false}
        onCheckedChange={(c) => !readOnly && onUpdate({ allow_manual_move: c })}
        disabled={readOnly}
        label={t("workflows:allowManualMove")}
        helpText={t("workflows:allowManualMoveHelp")}
        isDirty={isWorkflowStepValueDirty(
          step,
          savedStep,
          (item) => item.allow_manual_move ?? true,
        )}
      />
      <StepCheckboxRow
        id={`${step.id}-command-panel`}
        checked={step.show_in_command_panel !== false}
        onCheckedChange={(c) => !readOnly && onUpdate({ show_in_command_panel: c })}
        disabled={readOnly}
        label={t("workflows:showInCommandPanel")}
        helpText={t("workflows:showInCommandPanelHelp")}
        isDirty={isWorkflowStepValueDirty(
          step,
          savedStep,
          (item) => item.show_in_command_panel ?? false,
        )}
      />
      <StepAutoArchiveRow
        step={step}
        savedStep={savedStep}
        onUpdate={onUpdate}
        readOnly={readOnly}
      />
      <StepHoldRow step={step} savedStep={savedStep} onUpdate={onUpdate} readOnly={readOnly} />
    </>
  );
}

type StepHoldRowProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

// The step's hold nature, stored as stage_type. Consumers read the stored value
// instead of the step's name, so renaming a hold step keeps it a hold and naming
// a step "hold" does not make it one.
function StepHoldRow({ step, savedStep, onUpdate, readOnly }: StepHoldRowProps) {
  const { t } = useTranslation();
  const stageType = step.stage_type ?? "custom";
  return (
    <StepCheckboxRow
      id={`${step.id}-hold-step`}
      checked={stageType === "hold"}
      onCheckedChange={(checked) =>
        !readOnly && onUpdate({ stage_type: stageTypeOnHoldToggle(checked, savedStep?.stage_type) })
      }
      disabled={readOnly}
      label={t("workflows:holdStep")}
      helpText={t("workflows:holdStepHelp")}
      isDirty={isWorkflowStepValueDirty(step, savedStep, (item) => item.stage_type ?? "custom")}
    />
  );
}

function StepBehaviorSection({
  step,
  savedStep,
  steps,
  onUpdate,
  toggleOnEnterAction,
  isFinalStep,
  readOnly,
}: StepBehaviorSectionProps) {
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
        <StepToggleRows
          step={step}
          savedStep={savedStep}
          onUpdate={onUpdate}
          toggleOnEnterAction={toggleOnEnterAction}
          readOnly={readOnly}
        />
      </div>
      <CompleteTaskOnEnterToggle
        step={step}
        savedStep={savedStep}
        onUpdate={onUpdate}
        readOnly={readOnly}
        isFinalStep={isFinalStep}
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

// --- EventTransitionSelects ---

type EventTransitionSelectsProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  otherSteps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  setEventTransition: (eventKey: TransitionEventKey, type: string, targetStepId?: string) => void;
  readOnly: boolean;
};

/** Transition selectors for events raised outside agent turns. */
function EventTransitionSelects(props: EventTransitionSelectsProps) {
  const { t } = useTranslation();
  return (
    <>
      <EventTransitionSelect
        {...props}
        eventKey="on_children_completed"
        testIdSegment="children-completed"
        label={t("workflows:whenChildTasksComplete")}
        help={t("workflows:childrenCompletedHelp")}
        helpAria={t("workflows:childrenCompletedHelpAria")}
      />
      <EventTransitionSelect
        {...props}
        eventKey="on_pr_merged"
        testIdSegment="pr-merged"
        label={t("workflows:whenPRMerged")}
        help={t("workflows:prMergedHelp")}
        helpAria={t("workflows:prMergedHelpAria")}
      />
      <EventTransitionSelect
        {...props}
        eventKey="on_pr_closed"
        testIdSegment="pr-closed"
        label={t("workflows:whenPRClosed")}
        help={t("workflows:prClosedHelp")}
        helpAria={t("workflows:prClosedHelpAria")}
      />
    </>
  );
}

// --- StepTransitionsSection ---

type StepTransitionsSectionProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  setTransition: (type: string) => void;
  setOnTurnStartTransition: (type: string) => void;
  setEventTransition: (eventKey: TransitionEventKey, type: string, targetStepId?: string) => void;
  toggleDisablePlanMode: () => void;
  toggleOnExitAction: (type: string) => void;
  readOnly: boolean;
};

function StepTransitionsSection({
  step,
  savedStep,
  steps,
  onUpdate,
  setTransition,
  setOnTurnStartTransition,
  setEventTransition,
  toggleDisablePlanMode,
  toggleOnExitAction,
  readOnly,
}: StepTransitionsSectionProps) {
  const { t } = useTranslation();
  const otherSteps = steps.filter((s) => s.id !== step.id);
  const planModeEnabled = hasOnEnterAction(step, "enable_plan_mode");

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-1.5">
        <Label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("workflows:transitions")}
        </Label>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        <TurnStartSelect
          step={step}
          savedStep={savedStep}
          otherSteps={otherSteps}
          onUpdate={onUpdate}
          setOnTurnStartTransition={setOnTurnStartTransition}
          readOnly={readOnly}
        />
        <TurnCompleteSelect
          step={step}
          savedStep={savedStep}
          otherSteps={otherSteps}
          onUpdate={onUpdate}
          setTransition={setTransition}
          toggleDisablePlanMode={toggleDisablePlanMode}
          planModeEnabled={planModeEnabled}
          readOnly={readOnly}
        />
        <EventTransitionSelects
          step={step}
          savedStep={savedStep}
          otherSteps={otherSteps}
          onUpdate={onUpdate}
          setEventTransition={setEventTransition}
          readOnly={readOnly}
        />
      </div>
      {planModeEnabled && (
        <div className="space-y-2">
          <div className="flex items-center gap-1.5">
            <Label className="text-xs font-medium">{t("workflows:onExit")}</Label>
            <HelpTip text={t("workflows:onExitHelp")} />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id={`${step.id}-exit-disable-plan`}
              checked={hasOnExitAction(step, "disable_plan_mode")}
              onCheckedChange={() => {
                if (readOnly) return;
                toggleOnExitAction("disable_plan_mode");
              }}
              disabled={readOnly}
              data-settings-dirty={isWorkflowStepValueDirty(step, savedStep, (item) =>
                hasOnExitAction(item, "disable_plan_mode"),
              )}
            />
            <Label htmlFor={`${step.id}-exit-disable-plan`} className="text-sm">
              {t("workflows:disablePlanMode")}
            </Label>
            <HelpTip text={t("workflows:disablePlanModeOnExitHelp")} />
          </div>
        </div>
      )}
    </div>
  );
}

type StepConfigPanelProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRemove: () => void;
  onRestoreSourceStep?: (sourceStepId: string) => void;
  readOnly?: boolean;
  onSessionConfigResolutionPendingChange?: (pending: boolean) => void;
};

export function StepConfigPanel({
  step,
  savedStep,
  steps,
  onUpdate,
  onRemove,
  onRestoreSourceStep,
  readOnly = false,
  onSessionConfigResolutionPendingChange,
}: StepConfigPanelProps) {
  const [localName, setLocalName] = useState(step.name);
  const [localPrompt, setLocalPrompt] = useState(step.prompt ?? "");

  const debouncedUpdateName = useDebouncedCallback((name: string) => {
    onUpdate({ name });
  }, 500);
  const debouncedUpdatePrompt = useDebouncedCallback((prompt: string) => {
    onUpdate({ prompt });
  }, 500);

  const actions = useStepActions({ step, onUpdate });
  const sourceTarget = step.session_target?.kind === "step" ? step.session_target : undefined;
  const isFinalStep = steps[steps.length - 1]?.id === step.id;

  return (
    <div
      key={step.id}
      className="rounded-lg border bg-card animate-in fade-in-0 slide-in-from-top-2 duration-200"
      data-settings-dirty={isWorkflowStepDirty(step, savedStep)}
      data-settings-dirty-level="container"
      data-testid={`workflow-step-panel-${step.id}`}
    >
      <StepConfigHeader
        step={step}
        savedStep={savedStep}
        steps={steps}
        onRestoreSource={
          sourceTarget && onRestoreSourceStep
            ? () => onRestoreSourceStep(sourceTarget.step_id)
            : undefined
        }
        localName={localName}
        onLocalNameChange={setLocalName}
        onUpdate={onUpdate}
        onRemove={onRemove}
        readOnly={readOnly}
        debouncedUpdateName={debouncedUpdateName}
      />
      <div className="p-4 space-y-5">
        <StepBehaviorSection
          step={step}
          savedStep={savedStep}
          steps={steps}
          onUpdate={onUpdate}
          toggleOnEnterAction={actions.toggleOnEnterAction}
          isFinalStep={isFinalStep}
          readOnly={readOnly}
        />
        <SessionConfigEditor
          {...{ step, savedStep, steps, onUpdate, readOnly }}
          onResolutionPendingChange={onSessionConfigResolutionPendingChange}
        />
        <SandboxSelectionSettings
          scope="workflow_step"
          targetId={savedStep?.id ?? ""}
          agentProfileId={step.agent_profile_id}
          readOnly={readOnly}
        />
        <StepTransitionsSection
          step={step}
          savedStep={savedStep}
          steps={steps}
          onUpdate={onUpdate}
          setTransition={actions.setTransition}
          setOnTurnStartTransition={actions.setOnTurnStartTransition}
          setEventTransition={actions.setEventTransition}
          toggleDisablePlanMode={actions.toggleDisablePlanMode}
          toggleOnExitAction={actions.toggleOnExitAction}
          readOnly={readOnly}
        />
        <StepPromptSection
          step={step}
          savedStep={savedStep}
          localPrompt={localPrompt}
          onLocalPromptChange={setLocalPrompt}
          debouncedUpdatePrompt={debouncedUpdatePrompt}
          readOnly={readOnly}
        />
      </div>
    </div>
  );
}
