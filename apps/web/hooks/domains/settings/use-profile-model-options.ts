import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { resolveAgentModelConfig } from "@/lib/api/domains/settings-api";
import { t } from "@/lib/i18n";
import { reconcileConfigOptionValues } from "@/components/settings/profile-model-config";
import type {
  AgentModelConfigResponse,
  CapabilityStatus,
  ConfigOptionEntry,
  ModelConfig,
  ResolveAgentModelConfigRequest,
} from "@/lib/types/http";
import type {
  ProfileCapabilityState,
  ProfileModelSelection,
} from "./use-profile-model-capabilities";

const CAPABILITY_READY = "ready" as const;

type ModelOptionsState = {
  requestKey: string;
  status: CapabilityStatus | "failed";
  options: ConfigOptionEntry[];
  error: string | null;
  isLoading: boolean;
};

type ModelOptionsInput = {
  agentName: string;
  profile: ProfileModelSelection;
  modelConfig: ModelConfig;
  launchKey: string;
  profileIdentity: string;
  activeCapability: ProfileCapabilityState | null;
  isStaticContext: boolean | undefined;
  onChange?: (patch: { config_options: Record<string, string> }) => void;
  markCapabilityFailed: (launchKey: string) => void;
};

export function useProfileModelOptions({
  agentName,
  profile,
  modelConfig,
  launchKey,
  profileIdentity,
  activeCapability,
  isStaticContext,
  onChange,
  markCapabilityFailed,
}: ModelOptionsInput) {
  const [optionState, setOptionState] = useState<ModelOptionsState | null>(null);
  const hasUserSelectedModel = useProfileModelSelectionState(
    profileIdentity,
    profile.model,
    setOptionState,
  );
  const selectedModel = getSelectedModel(profile, activeCapability, modelConfig);
  const resolution = useResolvedModelOptions({
    agentName,
    profile,
    launchKey,
    selectedModel,
    activeCapability,
    isStaticContext,
    optionState,
    setOptionState,
    markCapabilityFailed,
  });
  const configOptions = getConfigOptions(
    isStaticContext,
    modelConfig,
    resolution.matchingOptionState,
  );
  const needsReconciliation = shouldReconcileOptions(
    onChange,
    hasUserSelectedModel,
    resolution.matchingOptionState,
    profile,
    configOptions,
  );
  const nextConfigOptions = needsReconciliation
    ? reconcileConfigOptionValues(profile.config_options, configOptions)
    : (profile.config_options ?? {});
  useEffect(() => {
    if (onChange && needsReconciliation) onChange({ config_options: nextConfigOptions });
  }, [needsReconciliation, nextConfigOptions, onChange]);

  const isConfigResolutionPending = Boolean(
    onChange &&
    hasUserSelectedModel &&
    activeCapability &&
    resolution.requestKey &&
    (!resolution.matchingOptionState ||
      resolution.matchingOptionState.isLoading ||
      needsReconciliation),
  );

  return {
    configOptions,
    configStatus: isStaticContext ? "ok" : resolution.matchingOptionState?.status,
    configError: resolution.matchingOptionState?.error ?? null,
    configIsLoading: Boolean(resolution.matchingOptionState?.isLoading),
    isConfigResolutionPending,
    refreshModelConfig: resolution.refreshModelConfig,
  };
}

function getConfigOptions(
  isStaticContext: boolean | undefined,
  modelConfig: ModelConfig,
  matchingOptionState: ModelOptionsState | null,
): ConfigOptionEntry[] {
  if (isStaticContext) return modelConfig.config_options ?? [];
  return matchingOptionState?.options ?? [];
}

type ModelOptionStateSetter = Dispatch<SetStateAction<ModelOptionsState | null>>;

function useProfileModelSelectionState(
  profileIdentity: string,
  selectedModel: string,
  setOptionState: ModelOptionStateSetter,
): boolean {
  const profileIdentityRef = useRef(profileIdentity);
  const initialProfileModel = useRef(selectedModel);
  const hasUserSelectedModel = useRef(false);

  useEffect(() => {
    if (profileIdentityRef.current === profileIdentity) return;
    profileIdentityRef.current = profileIdentity;
    initialProfileModel.current = selectedModel;
    hasUserSelectedModel.current = false;
    setOptionState(null);
  }, [profileIdentity, selectedModel, setOptionState]);

  useEffect(() => {
    if (selectedModel !== initialProfileModel.current) hasUserSelectedModel.current = true;
  }, [selectedModel]);

  return hasUserSelectedModel.current;
}

function getSelectedModel(
  profile: ProfileModelSelection,
  activeCapability: ProfileCapabilityState | null,
  modelConfig: ModelConfig,
): string {
  return (
    profile.model ||
    activeCapability?.response.current_model_id ||
    modelConfig.current_model_id ||
    modelConfig.default_model
  );
}

type ResolvedModelOptionsInput = {
  agentName: string;
  profile: ProfileModelSelection;
  launchKey: string;
  selectedModel: string;
  activeCapability: ProfileCapabilityState | null;
  isStaticContext: boolean | undefined;
  optionState: ModelOptionsState | null;
  setOptionState: ModelOptionStateSetter;
  markCapabilityFailed: (launchKey: string) => void;
};

function useResolvedModelOptions({
  agentName,
  profile,
  launchKey,
  selectedModel,
  activeCapability,
  isStaticContext,
  optionState,
  setOptionState,
  markCapabilityFailed,
}: ResolvedModelOptionsInput) {
  const optionSequence = useRef(0);
  const requestKeyRef = useRef<string | undefined>(undefined);
  const currentLaunchKeyRef = useRef(launchKey);
  currentLaunchKeyRef.current = launchKey;
  const configOptionsKey = JSON.stringify(
    Object.entries(profile.config_options ?? {}).sort(([left], [right]) =>
      left.localeCompare(right),
    ),
  );
  const stableConfigOptions = useMemo(() => {
    const entries = JSON.parse(configOptionsKey) as [string, string][];
    return entries.length > 0 ? Object.fromEntries(entries) : undefined;
  }, [configOptionsKey]);
  const requestKey = buildOptionsRequestKey(launchKey, activeCapability, selectedModel, profile);
  requestKeyRef.current = requestKey;

  const runModelOptions = useCallback(
    (context: ProfileCapabilityState, key: string) =>
      storeResolvedModelOptions({
        agentName,
        mode: profile.mode,
        configOptions: stableConfigOptions,
        selectedModel,
        context,
        key,
        optionSequence,
        requestKeyRef,
        currentLaunchKeyRef,
        setOptionState,
        markCapabilityFailed,
      }),
    [
      agentName,
      markCapabilityFailed,
      profile.mode,
      selectedModel,
      setOptionState,
      stableConfigOptions,
    ],
  );

  useEffect(() => {
    if (
      isStaticContext ||
      !activeCapability ||
      activeCapability.status !== CAPABILITY_READY ||
      !requestKey
    )
      return;
    void runModelOptions(activeCapability, requestKey);
    return () => {
      optionSequence.current += 1;
    };
  }, [activeCapability, isStaticContext, requestKey, runModelOptions]);

  const matchingOptionState =
    requestKey && optionState?.requestKey === requestKey ? optionState : null;
  const refreshModelConfig = useCallback(async () => {
    if (!isStaticContext && activeCapability && requestKey) {
      await runModelOptions(activeCapability, requestKey);
    }
  }, [activeCapability, isStaticContext, requestKey, runModelOptions]);

  return { requestKey, matchingOptionState, refreshModelConfig };
}

function buildOptionsRequestKey(
  launchKey: string,
  activeCapability: ProfileCapabilityState | null,
  selectedModel: string,
  profile: ProfileModelSelection,
): string | undefined {
  if (!activeCapability || activeCapability.status !== CAPABILITY_READY || !selectedModel)
    return undefined;
  return JSON.stringify([
    launchKey,
    activeCapability.response.context_revision ?? "",
    selectedModel,
    profile.mode,
    Object.entries(profile.config_options ?? {}).sort(([left], [right]) =>
      left.localeCompare(right),
    ),
  ]);
}

type CapabilitySequenceRef = { current: number };
type CapabilityLaunchKeyRef = { current: string };

type StoreResolvedModelOptionsInput = {
  agentName: string;
  mode: string;
  configOptions: Record<string, string> | undefined;
  selectedModel: string;
  context: ProfileCapabilityState;
  key: string;
  optionSequence: CapabilitySequenceRef;
  requestKeyRef: { current: string | undefined };
  currentLaunchKeyRef: CapabilityLaunchKeyRef;
  setOptionState: ModelOptionStateSetter;
  markCapabilityFailed: (launchKey: string) => void;
};

async function storeResolvedModelOptions({
  agentName,
  mode,
  configOptions,
  selectedModel,
  context,
  key,
  optionSequence,
  requestKeyRef,
  currentLaunchKeyRef,
  setOptionState,
  markCapabilityFailed,
}: StoreResolvedModelOptionsInput): Promise<void> {
  if (!agentName || !selectedModel) return;
  const sequence = ++optionSequence.current;
  const request = optionRequest(selectedModel, mode, configOptions, context.resolveContext);
  setOptionState({ requestKey: key, status: "probing", options: [], error: null, isLoading: true });
  const result = await requestModelOptions(agentName, request);
  if (
    !isCurrentOptionRequest({
      sequence,
      currentSequence: optionSequence,
      requestKey: key,
      currentRequestKey: requestKeyRef,
      context,
      currentLaunchKey: currentLaunchKeyRef,
    })
  )
    return;
  if (result.error || !result.response) {
    setOptionState({
      requestKey: key,
      status: "failed",
      options: [],
      error: result.error ?? null,
      isLoading: false,
    });
    return;
  }
  if (contextRevisionMismatch(context, result.response)) {
    setOptionState({
      requestKey: key,
      status: "failed",
      options: [],
      error: t("agents:failedToFetchCapabilities"),
      isLoading: false,
    });
    markCapabilityFailed(context.launchKey);
    return;
  }
  const response = result.response;
  setOptionState({
    requestKey: key,
    status: response.status,
    options: response.status === "ok" ? (response.config_options ?? []) : [],
    error: response.error ?? null,
    isLoading: false,
  });
}

function optionRequest(
  model: string,
  mode: string,
  options: Record<string, string> | undefined,
  context: ProfileCapabilityState["resolveContext"],
): ResolveAgentModelConfigRequest {
  return {
    model,
    ...(mode ? { mode } : {}),
    ...(options && Object.keys(options).length > 0 ? { config_options: options } : {}),
    ...context,
  };
}

async function requestModelOptions(
  agentName: string,
  request: ResolveAgentModelConfigRequest,
): Promise<{ response?: AgentModelConfigResponse; error?: string }> {
  try {
    return { response: await resolveAgentModelConfig(agentName, request) };
  } catch (err) {
    return { error: err instanceof Error ? err.message : t("agents:failedToFetchCapabilities") };
  }
}

type IsCurrentOptionRequestInput = {
  sequence: number;
  currentSequence: CapabilitySequenceRef;
  requestKey: string;
  currentRequestKey: { current: string | undefined };
  context: ProfileCapabilityState;
  currentLaunchKey: CapabilityLaunchKeyRef;
};

function isCurrentOptionRequest({
  sequence,
  currentSequence,
  requestKey,
  currentRequestKey,
  context,
  currentLaunchKey,
}: IsCurrentOptionRequestInput): boolean {
  return (
    sequence === currentSequence.current &&
    currentRequestKey.current === requestKey &&
    context.launchKey === currentLaunchKey.current
  );
}

function contextRevisionMismatch(
  context: ProfileCapabilityState,
  response: AgentModelConfigResponse,
): boolean {
  return Boolean(
    context.response.context_revision &&
    response.context_revision &&
    context.response.context_revision !== response.context_revision,
  );
}

function shouldReconcileOptions(
  onChange: ModelOptionsInput["onChange"],
  hasUserSelectedModel: boolean,
  optionState: ModelOptionsState | null,
  profile: ProfileModelSelection,
  configOptions: ConfigOptionEntry[],
): boolean {
  if (!onChange || !hasUserSelectedModel || optionState?.status !== "ok") return false;
  const nextValues = reconcileConfigOptionValues(profile.config_options, configOptions);
  return JSON.stringify(nextValues) !== JSON.stringify(profile.config_options ?? {});
}
