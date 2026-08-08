import { useResolvedModelConfig } from "@/hooks/domains/settings/use-dynamic-models";
import type { AvailableAgent } from "@/lib/types/http";
import type { ConfigureSessionRule } from "@/lib/types/workflow-actions";
import { modelConfigOptions } from "./workflow-session-config-shared";

function selectedModelForRule(
  selectedAgent: AvailableAgent | undefined,
  rule: ConfigureSessionRule,
): string | undefined {
  if (rule.operation !== "set") return undefined;
  return (
    rule.model ||
    selectedAgent?.model_config?.current_model_id ||
    selectedAgent?.model_config?.default_model ||
    selectedAgent?.model_config?.available_models?.[0]?.id
  );
}

function modelResolutionEnabled(
  selectedAgent: AvailableAgent | undefined,
  rule: ConfigureSessionRule,
): boolean {
  return rule.operation === "set" && !!selectedAgent?.model_config?.supports_dynamic_models;
}

function modelConfigWithResolvedOptions(
  selectedAgent: AvailableAgent | undefined,
  configOptions: ReturnType<typeof useResolvedModelConfig>["configOptions"],
) {
  if (!selectedAgent?.model_config) return undefined;
  return { ...selectedAgent.model_config, config_options: configOptions };
}

export function useSessionConfigModelOptions(
  selectedAgent: AvailableAgent | undefined,
  rule: ConfigureSessionRule,
) {
  const selectedModel = selectedModelForRule(selectedAgent, rule);
  const resolvedModelConfig = useResolvedModelConfig(selectedAgent?.name, selectedModel, {
    initialConfigOptions: selectedAgent?.model_config?.config_options,
    enabled: modelResolutionEnabled(selectedAgent, rule),
  });

  return {
    configOptions: modelConfigOptions(
      modelConfigWithResolvedOptions(selectedAgent, resolvedModelConfig.configOptions),
      rule,
    ),
    configStatus: resolvedModelConfig.status,
    configError: resolvedModelConfig.error,
    configIsLoading: resolvedModelConfig.isLoading,
    isConfigResolutionPending: resolvedModelConfig.isResolutionPending,
    refreshConfig: resolvedModelConfig.refresh,
    isConfigResolvedForRequest: resolvedModelConfig.isResolvedForRequest,
    resolvedConfigOptions: resolvedModelConfig.configOptions,
  };
}
