import { useCallback, useEffect, useRef } from "react";
import { useAgentCapabilities, useResolvedModelConfig } from "./use-dynamic-models";
import type { ModelConfig } from "@/lib/types/http";
import { reconcileConfigOptionValues } from "@/components/settings/profile-model-config";

type ProfileModelSelection = {
  model: string;
  mode: string;
  config_options?: Record<string, string>;
};

export function useProfileModelCapabilities(
  agentName: string,
  profile: ProfileModelSelection,
  modelConfig: ModelConfig,
  onChange?: (patch: { config_options: Record<string, string> }) => void,
) {
  const capabilities = useAgentCapabilities(agentName, modelConfig);
  const selectedModel =
    profile.model ||
    capabilities.currentModelId ||
    modelConfig.current_model_id ||
    modelConfig.default_model;
  const initialProfileModel = useRef(profile.model);
  const hasUserSelectedModel = useRef(false);
  const resolvedModelConfig = useResolvedModelConfig(agentName, selectedModel, {
    initialConfigOptions: modelConfig.config_options,
    enabled: modelConfig.supports_dynamic_models,
  });

  useEffect(() => {
    if (profile.model !== initialProfileModel.current) {
      hasUserSelectedModel.current = true;
    }
  }, [profile.model]);

  useEffect(() => {
    if (
      !onChange ||
      !hasUserSelectedModel.current ||
      resolvedModelConfig.status !== "ok" ||
      !resolvedModelConfig.isResolvedForRequest
    ) {
      return;
    }
    const nextConfigOptions = reconcileConfigOptionValues(
      profile.config_options,
      resolvedModelConfig.configOptions,
    );
    if (JSON.stringify(nextConfigOptions) === JSON.stringify(profile.config_options ?? {})) {
      return;
    }
    onChange({ config_options: nextConfigOptions });
  }, [
    onChange,
    profile.config_options,
    resolvedModelConfig.configOptions,
    resolvedModelConfig.isResolvedForRequest,
    resolvedModelConfig.status,
  ]);

  const refresh = useCallback(async () => {
    await Promise.all([capabilities.refresh(), resolvedModelConfig.refresh()]);
  }, [capabilities.refresh, resolvedModelConfig.refresh]);

  return {
    capabilities,
    configOptions: resolvedModelConfig.configOptions,
    configStatus: resolvedModelConfig.status,
    configError: resolvedModelConfig.error,
    configIsLoading: resolvedModelConfig.isLoading,
    refreshModelConfig: resolvedModelConfig.refresh,
    refresh,
  };
}
