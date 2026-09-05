"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconArrowDown, IconArrowUp, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import type { WorkflowStep } from "@/lib/types/http";
import {
  getWorkflowActionCatalog,
  scriptActionConfig,
  validateWorkflowScriptAction,
  type WorkflowActionRecord,
  type WorkflowLifecycleTrigger,
} from "@/lib/workflows/workflow-action-catalog";
import type { WorkflowScriptFailurePolicy } from "@/lib/types/workflow-actions";

export type WorkflowActionEditorProps = {
  action: WorkflowActionRecord;
  actionIndex: number;
  actionCount: number;
  trigger: WorkflowLifecycleTrigger;
  steps: WorkflowStep[];
  readOnly: boolean;
  onChange: (updates: Partial<WorkflowActionRecord>) => void;
  onMove: (direction: -1 | 1) => void;
  onRemove: () => void;
};

const TRIGGER_HELP_KEYS: Record<WorkflowLifecycleTrigger, string> = {
  on_enter: "workflows:runScriptHelp",
  on_turn_start: "workflows:onTurnStartHelp",
  on_turn_complete: "workflows:onTurnCompleteHelp",
  on_exit: "workflows:onExitHelp",
};

export function WorkflowActionEditor({
  action,
  actionIndex,
  actionCount,
  trigger,
  steps,
  readOnly,
  onChange,
  onMove,
  onRemove,
}: WorkflowActionEditorProps) {
  const { t } = useTranslation();
  const descriptor = getWorkflowActionCatalog(trigger).find((item) => item.type === action.type);
  const label = descriptor ? t(descriptor.labelKey) : action.type;
  return (
    <section
      className="space-y-4 rounded-lg border border-primary/30 bg-primary/[0.03] p-4"
      data-testid={`workflow-action-editor-${trigger}-${actionIndex}`}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">{label}</p>
          <p className="text-xs text-muted-foreground">{t(TRIGGER_HELP_KEYS[trigger])}</p>
        </div>
        <ActionEditorToolbar
          actionIndex={actionIndex}
          actionCount={actionCount}
          readOnly={readOnly}
          onMove={onMove}
          onRemove={onRemove}
        />
      </div>
      <ActionEditorFields
        action={action}
        trigger={trigger}
        steps={steps}
        readOnly={readOnly}
        onChange={onChange}
      />
    </section>
  );
}

function ActionEditorToolbar({
  actionIndex,
  actionCount,
  readOnly,
  onMove,
  onRemove,
}: Pick<
  WorkflowActionEditorProps,
  "actionIndex" | "actionCount" | "readOnly" | "onMove" | "onRemove"
>) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1">
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-11 w-11 cursor-pointer"
        aria-label={t("workflows:moveActionUp")}
        disabled={readOnly || actionIndex === 0}
        onClick={() => onMove(-1)}
      >
        <IconArrowUp className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-11 w-11 cursor-pointer"
        aria-label={t("workflows:moveActionDown")}
        disabled={readOnly || actionIndex === actionCount - 1}
        onClick={() => onMove(1)}
      >
        <IconArrowDown className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-11 w-11 cursor-pointer text-destructive hover:text-destructive"
        aria-label={t("workflows:removeAction")}
        disabled={readOnly}
        onClick={onRemove}
      >
        <IconTrash className="h-4 w-4" />
      </Button>
    </div>
  );
}

function ActionEditorFields({
  action,
  trigger,
  steps,
  readOnly,
  onChange,
}: Pick<WorkflowActionEditorProps, "action" | "trigger" | "steps" | "readOnly" | "onChange">) {
  const { t } = useTranslation();
  if (action.type === "run_script") {
    return (
      <ScriptActionFields
        action={action}
        trigger={trigger}
        readOnly={readOnly}
        onChange={onChange}
      />
    );
  }
  if (action.type === "move_to_step") {
    return (
      <MoveToStepFields action={action} steps={steps} readOnly={readOnly} onChange={onChange} />
    );
  }
  if (action.type === "configure_session") {
    return (
      <p className="text-xs text-muted-foreground">{t("workflows:configureSessionActionHelp")}</p>
    );
  }
  if (action.type === "set_session_mode") {
    return (
      <ActionConfigInput
        action={action}
        field="mode"
        labelKey="workflows:setSessionMode"
        helpKey="workflows:setSessionModeHelp"
        readOnly={readOnly}
        onChange={onChange}
      />
    );
  }
  if (action.type === "queue_run_for_each_participant") {
    return (
      <ActionConfigInput
        action={action}
        field="role"
        labelKey="workflows:participantRole"
        helpKey="workflows:queueRunForEachParticipantHelp"
        readOnly={readOnly}
        onChange={onChange}
      />
    );
  }
  if (action.type === "queue_run") {
    return (
      <ActionConfigInput
        action={action}
        field="target"
        labelKey="workflows:queueTarget"
        helpKey="workflows:queueRunHelp"
        readOnly={readOnly}
        onChange={onChange}
      />
    );
  }
  if (action.type === "ensure_participant_seat") {
    return (
      <ActionConfigInput
        action={action}
        field="role"
        labelKey="workflows:participantRole"
        helpKey="workflows:ensureParticipantSeatHelp"
        readOnly={readOnly}
        onChange={onChange}
      />
    );
  }
  if (action.type === "run_code_review") {
    return (
      <ActionConfigInput
        action={action}
        field="agent_profile_id"
        labelKey="workflows:agentProfile"
        helpKey="workflows:runCodeReviewHelp"
        readOnly={readOnly}
        onChange={onChange}
      />
    );
  }
  if (action.type === "auto_start_agent") {
    return <AutoStartFields action={action} readOnly={readOnly} onChange={onChange} />;
  }
  return <p className="text-xs text-muted-foreground">{t("workflows:actionRunsAutomatically")}</p>;
}

function ScriptActionFields({
  action,
  trigger,
  readOnly,
  onChange,
}: Pick<WorkflowActionEditorProps, "action" | "trigger" | "readOnly" | "onChange">) {
  const { t } = useTranslation();
  const config = scriptActionConfig(action) ?? { command: "" };
  const validation = validateWorkflowScriptAction(action);
  const updateConfig = (updates: Record<string, unknown>) =>
    onChange({ config: { ...action.config, ...updates } });
  return (
    <div className="space-y-4" data-testid={`workflow-script-editor-${trigger}`}>
      <div className="space-y-1.5">
        <Label htmlFor={`workflow-script-command-${trigger}`}>{t("workflows:scriptCommand")}</Label>
        <Textarea
          id={`workflow-script-command-${trigger}`}
          value={typeof config.command === "string" ? config.command : ""}
          onChange={(event) => updateConfig({ command: event.target.value })}
          disabled={readOnly}
          placeholder={t("workflows:scriptCommandPlaceholder")}
          className="min-h-24 font-mono text-xs"
          aria-invalid={validation.valid ? undefined : true}
        />
        {!validation.valid && <ValidationMessage message={t(validation.errorKey)} />}
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor={`workflow-script-timeout-${trigger}`}>
            {t("workflows:scriptTimeout")}
          </Label>
          <Input
            id={`workflow-script-timeout-${trigger}`}
            type="number"
            min={1}
            max={86400}
            value={typeof config.timeout_seconds === "number" ? config.timeout_seconds : 600}
            onChange={(event) => updateConfig({ timeout_seconds: Number(event.target.value) })}
            disabled={readOnly}
            inputMode="numeric"
          />
          <p className="text-xs text-muted-foreground">{t("workflows:scriptTimeoutHelp")}</p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={`workflow-script-policy-${trigger}`}>
            {t("workflows:scriptFailurePolicy")}
          </Label>
          <Select
            value={
              config.failure_policy === "continue" || config.failure_policy === "block"
                ? config.failure_policy
                : "block"
            }
            onValueChange={(value) =>
              updateConfig({ failure_policy: value as WorkflowScriptFailurePolicy })
            }
            disabled={readOnly}
          >
            <SelectTrigger id={`workflow-script-policy-${trigger}`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="block">{t("workflows:scriptFailurePolicyBlock")}</SelectItem>
              <SelectItem value="continue">{t("workflows:scriptFailurePolicyContinue")}</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">{t("workflows:scriptFailurePolicyHelp")}</p>
        </div>
      </div>
    </div>
  );
}

function MoveToStepFields({
  action,
  steps,
  readOnly,
  onChange,
}: Pick<WorkflowActionEditorProps, "action" | "steps" | "readOnly" | "onChange">) {
  const { t } = useTranslation();
  const target = typeof action.config?.step_id === "string" ? action.config.step_id : "";
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label>{t("workflows:transitionTarget")}</Label>
        <Select
          value={target}
          onValueChange={(stepId) => onChange({ config: { ...action.config, step_id: stepId } })}
          disabled={readOnly}
        >
          <SelectTrigger>
            <SelectValue placeholder={t("workflows:selectStep")} />
          </SelectTrigger>
          <SelectContent>
            {steps.map((step) => (
              <SelectItem key={step.id} value={step.id}>
                {step.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <TransitionGuardFields action={action} readOnly={readOnly} onChange={onChange} />
    </div>
  );
}

function TransitionGuardFields({
  action,
  readOnly,
  onChange,
}: Pick<WorkflowActionEditorProps, "action" | "readOnly" | "onChange">) {
  const { t } = useTranslation();
  const requiresApproval = action.config?.requires_approval === true;
  return (
    <label className="flex min-h-11 items-center gap-3 text-sm">
      <Checkbox
        checked={requiresApproval}
        onCheckedChange={(checked) =>
          onChange({ config: { ...action.config, requires_approval: checked === true } })
        }
        disabled={readOnly}
      />
      <span>{t("workflows:transitionRequiresApproval")}</span>
    </label>
  );
}

function AutoStartFields({
  action,
  readOnly,
  onChange,
}: Pick<WorkflowActionEditorProps, "action" | "readOnly" | "onChange">) {
  const { t } = useTranslation();
  const config = action.config ?? {};
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t("workflows:autoStartAgentHelp")}</p>
      <div className="space-y-1.5">
        <Label htmlFor="workflow-auto-start-prompt">{t("workflows:promptOverride")}</Label>
        <Textarea
          id="workflow-auto-start-prompt"
          value={typeof config.prompt_override === "string" ? config.prompt_override : ""}
          onChange={(event) =>
            onChange({ config: { ...action.config, prompt_override: event.target.value } })
          }
          disabled={readOnly}
          className="min-h-20 text-sm"
        />
      </div>
    </div>
  );
}

function ActionConfigInput({
  action,
  field,
  labelKey,
  helpKey,
  readOnly,
  onChange,
}: {
  action: WorkflowActionRecord;
  field: string;
  labelKey: string;
  helpKey: string;
  readOnly: boolean;
  onChange: (updates: Partial<WorkflowActionRecord>) => void;
}) {
  const { t } = useTranslation();
  const value = typeof action.config?.[field] === "string" ? action.config[field] : "";
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t(helpKey)}</p>
      <div className="space-y-1.5">
        <Label htmlFor={`workflow-action-${field}`}>{t(labelKey)}</Label>
        <Input
          id={`workflow-action-${field}`}
          value={value}
          onChange={(event) =>
            onChange({ config: { ...action.config, [field]: event.target.value } })
          }
          disabled={readOnly}
        />
      </div>
    </div>
  );
}

function ValidationMessage({ message }: { message: string }) {
  return (
    <p className="flex items-center gap-1 text-xs text-destructive" role="alert">
      <IconAlertTriangle className="h-3.5 w-3.5 shrink-0" />
      {message}
    </p>
  );
}
