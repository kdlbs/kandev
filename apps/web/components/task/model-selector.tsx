"use client";

import { memo, useCallback, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  configOptionToModelOptions,
  isModelConfigOption,
  ModelConfigSelector,
  type ModelSelectorOption,
  type SelectConfigOption,
  usableConfigOptions,
} from "@/components/model-config-selector";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { setSessionConfigOption, setSessionModel } from "@/lib/api/domains/session-api";
import { qk } from "@/lib/query/keys";
import { sessionModelsQueryOptions } from "@/lib/query/query-options";
import type { Agent, AgentProfile, AvailableAgent } from "@/lib/types/http";
import type {
  ConfigOptionEntry,
  SessionModelEntry,
} from "@/lib/state/slices/session-runtime/types";

type SessionModelsEntry = {
  currentModelId: string;
  models: SessionModelEntry[];
  configOptions: ConfigOptionEntry[];
};

type ModelSelectorProps = {
  sessionId: string | null;
  triggerClassName?: string;
};

function resolveSnapshotModel(snapshot: unknown): string | null {
  if (!snapshot || typeof snapshot !== "object") return null;
  const model = (snapshot as Record<string, unknown>).model;
  return typeof model === "string" && model ? model : null;
}

function resolveStaticModels(
  agents: Agent[],
  profileId: string | null | undefined,
  availableAgents: AvailableAgent[],
): ModelSelectorOption[] {
  if (!profileId) return [];
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (!profile) continue;
    const available = availableAgents.find((a: AvailableAgent) => a.name === agent.name);
    const models = available?.model_config?.available_models ?? [];
    return models.map((m) => ({
      ...m,
      description: m.id !== m.name ? m.id : undefined,
    }));
  }
  return [];
}

function sessionModelsToOptions(models: SessionModelEntry[]): ModelSelectorOption[] {
  return models.map((m) => ({
    id: m.modelId,
    name: m.name,
    description: m.description,
    usageMultiplier: m.usageMultiplier,
  }));
}

function buildModelOptions(
  availableModels: ModelSelectorOption[],
  currentModel: string | null,
): ModelSelectorOption[] {
  const options = [...availableModels];
  if (currentModel && !options.some((m) => m.id === currentModel)) {
    options.unshift({ id: currentModel, name: currentModel });
  }
  return options;
}

function resolveProfileModel(profileId: string | null | undefined, agents: Agent[]): string | null {
  if (!profileId) return null;
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (profile?.model) return profile.model;
  }
  return null;
}

function resolveCurrentModel(
  activeModel: string | null,
  acpCurrentModel: string | null,
  snapshotModel: string | null,
  profileModel: string | null,
): string | null {
  return activeModel || acpCurrentModel || snapshotModel || profileModel;
}

function updateConfigOptionValue(
  options: ConfigOptionEntry[],
  configId: string,
  value: string,
): ConfigOptionEntry[] {
  return options.map((option) =>
    option.id === configId ? { ...option, currentValue: value } : option,
  );
}

function nextCurrentModelId(
  data: { currentModelId: string; configOptions: ConfigOptionEntry[] },
  configId: string,
  value: string,
): string {
  const option = data.configOptions.find((item) => item.id === configId);
  if (option && isModelConfigOption(option)) return value;
  return data.currentModelId;
}

function resolveAvailableModels({
  modelConfig,
  usingAcpModels,
  sessionModels,
  settingsAgents,
  profileId,
  availableAgents,
}: {
  modelConfig: SelectConfigOption | undefined;
  usingAcpModels: boolean;
  sessionModels: SessionModelEntry[];
  settingsAgents: Agent[];
  profileId: string | null | undefined;
  availableAgents: AvailableAgent[];
}): ModelSelectorOption[] {
  if (modelConfig) return configOptionToModelOptions(modelConfig);
  if (usingAcpModels) return sessionModelsToOptions(sessionModels);
  return resolveStaticModels(settingsAgents, profileId, availableAgents);
}

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : "Unknown error";
}

/** Builds model/config change handlers with optimistic update + error toast + revert. */
function useModelChangeHandlers(
  configOptions: SelectConfigOption[],
  sessionModelsData: SessionModelsEntry | undefined,
) {
  const queryClient = useQueryClient();
  const activeModels = useAppStore((state) => state.activeModel.bySessionId);
  const setActiveModel = useAppStore((state) => state.setActiveModel);
  const setSessionModels = useAppStore((state) => state.setSessionModels);
  const { toast } = useToast();
  // Per-session monotonic request id so a stale failure doesn't clobber a
  // newer successful selection (rapid A -> B -> C where B fails).
  const latestReqId = useRef<Record<string, number>>({});

  const updateLocalConfig = useCallback(
    (sid: string, configId: string, value: string) => {
      if (!sessionModelsData) return;
      setSessionModels(sid, {
        ...sessionModelsData,
        currentModelId: nextCurrentModelId(sessionModelsData, configId, value),
        configOptions: updateConfigOptionValue(sessionModelsData.configOptions, configId, value),
      });
      queryClient.setQueryData(qk.sessionRuntime.models(sid), {
        ...sessionModelsData,
        currentModelId: nextCurrentModelId(sessionModelsData, configId, value),
        configOptions: updateConfigOptionValue(sessionModelsData.configOptions, configId, value),
      });
    },
    [queryClient, sessionModelsData, setSessionModels],
  );

  const onFail = useCallback(
    (
      sid: string,
      reqId: number,
      previousActive: string,
      previousModels: SessionModelsEntry | undefined,
    ) =>
      (err: unknown) => {
        if (latestReqId.current[sid] !== reqId) return;
        console.error("[ModelSelector] model change failed:", err);
        setActiveModel(sid, previousActive);
        if (previousModels) {
          setSessionModels(sid, previousModels);
          queryClient.setQueryData(qk.sessionRuntime.models(sid), previousModels);
        }
        toast({
          title: "Failed to change model",
          description: describeError(err),
          variant: "error",
        });
      },
    [queryClient, setActiveModel, setSessionModels, toast],
  );

  const nextReqId = useCallback((sid: string) => {
    const id = (latestReqId.current[sid] ?? 0) + 1;
    latestReqId.current[sid] = id;
    return id;
  }, []);

  const handleModelChange = useCallback(
    (sid: string, modelId: string) => {
      const reqId = nextReqId(sid);
      const fail = onFail(sid, reqId, activeModels[sid] ?? "", sessionModelsData);
      setActiveModel(sid, modelId);
      const modelConfig = configOptions.find(isModelConfigOption);
      if (modelConfig) {
        updateLocalConfig(sid, modelConfig.id, modelId);
        setSessionConfigOption(sid, modelConfig.id, modelId).catch(fail);
        return;
      }
      setSessionModel(sid, modelId).catch(fail);
    },
    [
      activeModels,
      configOptions,
      nextReqId,
      onFail,
      sessionModelsData,
      setActiveModel,
      updateLocalConfig,
    ],
  );

  const handleConfigChange = useCallback(
    (sid: string, configId: string, value: string) => {
      const reqId = nextReqId(sid);
      const fail = onFail(sid, reqId, activeModels[sid] ?? "", sessionModelsData);
      updateLocalConfig(sid, configId, value);
      setSessionConfigOption(sid, configId, value).catch(fail);
    },
    [activeModels, nextReqId, onFail, sessionModelsData, updateLocalConfig],
  );

  return { handleModelChange, handleConfigChange };
}

function useTaskSessionForModel(sessionId: string | null) {
  return useAppStore((state) => {
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId] ?? null;
  });
}

function useActiveModelForSession(sessionId: string | null) {
  return useAppStore((state) => {
    if (!sessionId) return null;
    return state.activeModel.bySessionId[sessionId] || null;
  });
}

function useSessionModelsData(sessionId: string | null) {
  const sessionModelsQuery = useQuery(sessionModelsQueryOptions(sessionId ?? ""));
  const storeSessionModelsData = useAppStore((state) =>
    sessionId ? state.sessionModels.bySessionId[sessionId] : undefined,
  );
  return sessionModelsQuery.data ?? storeSessionModelsData;
}

/** Resolves available models, config options and current model from store state. */
function useModelSelectorState(sessionId: string | null) {
  const settingsCatalog = useSettingsData(true);

  const settingsAgents = settingsCatalog.settingsAgents;
  const { items: availableAgents } = useAvailableAgents();
  const session = useTaskSessionForModel(sessionId);
  const activeModel = useActiveModelForSession(sessionId);
  const sessionModelsData = useSessionModelsData(sessionId);

  const snapshotModel = resolveSnapshotModel(session?.agent_profile_snapshot);
  const profileModel = useMemo(
    () => resolveProfileModel(session?.agent_profile_id, settingsAgents as Agent[]),
    [session?.agent_profile_id, settingsAgents],
  );

  const usingAcpModels = !!sessionModelsData?.models?.length;
  const configOptions = usableConfigOptions(sessionModelsData?.configOptions);
  const modelConfig = configOptions.find(isModelConfigOption);
  const availableModels = resolveAvailableModels({
    modelConfig,
    usingAcpModels,
    sessionModels: sessionModelsData?.models ?? [],
    settingsAgents: settingsAgents as Agent[],
    profileId: session?.agent_profile_id,
    availableAgents,
  });

  const acpCurrentModel = sessionModelsData?.currentModelId || null;
  const currentModel = resolveCurrentModel(
    activeModel,
    acpCurrentModel,
    snapshotModel,
    profileModel,
  );
  const modelOptions = buildModelOptions(availableModels, currentModel);

  const { handleModelChange, handleConfigChange } = useModelChangeHandlers(
    configOptions,
    sessionModelsData,
  );

  return { currentModel, modelOptions, configOptions, handleModelChange, handleConfigChange };
}

export const ModelSelector = memo(function ModelSelector({
  sessionId,
  triggerClassName,
}: ModelSelectorProps) {
  const { currentModel, modelOptions, configOptions, handleModelChange, handleConfigChange } =
    useModelSelectorState(sessionId);
  const modelConfig = configOptions.find(isModelConfigOption);

  const onModelChange = useCallback(
    (value: string) => {
      if (!sessionId) return;
      handleModelChange(sessionId, value);
    },
    [sessionId, handleModelChange],
  );

  const onConfigChange = useCallback(
    (configId: string, value: string) => {
      if (!sessionId) return;
      handleConfigChange(sessionId, configId, value);
    },
    [sessionId, handleConfigChange],
  );

  if (!sessionId || (!currentModel && !modelConfig)) return null;

  return (
    <ModelConfigSelector
      modelOptions={modelOptions}
      currentModel={currentModel}
      configOptions={configOptions}
      onModelChange={onModelChange}
      onConfigChange={onConfigChange}
      placeholder="Model"
      ariaLabel="Session model settings"
      variant="compact"
      popoverSide="top"
      triggerClassName={triggerClassName}
    />
  );
});
