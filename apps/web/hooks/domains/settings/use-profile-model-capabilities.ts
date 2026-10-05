import { useMemo } from "react";
import { useProfileModelOptions } from "./use-profile-model-options";
import {
  canonicalLaunchSettings,
  profileContextKey,
  responseCommands,
  responseModes,
  responseModels,
  useProfileCapabilityDiscovery,
  type ProfileCapabilityDiscoveryOptions,
  type ProfileCapabilityState,
  type ProfileDiscoveryStatus,
  type ProfileModelSelection,
} from "./use-profile-capability-discovery";
import type { DynamicModelsResponse, ModelConfig } from "@/lib/types/http";

export type {
  ProfileModelSelection,
  ProfileCapabilityDiscoveryOptions as ProfileModelCapabilitiesOptions,
  ProfileDiscoveryStatus,
  ProfileCapabilityState,
};

export {
  canonicalLaunchSettings,
  profileContextKey,
  responseCommands,
  responseModes,
  responseModels,
  useProfileCapabilityDiscovery,
};

export function useProfileModelCapabilities(
  agentName: string,
  profile: ProfileModelSelection,
  modelConfig: ModelConfig,
  onChange?: (patch: { config_options: Record<string, string> }) => void,
  options: ProfileCapabilityDiscoveryOptions = {},
) {
  const isStaticContext = options.skipCapabilityProbe || !modelConfig.supports_dynamic_models;
  const launchSettings = useMemo(
    () => canonicalLaunchSettings(profile),
    [profile.env_vars, profile.cli_flags, profile.command_prefix],
  );
  const currentLaunchKey = profileContextKey(agentName, options.profileId, launchSettings);
  const profileIdentity = `${agentName}:${options.profileId ?? "draft"}`;
  const discovery = useProfileCapabilityDiscovery(agentName, profile, {
    ...options,
    supportsDynamicModels: modelConfig.supports_dynamic_models,
  });
  const modelOptions = useProfileModelOptions({
    agentName,
    profile,
    modelConfig,
    launchKey: currentLaunchKey,
    profileIdentity,
    activeCapability: discovery.activeCapability,
    isStaticContext,
    onChange,
    markCapabilityFailed: discovery.markCapabilityFailed,
  });
  const capability = discovery.activeCapability?.response;
  const capabilities = buildProfileCapabilities({
    modelConfig,
    isStaticContext,
    capability,
    discoveryState: discovery.discoveryState,
    error: discovery.error,
    refresh: discovery.refresh,
    configIsLoading: modelOptions.configIsLoading,
  });

  return {
    capabilities,
    discoveryState: discovery.discoveryState,
    configOptions: modelOptions.configOptions,
    configStatus: modelOptions.configStatus,
    configError: modelOptions.configError,
    configIsLoading: modelOptions.configIsLoading,
    isConfigResolutionPending: modelOptions.isConfigResolutionPending,
    refreshModelConfig: modelOptions.refreshModelConfig,
    refresh: discovery.refresh,
  };
}

type BuildProfileCapabilitiesInput = {
  modelConfig: ModelConfig;
  isStaticContext: boolean | undefined;
  capability: DynamicModelsResponse | undefined;
  discoveryState: ProfileDiscoveryStatus;
  error: string | null;
  refresh: () => Promise<void>;
  configIsLoading: boolean;
};

function buildProfileCapabilities(input: BuildProfileCapabilitiesInput) {
  if (input.isStaticContext) return buildStaticProfileCapabilities(input);
  return buildDynamicProfileCapabilities(input);
}

function buildStaticProfileCapabilities({
  modelConfig,
  error,
  refresh,
  configIsLoading,
  discoveryState,
}: BuildProfileCapabilitiesInput) {
  return {
    models: modelConfig.available_models,
    modes: modelConfig.available_modes ?? [],
    commands: modelConfig.available_commands ?? [],
    currentModelId: modelConfig.current_model_id,
    currentModeId: modelConfig.current_mode_id,
    status: modelConfig.status ?? "ok",
    isLoading: discoveryState === "loading" || configIsLoading,
    error,
    refresh,
  };
}

function buildDynamicProfileCapabilities({
  capability,
  discoveryState,
  error,
  refresh,
  configIsLoading,
}: BuildProfileCapabilitiesInput) {
  let status = capability?.status;
  if (!status && discoveryState === "loading") status = "probing";
  return {
    models: responseModels(capability),
    modes: responseModes(capability),
    commands: responseCommands(capability),
    currentModelId: capability?.current_model_id,
    currentModeId: capability?.current_mode_id,
    status,
    isLoading: discoveryState === "loading" || configIsLoading,
    error,
    refresh,
  };
}
