import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { probeAgentProfile } from "@/lib/api/domains/profile-capability-api";
import { useProfileModelOptions } from "./use-profile-model-options";
import type {
  CapabilityStatus,
  CLIFlag,
  CommandEntry,
  DynamicModelsResponse,
  ModelConfig,
  ModeEntry,
  ModelEntry,
  ProfileCapabilityRequest,
  ProfileLaunchSettingsRequest,
} from "@/lib/types/http";
import { t } from "@/lib/i18n";

export type ProfileModelSelection = {
  model: string;
  mode: string;
  config_options?: Record<string, string>;
  env_vars?: ProfileLaunchSettingsRequest["env_vars"];
  cli_flags?: CLIFlag[];
  command_prefix?: string;
  provider_kind?: string;
};

type ProfileModelCapabilitiesOptions = {
  profileId?: string;
  savedLaunchSettings?: ProfileLaunchSettingsRequest;
  skipCapabilityProbe?: boolean;
};

export type ProfileDiscoveryStatus = "loading" | "ready" | "stale" | "failed";

const CAPABILITY_READY = "ready" as const;

export type ProfileCapabilityState = {
  launchKey: string;
  status: "loading" | "ready" | "failed";
  response: DynamicModelsResponse;
  resolveContext: Omit<ProfileCapabilityRequest, "refresh">;
};

function canonicalLaunchSettings(
  profile: Pick<ProfileModelSelection, "env_vars" | "cli_flags" | "command_prefix">,
): ProfileLaunchSettingsRequest {
  return {
    env_vars: [...(profile.env_vars ?? [])]
      .map((item) => ({ key: item.key, value: item.value, secret_id: item.secret_id }))
      .sort((left, right) => left.key.localeCompare(right.key)),
    cli_flags: (profile.cli_flags ?? []).map((item) => ({ ...item })),
    command_prefix: profile.command_prefix ?? "",
  };
}

function launchSettingsKey(settings: ProfileLaunchSettingsRequest): string {
  return JSON.stringify(settings);
}

function profileContextKey(
  agentName: string,
  profileId: string | undefined,
  settings: ProfileLaunchSettingsRequest,
) {
  return JSON.stringify([agentName, profileId ?? "draft", launchSettingsKey(settings)]);
}

function emptyResponse(agentName: string, status: CapabilityStatus): DynamicModelsResponse {
  return { agent_name: agentName, status, models: [], modes: [], commands: [], error: null };
}

function responseModels(response?: DynamicModelsResponse): ModelEntry[] {
  return response?.status === "ok" ? (response.models ?? []) : [];
}

function responseModes(response?: DynamicModelsResponse): ModeEntry[] {
  return response?.status === "ok" ? (response.modes ?? []) : [];
}

function responseCommands(response?: DynamicModelsResponse): CommandEntry[] {
  return response?.status === "ok" ? (response.commands ?? []) : [];
}

export function useProfileModelCapabilities(
  agentName: string,
  profile: ProfileModelSelection,
  modelConfig: ModelConfig,
  onChange?: (patch: { config_options: Record<string, string> }) => void,
  options: ProfileModelCapabilitiesOptions = {},
) {
  const isStaticContext = options.skipCapabilityProbe || !modelConfig.supports_dynamic_models;
  const launchSettings = useMemo(
    () => canonicalLaunchSettings(profile),
    [profile.env_vars, profile.cli_flags, profile.command_prefix],
  );
  const currentLaunchKey = profileContextKey(agentName, options.profileId, launchSettings);
  const profileIdentity = `${agentName}:${options.profileId ?? "draft"}`;
  const discovery = useProfileCapabilityDiscovery({
    agentName,
    launchSettings,
    launchKey: currentLaunchKey,
    isStaticContext,
    options,
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

type CapabilityDiscoveryInput = {
  agentName: string;
  launchSettings: ProfileLaunchSettingsRequest;
  launchKey: string;
  isStaticContext: boolean;
  options: ProfileModelCapabilitiesOptions;
};

function useProfileCapabilityDiscovery({
  agentName,
  launchSettings,
  launchKey,
  isStaticContext,
  options,
}: CapabilityDiscoveryInput) {
  const profileIdentity = `${agentName}:${options.profileId ?? "draft"}`;
  const profileIdentityRef = useRef(profileIdentity);
  const capabilitySequence = useRef(0);
  const currentLaunchKeyRef = useRef(launchKey);
  currentLaunchKeyRef.current = launchKey;
  const [capabilityState, setCapabilityState] = useState<ProfileCapabilityState | null>(null);
  const canAutoProbeSaved = isSavedLaunchSettingsCurrent(agentName, options, launchKey);

  useEffect(() => {
    if (profileIdentityRef.current === profileIdentity) return;
    profileIdentityRef.current = profileIdentity;
    setCapabilityState(null);
  }, [profileIdentity]);

  const commitCapabilityResponse = useCallback(
    (
      response: DynamicModelsResponse,
      responseLaunchKey: string,
      resolveContext: ProfileCapabilityState["resolveContext"],
    ) => {
      setCapabilityState({
        launchKey: responseLaunchKey,
        status: response.status === "ok" ? CAPABILITY_READY : "failed",
        response,
        resolveContext,
      });
    },
    [],
  );

  useSavedProfileCapabilityProbe({
    agentName,
    profileId: options.profileId,
    canAutoProbeSaved,
    launchKey,
    isStaticContext,
    capabilitySequence,
    currentLaunchKeyRef,
    setCapabilityState,
    commitCapabilityResponse,
  });

  const refresh = useProfileCapabilityRefresh({
    agentName,
    profileId: options.profileId,
    launchSettings,
    launchKey,
    isStaticContext,
    capabilitySequence,
    currentLaunchKeyRef,
    setCapabilityState,
    commitCapabilityResponse,
  });

  const activeCapability = capabilityState?.launchKey === launchKey ? capabilityState : null;
  const discoveryState: ProfileDiscoveryStatus = getDiscoveryState(
    isStaticContext,
    activeCapability,
    canAutoProbeSaved,
  );
  const markCapabilityFailed = useCallback((failedLaunchKey: string) => {
    setCapabilityState((current) =>
      current?.launchKey === failedLaunchKey ? { ...current, status: "failed" } : current,
    );
  }, []);

  return {
    activeCapability,
    discoveryState,
    error: discoveryState === "failed" ? (activeCapability?.response.error ?? null) : null,
    refresh,
    markCapabilityFailed,
  };
}

function capabilityRequestIsStale(
  requestSequence: number,
  currentSequence: number,
  currentLaunchKey: string,
  requestLaunchKey: string,
): boolean {
  return requestSequence !== currentSequence || currentLaunchKey !== requestLaunchKey;
}

type CapabilityResponseCommit = (
  response: DynamicModelsResponse,
  responseLaunchKey: string,
  resolveContext: ProfileCapabilityState["resolveContext"],
) => void;

type ProfileCapabilityRefreshInput = {
  agentName: string;
  profileId?: string;
  launchSettings: ProfileLaunchSettingsRequest;
  launchKey: string;
  isStaticContext: boolean;
  capabilitySequence: CapabilitySequenceRef;
  currentLaunchKeyRef: CapabilityLaunchKeyRef;
  setCapabilityState: CapabilityStateSetter;
  commitCapabilityResponse: CapabilityResponseCommit;
};

function useProfileCapabilityRefresh({
  agentName,
  profileId,
  launchSettings,
  launchKey,
  isStaticContext,
  capabilitySequence,
  currentLaunchKeyRef,
  setCapabilityState,
  commitCapabilityResponse,
}: ProfileCapabilityRefreshInput) {
  return useCallback(async () => {
    if (isStaticContext || !agentName) return;
    const sequence = ++capabilitySequence.current;
    const resolveContext: ProfileCapabilityState["resolveContext"] = {
      ...(profileId ? { profile_id: profileId } : {}),
      launch_settings: launchSettings,
    };
    setCapabilityState({
      launchKey,
      status: "loading",
      response: emptyResponse(agentName, "probing"),
      resolveContext,
    });
    try {
      const response = await probeAgentProfile(agentName, { ...resolveContext, refresh: true });
      if (
        capabilityRequestIsStale(
          sequence,
          capabilitySequence.current,
          currentLaunchKeyRef.current,
          launchKey,
        )
      )
        return;
      commitCapabilityResponse(response, launchKey, resolveContext);
    } catch (err) {
      if (
        capabilityRequestIsStale(
          sequence,
          capabilitySequence.current,
          currentLaunchKeyRef.current,
          launchKey,
        )
      )
        return;
      const error = err instanceof Error ? err.message : t("agents:failedToFetchCapabilities");
      setCapabilityState({
        launchKey,
        status: "failed",
        response: { ...emptyResponse(agentName, "failed"), error },
        resolveContext,
      });
    }
  }, [
    agentName,
    capabilitySequence,
    commitCapabilityResponse,
    currentLaunchKeyRef,
    isStaticContext,
    launchKey,
    launchSettings,
    profileId,
    setCapabilityState,
  ]);
}

type CapabilitySequenceRef = { current: number };
type CapabilityLaunchKeyRef = { current: string };
type CapabilityStateSetter = Dispatch<SetStateAction<ProfileCapabilityState | null>>;

type SavedProfileProbeInput = {
  agentName: string;
  profileId?: string;
  canAutoProbeSaved: boolean;
  launchKey: string;
  isStaticContext: boolean;
  capabilitySequence: CapabilitySequenceRef;
  currentLaunchKeyRef: CapabilityLaunchKeyRef;
  setCapabilityState: CapabilityStateSetter;
  commitCapabilityResponse: (
    response: DynamicModelsResponse,
    responseLaunchKey: string,
    resolveContext: ProfileCapabilityState["resolveContext"],
  ) => void;
};

function useSavedProfileCapabilityProbe({
  agentName,
  profileId,
  canAutoProbeSaved,
  launchKey,
  isStaticContext,
  capabilitySequence,
  currentLaunchKeyRef,
  setCapabilityState,
  commitCapabilityResponse,
}: SavedProfileProbeInput): void {
  useEffect(() => {
    if (isStaticContext || !agentName || !canAutoProbeSaved || !profileId) return;
    const sequence = ++capabilitySequence.current;
    const resolveContext = { profile_id: profileId };
    setCapabilityState({
      launchKey,
      status: "loading",
      response: emptyResponse(agentName, "probing"),
      resolveContext,
    });
    void probeAgentProfile(agentName, resolveContext)
      .then((response) => {
        if (
          !isCurrentCapabilityRequest(sequence, capabilitySequence, launchKey, currentLaunchKeyRef)
        )
          return;
        commitCapabilityResponse(response, launchKey, resolveContext);
      })
      .catch((err: unknown) => {
        if (
          !isCurrentCapabilityRequest(sequence, capabilitySequence, launchKey, currentLaunchKeyRef)
        )
          return;
        const error = err instanceof Error ? err.message : t("agents:failedToFetchCapabilities");
        setCapabilityState({
          launchKey,
          status: "failed",
          response: { ...emptyResponse(agentName, "failed"), error },
          resolveContext,
        });
      });
    return () => {
      capabilitySequence.current += 1;
    };
  }, [
    agentName,
    canAutoProbeSaved,
    capabilitySequence,
    commitCapabilityResponse,
    currentLaunchKeyRef,
    isStaticContext,
    launchKey,
    profileId,
    setCapabilityState,
  ]);
}

function isCurrentCapabilityRequest(
  sequence: number,
  currentSequence: CapabilitySequenceRef,
  launchKey: string,
  currentLaunchKey: CapabilityLaunchKeyRef,
): boolean {
  return sequence === currentSequence.current && currentLaunchKey.current === launchKey;
}

function isSavedLaunchSettingsCurrent(
  agentName: string,
  options: ProfileModelCapabilitiesOptions,
  launchKey: string,
): boolean {
  if (!options.profileId || !options.savedLaunchSettings) return false;
  const savedSettings = canonicalLaunchSettings(options.savedLaunchSettings);
  return profileContextKey(agentName, options.profileId, savedSettings) === launchKey;
}

function getDiscoveryState(
  isStaticContext: boolean | undefined,
  activeCapability: ProfileCapabilityState | null,
  canAutoProbeSaved: boolean,
): ProfileDiscoveryStatus {
  if (isStaticContext) return CAPABILITY_READY;
  if (activeCapability) return activeCapability.status;
  return canAutoProbeSaved ? "loading" : "stale";
}
