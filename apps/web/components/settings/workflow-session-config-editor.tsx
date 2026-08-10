"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useHealthyAgentProfiles } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import type { WorkflowStep } from "@/lib/types/http";
import type { ConfigureSessionOperation, ConfigureSessionRule } from "@/lib/types/workflow-actions";
import {
  analyzeSessionConfigCarryForward,
  type SessionConfigCarryWarning,
} from "@/lib/workflows/session-config-carry-analysis";
import { HelpTip } from "./workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "./workflow-dirty-state";
import { SessionConfigCarryWarningPanel } from "./workflow-session-config-carry-warning";
import { SessionConfigRuleCard } from "./workflow-session-config-rule-card";
import {
  buildAgentChoices,
  configureSessionAction,
  defaultConfigureSessionRule,
  defaultModelForAgent,
  withoutConfigureSession,
  withConfigureSessionRules,
  type AgentChoice,
} from "./workflow-session-config-shared";

type SessionConfigEditorProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
  onResolutionPendingChange?: (pending: boolean) => void;
};

export function SessionConfigToggle({
  step,
  savedStep,
  onUpdate,
  readOnly,
}: {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
}) {
  const { t } = useTranslation();
  const profiles = useHealthyAgentProfiles(step.agent_profile_id);
  const availableAgents = useAvailableAgents();
  const enabled = !!configureSessionAction(step);
  const isDirty = isWorkflowStepValueDirty(step, savedStep, (item) =>
    JSON.stringify(configureSessionAction(item)?.config?.rules ?? []),
  );
  const disabled = readOnly || !!step.agent_profile_id || (!enabled && profiles.length === 0);

  const setEnabled = (nextEnabled: boolean) => {
    if (readOnly) return;
    if (!nextEnabled) {
      onUpdate(withoutConfigureSession(step));
      return;
    }
    const rule = defaultConfigureSessionRule(profiles, availableAgents.items);
    if (!rule) return;
    onUpdate(withConfigureSessionRules(step, [rule]));
  };

  return (
    <div className="flex min-h-10 items-center gap-2 sm:ml-1">
      <Checkbox
        id={`${step.id}-override-original-session`}
        checked={enabled}
        onCheckedChange={(checked) => setEnabled(checked === true)}
        disabled={disabled}
        data-settings-dirty={isDirty}
        data-testid={`${step.id}-override-original-session`}
      />
      <Label htmlFor={`${step.id}-override-original-session`} className="text-sm">
        {t("workflows:overrideOriginalSessionOptions")}
      </Label>
      <HelpTip
        testId={`${step.id}-override-original-session-help`}
        text={t("workflows:overrideOriginalSessionOptionsHelp")}
      />
    </div>
  );
}

export function SessionConfigEditor({
  step,
  savedStep,
  steps,
  onUpdate,
  readOnly,
  onResolutionPendingChange,
}: SessionConfigEditorProps) {
  const profiles = useHealthyAgentProfiles(step.agent_profile_id);
  const availableAgents = useAvailableAgents();
  const action = configureSessionAction(step);
  const rules = action?.config?.rules ?? [];
  const warnings = analyzeSessionConfigCarryForward(steps, step.id);
  const choices = useMemo<AgentChoice[]>(() => buildAgentChoices(profiles), [profiles]);
  const availableChoices = choices.filter(
    (choice) => !rules.some((rule) => rule.agent_name === choice.name),
  );
  const isDirty = isWorkflowStepValueDirty(step, savedStep, (item) =>
    JSON.stringify(configureSessionAction(item)?.config?.rules ?? []),
  );
  const [pendingRuleKeys, setPendingRuleKeys] = useState<Set<string>>(() => new Set());
  const updateRuleResolutionPending = useCallback((key: string, pending: boolean) => {
    setPendingRuleKeys((current) => {
      const next = new Set(current);
      if (pending) next.add(key);
      else next.delete(key);
      if (next.size === current.size && [...next].every((item) => current.has(item))) {
        return current;
      }
      return next;
    });
  }, []);

  useEffect(() => {
    onResolutionPendingChange?.(pendingRuleKeys.size > 0);
  }, [onResolutionPendingChange, pendingRuleKeys]);

  const updateRules = (nextRules: ConfigureSessionRule[]) => {
    onUpdate(withConfigureSessionRules(step, nextRules));
  };

  const addRule = (agentName?: string, operation: ConfigureSessionOperation = "set") => {
    const selectedAgent =
      availableChoices.find((choice) => choice.name === agentName)?.name ??
      availableChoices[0]?.name ??
      "";
    if (!selectedAgent) return;
    updateRules([
      ...rules,
      {
        agent_name: selectedAgent,
        operation,
        ...(operation === "set"
          ? { model: defaultModelForAgent(selectedAgent, availableAgents.items) }
          : {}),
      },
    ]);
  };

  const createCarryRule = (
    warning: SessionConfigCarryWarning,
    operation: ConfigureSessionOperation,
  ) => {
    if (rules.some((rule) => rule.agent_name === warning.agentName)) return;
    addRule(warning.agentName, operation);
  };

  if (!action && warnings.length === 0) return null;

  return (
    <section
      className="space-y-3 rounded-md border border-border/70 bg-muted/20 p-3"
      data-testid={`${step.id}-session-config-editor`}
      data-settings-dirty={isDirty}
    >
      {action && (
        <SessionConfigOptionsHeader
          step={step}
          isDirty={isDirty}
          readOnly={readOnly}
          onAddRule={() => addRule()}
          canAddRule={availableChoices.length > 0}
        />
      )}
      <SessionConfigBody
        step={step}
        action={action}
        rules={rules}
        warnings={warnings}
        choices={choices}
        availableAgents={availableAgents.items}
        readOnly={readOnly}
        fixedProfile={!!step.agent_profile_id}
        onChooseCarryRule={createCarryRule}
        onUpdateRules={updateRules}
        onResolutionPendingChange={updateRuleResolutionPending}
      />
    </section>
  );
}

function SessionConfigBody({
  step,
  action,
  rules,
  warnings,
  choices,
  availableAgents,
  readOnly,
  fixedProfile,
  onChooseCarryRule,
  onUpdateRules,
  onResolutionPendingChange,
}: {
  step: WorkflowStep;
  action: ReturnType<typeof configureSessionAction>;
  rules: ConfigureSessionRule[];
  warnings: SessionConfigCarryWarning[];
  choices: AgentChoice[];
  availableAgents: ReturnType<typeof useAvailableAgents>["items"];
  readOnly: boolean;
  fixedProfile: boolean;
  onChooseCarryRule: (
    warning: SessionConfigCarryWarning,
    operation: ConfigureSessionOperation,
  ) => void;
  onUpdateRules: (rules: ConfigureSessionRule[]) => void;
  onResolutionPendingChange: (key: string, pending: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {step.agent_profile_id && action && (
        <p className="rounded-md border border-amber-500/30 bg-amber-500/10 p-2 text-xs text-amber-200">
          {t("workflows:sessionConfigFixedProfileWarning")}
        </p>
      )}
      {!action && warnings.length > 0 && !step.agent_profile_id && (
        <SessionConfigCarryWarningPanel
          warnings={warnings}
          onChoose={onChooseCarryRule}
          readOnly={readOnly}
          disabled={fixedProfile}
        />
      )}
      {action && (
        <SessionConfigRuleList
          rules={rules}
          warnings={warnings}
          choices={choices}
          availableAgents={availableAgents}
          readOnly={readOnly || fixedProfile}
          fixedProfile={fixedProfile}
          onChooseCarryRule={onChooseCarryRule}
          onUpdateRules={onUpdateRules}
          onResolutionPendingChange={onResolutionPendingChange}
        />
      )}
    </>
  );
}

function SessionConfigRuleList({
  rules,
  warnings,
  choices,
  availableAgents,
  readOnly,
  fixedProfile,
  onChooseCarryRule,
  onUpdateRules,
  onResolutionPendingChange,
}: {
  rules: ConfigureSessionRule[];
  warnings: SessionConfigCarryWarning[];
  choices: AgentChoice[];
  availableAgents: ReturnType<typeof useAvailableAgents>["items"];
  readOnly: boolean;
  fixedProfile: boolean;
  onChooseCarryRule: (
    warning: SessionConfigCarryWarning,
    operation: ConfigureSessionOperation,
  ) => void;
  onUpdateRules: (rules: ConfigureSessionRule[]) => void;
  onResolutionPendingChange: (key: string, pending: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      {warnings.length > 0 && (
        <SessionConfigCarryWarningPanel
          warnings={warnings}
          onChoose={onChooseCarryRule}
          readOnly={readOnly}
          disabled={fixedProfile}
        />
      )}
      {rules.map((rule, index) => (
        <SessionConfigRuleCard
          key={`${rule.agent_name}-${index}`}
          rule={rule}
          index={index}
          choices={choices.filter(
            (choice) =>
              choice.name === rule.agent_name ||
              !rules.some(
                (candidate, candidateIndex) =>
                  candidateIndex !== index && candidate.agent_name === choice.name,
              ),
          )}
          availableAgents={availableAgents}
          readOnly={readOnly}
          onResolutionPendingChange={onResolutionPendingChange}
          onChange={(nextRule) => {
            if (
              nextRule.agent_name !== rule.agent_name &&
              rules.some(
                (candidate, candidateIndex) =>
                  candidateIndex !== index && candidate.agent_name === nextRule.agent_name,
              )
            ) {
              return;
            }
            const nextRules = rules.slice();
            nextRules[index] = nextRule;
            onUpdateRules(nextRules);
          }}
          onRemove={() =>
            onUpdateRules(rules.filter((_, candidateIndex) => candidateIndex !== index))
          }
        />
      ))}
      {rules.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("workflows:sessionConfigEmptyRules")}</p>
      )}
    </div>
  );
}

function SessionConfigOptionsHeader({
  step,
  isDirty,
  readOnly,
  onAddRule,
  canAddRule,
}: {
  step: WorkflowStep;
  isDirty: boolean;
  readOnly: boolean;
  onAddRule: () => void;
  canAddRule: boolean;
}) {
  const { t } = useTranslation();
  const disabled = readOnly || !!step.agent_profile_id;
  return (
    <div className="flex flex-wrap items-start justify-between gap-3" data-settings-dirty={isDirty}>
      <div className="flex min-w-0 items-start gap-2">
        <div className="min-w-0">
          <Label className="text-sm font-medium">
            {t("workflows:configureOriginalSessionOptions")}
          </Label>
          <p className="text-xs text-muted-foreground">
            {t("workflows:configureOriginalSessionOptionsDescription")}
          </p>
        </div>
        <HelpTip text={t("workflows:sessionConfigRulesHelp")} />
      </div>
      {!readOnly && !step.agent_profile_id && (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="min-h-10 cursor-pointer"
          onClick={onAddRule}
          disabled={disabled || !canAddRule}
          data-testid={`${step.id}-add-session-config-rule`}
        >
          <IconPlus className="mr-1.5 h-4 w-4" />
          {t("workflows:addCondition")}
        </Button>
      )}
    </div>
  );
}
