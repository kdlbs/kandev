import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { fetchDynamicModels, resolveAgentModelConfig } from "@/lib/api/domains/settings-api";
import type {
  AgentModelConfigResponse,
  CommandEntry,
  ConfigOptionEntry,
  ModeEntry,
  ModelEntry,
  CapabilityStatus,
  DynamicModelsResponse,
  ModelConfig,
  ResolveAgentModelConfigRequest,
} from "@/lib/types/http";
// Set from an async catch rather than rendered as a literal, so this uses the
// module-level `t`, which resolves at call time. The message surfaces in the
// profile editor through `agents:failedToRefresh`.
import { t } from "@/lib/i18n";

type UseAgentCapabilitiesState = {
  models: ModelEntry[];
  modes: ModeEntry[];
  commands: CommandEntry[];
  currentModelId: string | undefined;
  currentModeId: string | undefined;
  status: CapabilityStatus | undefined;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

type UseResolvedModelConfigOptions = {
  mode?: string;
  configOptions?: Record<string, string>;
  initialConfigOptions?: ConfigOptionEntry[];
  enabled?: boolean;
};

type UseResolvedModelConfigState = {
  configOptions: ConfigOptionEntry[];
  status: CapabilityStatus | undefined;
  isResolvedForRequest: boolean;
  isResolutionPending: boolean;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

type CachedModelConfigResolution = {
  response: AgentModelConfigResponse;
  expiresAt: number;
};

const MODEL_CONFIG_RESOLUTION_TTL_MS = 5 * 60 * 1000;
const CAPABILITY_PENDING_POLL_DELAY_MS = 250;
const CAPABILITY_PENDING_MAX_ATTEMPTS = 240;
const modelConfigResolutionCache = new Map<string, CachedModelConfigResolution>();
const modelConfigResolutionInflight = new Map<string, Promise<AgentModelConfigResponse>>();
let modelConfigResolutionGeneration = 0;

export function invalidateModelConfigResolutionCache(agentName?: string): void {
  modelConfigResolutionGeneration += 1;
  if (!agentName) {
    modelConfigResolutionCache.clear();
    modelConfigResolutionInflight.clear();
    return;
  }
  for (const key of modelConfigResolutionCache.keys()) {
    if (modelConfigResolutionKeyAgent(key) === agentName) {
      modelConfigResolutionCache.delete(key);
    }
  }
  for (const key of modelConfigResolutionInflight.keys()) {
    if (modelConfigResolutionKeyAgent(key) === agentName) {
      modelConfigResolutionInflight.delete(key);
    }
  }
}

export function __resetModelConfigResolutionCache(): void {
  modelConfigResolutionGeneration += 1;
  modelConfigResolutionCache.clear();
  modelConfigResolutionInflight.clear();
}

function modelConfigResolutionKeyAgent(key: string): string | undefined {
  try {
    const parsed = JSON.parse(key);
    return Array.isArray(parsed) && typeof parsed[0] === "string" ? parsed[0] : undefined;
  } catch {
    return undefined;
  }
}

function stableRecordKey(values: Record<string, string> | undefined): string {
  return JSON.stringify(
    Object.entries(values ?? {}).sort(([left], [right]) => left.localeCompare(right)),
  );
}

function modelConfigResolutionKey(
  agentName: string,
  request: ResolveAgentModelConfigRequest,
): string {
  return JSON.stringify([
    agentName,
    request.model,
    request.mode ?? "",
    stableRecordKey(request.config_options),
  ]);
}

function cloneModelConfigResponse(response: AgentModelConfigResponse): AgentModelConfigResponse {
  return {
    ...response,
    config_options: (response.config_options ?? []).map((option) => ({
      ...option,
      options: option.options?.map((item) => ({ ...item })),
    })),
  };
}

async function fetchResolvedModelConfig(
  agentName: string,
  request: ResolveAgentModelConfigRequest,
  forceRefresh: boolean,
): Promise<AgentModelConfigResponse> {
  const key = modelConfigResolutionKey(agentName, request);
  if (!forceRefresh) {
    const cached = modelConfigResolutionCache.get(key);
    if (cached && cached.expiresAt > Date.now()) {
      return cloneModelConfigResponse(cached.response);
    }
    if (cached) {
      modelConfigResolutionCache.delete(key);
    }
  }

  const existing = modelConfigResolutionInflight.get(key);
  if (existing) return existing.then(cloneModelConfigResponse);

  const generation = modelConfigResolutionGeneration;
  const payload = forceRefresh ? { ...request, refresh: true } : request;
  const promise = resolveAgentModelConfig(agentName, payload).then((response) => {
    const cloned = cloneModelConfigResponse(response);
    if (cloned.status === "ok" && generation === modelConfigResolutionGeneration) {
      modelConfigResolutionCache.set(key, {
        response: cloned,
        expiresAt: Date.now() + MODEL_CONFIG_RESOLUTION_TTL_MS,
      });
    }
    return cloned;
  });
  modelConfigResolutionInflight.set(key, promise);
  void promise.then(
    () => {
      if (modelConfigResolutionInflight.get(key) === promise) {
        modelConfigResolutionInflight.delete(key);
      }
    },
    () => {
      if (modelConfigResolutionInflight.get(key) === promise) {
        modelConfigResolutionInflight.delete(key);
      }
    },
  );
  return promise.then(cloneModelConfigResponse);
}

function isCapabilityProbePending(status: CapabilityStatus): boolean {
  return status === "not_configured" || status === "probing";
}

async function fetchCapabilitiesUntilSettled(
  agentName: string,
  forceRefresh: boolean,
): Promise<DynamicModelsResponse> {
  for (let attempt = 0; ; attempt += 1) {
    const response = await fetchDynamicModels(agentName, { refresh: forceRefresh });
    if (forceRefresh || !isCapabilityProbePending(response.status)) {
      return response;
    }
    if (attempt >= CAPABILITY_PENDING_MAX_ATTEMPTS) {
      return response;
    }
    await new Promise((resolve) => setTimeout(resolve, CAPABILITY_PENDING_POLL_DELAY_MS));
  }
}

type ModelConfigResolutionResult =
  | { kind: "response"; response: AgentModelConfigResponse }
  | { kind: "error"; error: string };

async function resolveModelConfig(
  agentName: string,
  request: ResolveAgentModelConfigRequest,
  forceRefresh: boolean,
): Promise<ModelConfigResolutionResult> {
  try {
    return {
      kind: "response",
      response: await fetchResolvedModelConfig(agentName, request, forceRefresh),
    };
  } catch (err) {
    return {
      kind: "error",
      error: err instanceof Error ? err.message : t("agents:failedToFetchCapabilities"),
    };
  }
}

/**
 * useAgentCapabilities fetches the full ACP probe cache for an agent
 * (models, modes, current defaults) and keeps it in sync. Refresh triggers
 * a live re-probe against the host utility and updates both models and
 * modes atomically — so the profile page's refresh button covers the whole
 * agent surface, not just models.
 */
export function useAgentCapabilities(
  agentName: string | undefined,
  initial: ModelConfig,
): UseAgentCapabilitiesState {
  const supportsDynamicModels = initial.supports_dynamic_models;
  const [models, setModels] = useState<ModelEntry[]>(initial.available_models);
  const [modes, setModes] = useState<ModeEntry[]>(initial.available_modes ?? []);
  const [commands, setCommands] = useState<CommandEntry[]>(initial.available_commands ?? []);
  const [currentModelId, setCurrentModelId] = useState<string | undefined>(
    initial.current_model_id,
  );
  const [currentModeId, setCurrentModeId] = useState<string | undefined>(initial.current_mode_id);
  const [status, setStatus] = useState<CapabilityStatus | undefined>(initial.status);
  const [manualRefreshAgentName, setManualRefreshAgentName] = useState<string>();
  const hasManualRefresh = manualRefreshAgentName === agentName;
  const [isLoading, setIsLoading] = useState(supportsDynamicModels && !!agentName);
  const [error, setError] = useState<string | null>(null);
  const capabilityRequestSequence = useRef(0);

  const fetchCaps = useCallback(
    async (forceRefresh: boolean) => {
      if (!agentName || !supportsDynamicModels) {
        return;
      }
      const sequence = ++capabilityRequestSequence.current;
      setIsLoading(true);
      if (forceRefresh) {
        invalidateModelConfigResolutionCache(agentName);
      }
      setError(null);
      try {
        const response = await fetchCapabilitiesUntilSettled(agentName, forceRefresh);
        if (sequence !== capabilityRequestSequence.current) return;
        setStatus(response.status);
        setError(response.error ?? null);
        if (forceRefresh) {
          setManualRefreshAgentName(agentName);
        }
        if (response.status === "ok") {
          setModels(response.models ?? []);
          setModes(response.modes ?? []);
          setCommands(response.commands ?? []);
          setCurrentModelId(response.current_model_id);
          setCurrentModeId(response.current_mode_id);
        }
      } catch (err) {
        if (sequence !== capabilityRequestSequence.current) return;
        setError(err instanceof Error ? err.message : t("agents:failedToFetchCapabilities"));
      } finally {
        if (sequence === capabilityRequestSequence.current) {
          setIsLoading(false);
        }
      }
    },
    [agentName, supportsDynamicModels],
  );

  useEffect(() => {
    if (!hasManualRefresh) {
      setStatus(initial.status);
    }
  }, [hasManualRefresh, initial.status]);

  useEffect(() => {
    if (supportsDynamicModels && agentName) {
      void fetchCaps(false);
    }
    return () => {
      capabilityRequestSequence.current += 1;
    };
  }, [agentName, supportsDynamicModels, fetchCaps]);

  const refresh = useCallback(() => fetchCaps(true), [fetchCaps]);

  return {
    models,
    modes,
    commands,
    currentModelId,
    currentModeId,
    status,
    isLoading,
    error,
    refresh,
  };
}

/**
 * Resolve the complete provider option snapshot for the selected model.
 *
 * The response is model-aware. A provider can therefore return a different
 * option list for every model without adding provider-specific frontend code.
 */
export function useResolvedModelConfig(
  agentName: string | undefined,
  model: string | undefined,
  options: UseResolvedModelConfigOptions = {},
): UseResolvedModelConfigState {
  const { mode, configOptions, initialConfigOptions = [], enabled = true } = options;
  const configOptionsKey = stableRecordKey(configOptions);
  const initialConfigOptionsKey = JSON.stringify(initialConfigOptions);
  const stableConfigOptions = useMemo(() => configOptions, [configOptionsKey]);
  const stableInitialConfigOptions = useMemo(() => initialConfigOptions, [initialConfigOptionsKey]);
  const request = useMemo<ResolveAgentModelConfigRequest | null>(() => {
    if (!model) return null;
    return {
      model,
      ...(mode ? { mode } : {}),
      ...(stableConfigOptions && Object.keys(stableConfigOptions).length > 0
        ? { config_options: stableConfigOptions }
        : {}),
    };
  }, [mode, model, stableConfigOptions]);
  const requestKey = useMemo(
    () => (request ? modelConfigResolutionKey(agentName ?? "", request) : undefined),
    [agentName, request],
  );
  const [resolvedOptions, setResolvedOptions] = useState<ConfigOptionEntry[]>(
    stableInitialConfigOptions,
  );
  const [status, setStatus] = useState<CapabilityStatus>();
  const [resolvedRequestKey, setResolvedRequestKey] = useState<string>();
  const [settledRequestKey, setSettledRequestKey] = useState<string>();
  const [isLoading, setIsLoading] = useState(Boolean(enabled && agentName && request));
  const [error, setError] = useState<string | null>(null);
  const requestSequence = useRef(0);

  const run = useCallback(
    async (forceRefresh: boolean) => {
      const sequence = ++requestSequence.current;
      if (!enabled || !agentName || !request) {
        setResolvedOptions(stableInitialConfigOptions);
        setStatus(undefined);
        setResolvedRequestKey(undefined);
        setSettledRequestKey(undefined);
        setError(null);
        setIsLoading(false);
        return;
      }

      setResolvedRequestKey(undefined);
      setSettledRequestKey(undefined);
      setResolvedOptions(stableInitialConfigOptions);
      setStatus(forceRefresh ? "probing" : undefined);
      setError(null);
      setIsLoading(true);
      const result = await resolveModelConfig(agentName, request, forceRefresh);
      if (sequence !== requestSequence.current) return;
      if (result.kind === "error") {
        setStatus("failed");
        setError(result.error);
        setResolvedOptions(stableInitialConfigOptions);
        if (requestKey) setSettledRequestKey(requestKey);
      } else {
        const response = result.response;
        setStatus(response.status);
        setError(response.error ?? null);
        setResolvedOptions(
          response.status === "ok" ? (response.config_options ?? []) : stableInitialConfigOptions,
        );
        if (response.status === "ok" && requestKey) {
          setResolvedRequestKey(requestKey);
        }
        if (requestKey) setSettledRequestKey(requestKey);
      }
      setIsLoading(false);
    },
    [agentName, enabled, request, requestKey, stableInitialConfigOptions],
  );

  useEffect(() => {
    void run(false);
    return () => {
      requestSequence.current += 1;
    };
  }, [run]);

  const refresh = useCallback(() => run(true), [run]);

  return {
    configOptions: resolvedOptions,
    status,
    isResolvedForRequest: requestKey !== undefined && resolvedRequestKey === requestKey,
    isResolutionPending: Boolean(
      enabled && agentName && requestKey && (isLoading || settledRequestKey !== requestKey),
    ),
    isLoading,
    error,
    refresh,
  };
}
