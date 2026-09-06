"use client";

import { useTranslation } from "react-i18next";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import type { WorkflowStep } from "@/lib/types/http";
import { AutomationTab, type WorkflowActionSelection } from "./automation-tab";
import { WorkflowStepAgentProfileSelector } from "@/components/settings/workflow-step-agent-profile-selector";
import {
  SessionConfigEditor,
  SessionConfigToggle,
} from "@/components/settings/workflow-session-config-editor";
import { StepWipControls } from "@/components/settings/workflow-pipeline-editor-wip-controls";
import { HelpTip, STEP_COLORS } from "@/components/settings/workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "@/components/settings/workflow-dirty-state";
import { cn } from "@/lib/utils";

export type WorkflowInspectorTab = "agent" | "automation" | "policies";

export type WorkflowInspectorProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  activeTab: WorkflowInspectorTab;
  readOnly: boolean;
  onTabChange: (tab: WorkflowInspectorTab) => void;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRemove?: () => void;
  focusedAction?: WorkflowActionSelection | null;
  mobile?: boolean;
  onSessionConfigResolutionPendingChange?: (pending: boolean) => void;
  onFocusAction?: (selection: WorkflowActionSelection | null, mode?: "push" | "replace") => void;
};

export function WorkflowInspector({
  step,
  savedStep,
  steps,
  activeTab,
  readOnly,
  onTabChange,
  onUpdate,
  onRemove,
  focusedAction,
  mobile = false,
  onSessionConfigResolutionPendingChange,
  onFocusAction,
}: WorkflowInspectorProps) {
  return (
    <section
      className="min-w-0 rounded-xl border border-border bg-card"
      data-testid="workflow-editor-inspector"
    >
      <InspectorHeader
        step={step}
        savedStep={savedStep}
        readOnly={readOnly}
        onUpdate={onUpdate}
        onRemove={onRemove}
        activeTab={activeTab}
        onTabChange={onTabChange}
      />
      <div className="min-w-0 p-4">
        {activeTab === "agent" && (
          <AgentTab
            step={step}
            savedStep={savedStep}
            steps={steps}
            readOnly={readOnly}
            onUpdate={onUpdate}
            onSessionConfigResolutionPendingChange={onSessionConfigResolutionPendingChange}
          />
        )}
        {activeTab === "automation" && (
          <AutomationTab
            step={step}
            steps={steps}
            readOnly={readOnly}
            onUpdate={onUpdate}
            focusedAction={focusedAction}
            mobile={mobile}
            onFocusAction={onFocusAction}
          />
        )}
        {activeTab === "policies" && (
          <PoliciesTab
            step={step}
            savedStep={savedStep}
            steps={steps}
            readOnly={readOnly}
            onUpdate={onUpdate}
          />
        )}
      </div>
    </section>
  );
}

function InspectorHeader({
  step,
  savedStep,
  readOnly,
  onUpdate,
  onRemove,
  activeTab,
  onTabChange,
}: Pick<
  WorkflowInspectorProps,
  "step" | "savedStep" | "readOnly" | "onUpdate" | "onRemove" | "activeTab" | "onTabChange"
>) {
  const { t } = useTranslation();
  return (
    <div className="border-b border-border p-3">
      <div className="mb-3 flex min-w-0 items-center gap-2">
        <span className={cn("h-3 w-3 shrink-0 rounded-full", step.color)} />
        <Input
          id={`${step.id}-name`}
          aria-label={t("workflows:stepName")}
          value={step.name}
          onChange={(event) => onUpdate({ name: event.target.value })}
          disabled={readOnly}
          placeholder={t("workflows:stepNamePlaceholder")}
          className="h-8 min-w-0 flex-1 sm:max-w-[240px]"
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
            className="h-8 w-24 shrink-0"
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
            className="h-11 w-11 shrink-0 cursor-pointer text-destructive hover:text-destructive"
            aria-label={t("workflows:deleteStep")}
            onClick={onRemove}
            disabled={readOnly}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        )}
      </div>
      <nav
        className="flex min-w-0 gap-1 overflow-x-auto rounded-lg bg-muted/30 p-1"
        aria-label={t("workflows:stepInspector")}
      >
        <InspectorTabButton
          tab="agent"
          label={t("workflows:agentTab")}
          activeTab={activeTab}
          onTabChange={onTabChange}
        />
        <InspectorTabButton
          tab="automation"
          label={t("workflows:automationTab")}
          activeTab={activeTab}
          onTabChange={onTabChange}
        />
        <InspectorTabButton
          tab="policies"
          label={t("workflows:policiesTab")}
          activeTab={activeTab}
          onTabChange={onTabChange}
        />
      </nav>
    </div>
  );
}

function InspectorTabButton({
  tab,
  label,
  activeTab,
  onTabChange,
}: {
  tab: WorkflowInspectorTab;
  label: string;
  activeTab: WorkflowInspectorTab;
  onTabChange: (tab: WorkflowInspectorTab) => void;
}) {
  const active = activeTab === tab;
  return (
    <button
      type="button"
      className={cn(
        "min-h-11 h-11 min-w-0 flex-1 cursor-pointer rounded-md px-2 text-xs font-medium transition-colors sm:h-8 sm:min-h-8",
        active
          ? "bg-primary text-primary-foreground"
          : "bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
      aria-current={active ? "page" : undefined}
      onClick={() => onTabChange(tab)}
      data-testid={`workflow-editor-tab-${tab}`}
    >
      {label}
    </button>
  );
}

function AgentTab({
  step,
  savedStep,
  steps,
  readOnly,
  onUpdate,
  onSessionConfigResolutionPendingChange,
}: Pick<
  WorkflowInspectorProps,
  | "step"
  | "savedStep"
  | "steps"
  | "readOnly"
  | "onUpdate"
  | "onSessionConfigResolutionPendingChange"
>) {
  const { t } = useTranslation();
  return (
    <div className="space-y-5" data-testid="workflow-agent-tab">
      <WorkflowStepAgentProfileSelector
        step={step}
        savedStep={savedStep}
        onUpdate={onUpdate}
        readOnly={readOnly}
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
      <div className="space-y-1.5">
        <Label htmlFor={`workflow-step-prompt-${step.id}`}>{t("workflows:stepPrompt")}</Label>
        <Textarea
          id={`workflow-step-prompt-${step.id}`}
          value={step.prompt ?? ""}
          onChange={(event) => onUpdate({ prompt: event.target.value })}
          disabled={readOnly}
          placeholder={t("workflows:stepPromptPlaceholder")}
          className="min-h-32"
        />
        <p className="text-xs text-muted-foreground">{t("workflows:stepPromptHelp")}</p>
      </div>
    </div>
  );
}

function PoliciesTab({
  step,
  savedStep,
  steps,
  readOnly,
  onUpdate,
}: Pick<WorkflowInspectorProps, "step" | "savedStep" | "steps" | "readOnly" | "onUpdate">) {
  const { t } = useTranslation();
  return (
    <div className="space-y-5" data-testid="workflow-policies-tab">
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
      <AutoArchivePolicy
        step={step}
        savedStep={savedStep}
        readOnly={readOnly}
        onUpdate={onUpdate}
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
        <Label htmlFor={`${step.id}-auto-archive`} className="text-sm">
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
            className="h-8 w-20"
            data-settings-dirty={dirty}
          />
          <span className="text-sm text-muted-foreground">{t("workflows:autoArchiveHours")}</span>
        </div>
      )}
    </div>
  );
}

function PolicyCheckbox({
  id,
  checked,
  dirty,
  label,
  help,
  disabled,
  onChange,
  rowTestId,
  labelTestId,
  helpTestId,
}: {
  id: string;
  checked: boolean;
  dirty: boolean;
  label: string;
  help: string;
  disabled: boolean;
  onChange: (checked: boolean) => void;
  rowTestId?: string;
  labelTestId?: string;
  helpTestId?: string;
}) {
  return (
    <div className="flex items-start gap-3" data-testid={rowTestId}>
      <Checkbox
        id={id}
        className="mt-1"
        checked={checked}
        onCheckedChange={(value) => onChange(value === true)}
        disabled={disabled}
        data-settings-dirty={dirty}
      />
      <div className="min-w-0 space-y-0.5">
        <div className="flex items-center gap-1">
          <Label htmlFor={id} className="text-sm" data-testid={labelTestId}>
            {label}
          </Label>
          {helpTestId && <HelpTip testId={helpTestId} text={help} />}
        </div>
        <p className="text-xs text-muted-foreground">{help}</p>
      </div>
    </div>
  );
}
