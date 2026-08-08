"use client";

import { useEffect, useRef } from "react";
import { IconTrash } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  configOptionToModelOptions,
  isModelConfigOption,
  ModelConfigSelector,
  type SelectConfigOption,
} from "@/components/model-config-selector";
import { ModelConfigResolutionStatus } from "@/components/settings/model-config-resolution-status";
import type { AvailableAgent } from "@/lib/types/http";
import type { ConfigureSessionOperation, ConfigureSessionRule } from "@/lib/types/workflow-actions";
import { reconcileConfigOptionValues } from "./profile-model-config";
import { useSessionConfigModelOptions } from "./use-session-config-model-options";
import {
  type AgentChoice,
  defaultModelForAgent,
  operationRule,
} from "./workflow-session-config-shared";

function modelSelectionForRule(
  rule: ConfigureSessionRule,
  configOptions: SelectConfigOption[],
  selectedAgent: AvailableAgent | undefined,
) {
  const modelConfigOption = configOptions.find(isModelConfigOption);
  const modelOptions = modelConfigOption
    ? configOptionToModelOptions(modelConfigOption)
    : (selectedAgent?.model_config?.available_models ?? []).map((model) => ({
        id: model.id,
        name: model.name,
        description: model.description ?? (model.id !== model.name ? model.id : undefined),
        usageMultiplier:
          typeof model.meta?.copilotUsage === "string" ? model.meta.copilotUsage : undefined,
      }));

  return {
    modelOptions,
    currentModel: currentModelForRule(rule, modelConfigOption, selectedAgent),
  };
}

export function SessionConfigRuleCard({
  rule,
  index,
  choices,
  availableAgents,
  readOnly,
  onChange,
  onRemove,
  onResolutionPendingChange,
}: {
  rule: ConfigureSessionRule;
  index: number;
  choices: AgentChoice[];
  availableAgents: AvailableAgent[];
  readOnly: boolean;
  onChange: (rule: ConfigureSessionRule) => void;
  onRemove: () => void;
  onResolutionPendingChange: (key: string, pending: boolean) => void;
}) {
  const selectedAgent = availableAgents.find((agent) => agent.name === rule.agent_name);
  const {
    configOptions,
    configStatus,
    configError,
    configIsLoading,
    isResolutionPending,
    refreshConfig,
    modelOptions,
    currentModel,
    handleRuleChange,
  } = useSessionConfigRuleModelState({
    rule,
    index,
    selectedAgent,
    readOnly,
    onChange,
    onResolutionPendingChange,
  });

  return (
    <div
      className="space-y-3 rounded-md border bg-card p-3"
      data-testid={`session-config-rule-${index}`}
    >
      <SessionConfigRuleHeader
        rule={rule}
        index={index}
        choices={choices}
        availableAgents={availableAgents}
        readOnly={readOnly}
        onChange={onChange}
        onRemove={onRemove}
      />

      {rule.operation === "set" && (
        <SessionConfigRuleSettings
          rule={rule}
          modelOptions={modelOptions}
          currentModel={currentModel}
          configOptions={configOptions}
          configStatus={configStatus}
          configError={configError}
          configIsLoading={configIsLoading}
          isConfigResolutionPending={isResolutionPending}
          onRetryConfig={refreshConfig}
          readOnly={readOnly}
          onChange={handleRuleChange}
        />
      )}
    </div>
  );
}

function useSessionConfigRuleModelState({
  rule,
  index,
  selectedAgent,
  readOnly,
  onChange,
  onResolutionPendingChange,
}: {
  rule: ConfigureSessionRule;
  index: number;
  selectedAgent: AvailableAgent | undefined;
  readOnly: boolean;
  onChange: (rule: ConfigureSessionRule) => void;
  onResolutionPendingChange: (key: string, pending: boolean) => void;
}) {
  const {
    configOptions,
    configStatus,
    configError,
    configIsLoading,
    isConfigResolutionPending,
    refreshConfig,
    isConfigResolvedForRequest,
    resolvedConfigOptions,
  } = useSessionConfigModelOptions(selectedAgent, rule);
  const selectedRuleModel = rule.operation === "set" ? rule.model : undefined;
  const initialRuleModel = useRef(selectedRuleModel);
  const hasUserSelectedModel = useRef(false);
  const pendingKey = `${rule.agent_name}:${index}`;
  const currentConfigOptions = rule.operation === "set" ? rule.config_options : undefined;

  const reconciledConfigOptions = reconcileConfigOptionValues(
    currentConfigOptions,
    resolvedConfigOptions,
  );
  const needsReconciliation =
    !readOnly &&
    rule.operation === "set" &&
    configStatus === "ok" &&
    isConfigResolvedForRequest &&
    JSON.stringify(reconciledConfigOptions) !== JSON.stringify(currentConfigOptions ?? {});
  const isResolutionPending = isConfigResolutionPending || needsReconciliation;

  useEffect(() => {
    onResolutionPendingChange(pendingKey, isResolutionPending);
    return () => onResolutionPendingChange(pendingKey, false);
  }, [isResolutionPending, onResolutionPendingChange, pendingKey]);

  useEffect(() => {
    if (selectedRuleModel !== initialRuleModel.current) {
      hasUserSelectedModel.current = true;
    }
  }, [selectedRuleModel]);

  useEffect(() => {
    if (
      readOnly ||
      !hasUserSelectedModel.current ||
      rule.operation !== "set" ||
      configStatus !== "ok" ||
      !isConfigResolvedForRequest
    ) {
      return;
    }
    const nextConfigOptions = reconciledConfigOptions;
    if (JSON.stringify(nextConfigOptions) === JSON.stringify(currentConfigOptions ?? {})) {
      return;
    }
    const nextRule = { ...rule };
    if (Object.keys(nextConfigOptions).length > 0) {
      nextRule.config_options = nextConfigOptions;
    } else {
      delete nextRule.config_options;
    }
    onChange(nextRule);
  }, [configStatus, isConfigResolvedForRequest, onChange, readOnly, reconciledConfigOptions, rule]);

  const { modelOptions, currentModel } = modelSelectionForRule(rule, configOptions, selectedAgent);
  const handleRuleChange = (nextRule: ConfigureSessionRule) => {
    const nextModel = nextRule.operation === "set" ? nextRule.model : undefined;
    if (nextModel !== selectedRuleModel) {
      hasUserSelectedModel.current = true;
    }
    onChange(nextRule);
  };

  return {
    configOptions,
    configStatus,
    configError,
    configIsLoading,
    isResolutionPending,
    refreshConfig,
    modelOptions,
    currentModel,
    handleRuleChange,
  };
}

function SessionConfigRuleHeader({
  rule,
  index,
  choices,
  availableAgents,
  readOnly,
  onChange,
  onRemove,
}: {
  rule: ConfigureSessionRule;
  index: number;
  choices: AgentChoice[];
  availableAgents: AvailableAgent[];
  readOnly: boolean;
  onChange: (rule: ConfigureSessionRule) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
      <Label className="text-xs font-medium text-muted-foreground sm:w-24">
        {t("workflows:whenAgentIs")}
      </Label>
      <Select
        value={rule.agent_name}
        onValueChange={(agentName) =>
          onChange({
            agent_name: agentName,
            operation: rule.operation,
            ...(rule.operation === "set"
              ? { model: defaultModelForAgent(agentName, availableAgents) }
              : {}),
          })
        }
        disabled={readOnly}
      >
        <SelectTrigger
          className="min-h-10 w-full cursor-pointer sm:flex-1"
          data-testid={`session-config-agent-${index}`}
        >
          <SelectValue placeholder={t("workflows:chooseAgentFamily")} />
        </SelectTrigger>
        <SelectContent>
          {choices.map((choice) => (
            <SelectItem key={choice.name} value={choice.name}>
              {choice.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={rule.operation}
        onValueChange={(operation) =>
          onChange(operationRule(rule, operation as ConfigureSessionOperation, availableAgents))
        }
        disabled={readOnly}
      >
        <SelectTrigger
          className="min-h-10 w-full cursor-pointer sm:w-44"
          data-testid={`session-config-operation-${index}`}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="set">{t("workflows:setSettings")}</SelectItem>
          <SelectItem value="keep">{t("workflows:keepCurrent")}</SelectItem>
          <SelectItem value="restore_original">{t("workflows:restoreOriginal")}</SelectItem>
        </SelectContent>
      </Select>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="min-h-10 cursor-pointer self-end text-destructive hover:text-destructive sm:self-auto"
        onClick={onRemove}
        disabled={readOnly}
        aria-label={t("workflows:removeAgentCondition", { index: index + 1 })}
      >
        <IconTrash className="h-4 w-4" />
      </Button>
    </div>
  );
}

function currentModelForRule(
  rule: ConfigureSessionRule,
  modelConfigOption: SelectConfigOption | undefined,
  selectedAgent: AvailableAgent | undefined,
): string | null {
  return (
    (rule.operation === "set" ? rule.model : undefined) ||
    modelConfigOption?.currentValue ||
    selectedAgent?.model_config?.current_model_id ||
    selectedAgent?.model_config?.default_model ||
    null
  );
}

function SessionConfigRuleSettings({
  rule,
  modelOptions,
  currentModel,
  configOptions,
  configStatus,
  configError,
  configIsLoading,
  isConfigResolutionPending,
  onRetryConfig,
  readOnly,
  onChange,
}: {
  rule: Extract<ConfigureSessionRule, { operation: "set" }>;
  modelOptions: { id: string; name: string; description?: string; usageMultiplier?: string }[];
  currentModel: string | null;
  configOptions: SelectConfigOption[];
  configStatus: ReturnType<typeof useSessionConfigModelOptions>["configStatus"];
  configError: string | null;
  configIsLoading: boolean;
  isConfigResolutionPending: boolean;
  onRetryConfig: () => Promise<void>;
  readOnly: boolean;
  onChange: (rule: ConfigureSessionRule) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 border-t border-border/60 pt-3">
      {modelOptions.length > 0 || configOptions.length > 0 ? (
        <ModelConfigSelector
          modelOptions={modelOptions}
          currentModel={currentModel}
          configOptions={configOptions}
          onModelChange={(model) => onChange({ ...rule, model })}
          onConfigChange={(configId, value) =>
            onChange({
              ...rule,
              config_options: { ...(rule.config_options ?? {}), [configId]: value },
            })
          }
          disabled={readOnly}
          placeholder={t("workflows:chooseModelAndSessionSettings")}
          ariaLabel={t("workflows:settingsForAgent", { agent: rule.agent_name })}
          triggerClassName="min-h-11 w-full sm:min-h-10"
        />
      ) : (
        <p className="rounded-md border border-border/60 p-2 text-xs text-muted-foreground">
          {t("workflows:sessionConfigModelOptionsUnavailable")}
        </p>
      )}
      <ModelConfigResolutionStatus
        status={configStatus}
        error={configError}
        isLoading={configIsLoading || isConfigResolutionPending}
        onRetry={onRetryConfig}
      />
    </div>
  );
}
