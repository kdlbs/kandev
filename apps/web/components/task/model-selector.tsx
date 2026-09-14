"use client";

import { memo, useCallback, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

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
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { setSessionConfigOption, setSessionModel } from "@/lib/api/domains/session-api";
import type { Agent, AgentProfile, AvailableAgent, TaskSession } from "@/lib/types/http";
import type {
  ConfigOptionEntry,
  SessionModelEntry,
} from "@/lib/state/slices/session-runtime/types";
type SessionModelsEntry = {
  currentModelId: string;
  models: SessionModelEntry[];
  configOptions: ConfigOptionEntry[];
  configOptionsSettled?: boolean;
  configBaseline?: Record<string, string>;
  /** Set when the session started on the profile's fallback model. */
  fallbackModel?: string;
};

type ModelSelectorProps = {
  sessionId: string | null;
  triggerClassName?: string;
};

const debug = createDebugLogger("model-selector:gate");

function configValueKeys(value: unknown): string[] {
  if (!value || typeof value !== "object" || Array.isArray(value)) return [];
  return Object.entries(value)
    .filter((entry): entry is [string, string] => typeof entry[1] === "string" && !!entry[1])
    .map(([key]) => key);
}

function configValue(value: unknown, key: string): string | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const candidate = (value as Record<string, unknown>)[key];
  return typeof candidate === "string" && candidate ? candidate : undefined;
}

const MODEL_CONFIG_KEY = "model";
// "agent" identifies which agent runs the session. Legacy snapshots can record
// it as an identity or the stale value "default", but provider-defined agent
// options remain required when the provider advertises them.
const AGENT_CONFIG_KEY = "agent";
const LEGACY_AGENT_CONFIG_VALUE = "default";

function isLegacyAgentConfig(session: TaskSession | null, agents: Agent[]): boolean {
  if (!session) return false;
  const snapshot = session.agent_profile_snapshot;
  const profile = agents
    .flatMap((agent) => agent.profiles)
    .find((item) => item.id === session.agent_profile_id);
  const identityValues = new Set(
    [
      configValue(snapshot, "agent_id"),
      configValue(snapshot, "agent_name"),
      profile?.agentId,
      profile?.agentDisplayName,
    ].filter((value): value is string => !!value),
  );
  const optionSources = [
    snapshot?.config_options,
    profile?.configOptions,
    (session.metadata?.runtime_config as Record<string, unknown> | undefined)?.config_options,
    (session.metadata?.runtime_config_overrides as Record<string, unknown> | undefined)
      ?.config_options,
  ];
  return optionSources.some((source) => {
    const value = configValue(source, AGENT_CONFIG_KEY);
    return value === LEGACY_AGENT_CONFIG_VALUE || (!!value && identityValues.has(value));
  });
}

// profileRequiredConfigKeys returns keys the current agent profile itself
// declares (snapshot + matched profile). These stay required even when the
// running agent has not advertised them, because the profile chose them.
function profileRequiredConfigKeys(session: TaskSession, agents: Agent[]): Set<string> {
  const keys = new Set(configValueKeys(session.agent_profile_snapshot?.config_options));
  for (const agent of agents) {
    const profile = agent.profiles.find((item) => item.id === session.agent_profile_id);
    for (const key of Object.keys(profile?.configOptions ?? {})) keys.add(key);
  }
  return keys;
}

// persistedRuntimeConfigKeys returns keys recorded only in the session's
// persisted runtime metadata. A prior agent type may have written keys the
// current agent never advertises; these must not block the selector.
function persistedRuntimeConfigKeys(session: TaskSession): Set<string> {
  const keys = new Set<string>();
  for (const value of [
    session.metadata?.runtime_config,
    session.metadata?.runtime_config_overrides,
  ]) {
    if (!value || typeof value !== "object") continue;
    for (const key of configValueKeys((value as Record<string, unknown>).config_options)) {
      keys.add(key);
    }
  }
  return keys;
}

export function requiredConfigKeys(session: TaskSession | null, agents: Agent[]): string[] {
  if (!session) return [];
  const keys = profileRequiredConfigKeys(session, agents);
  for (const key of persistedRuntimeConfigKeys(session)) keys.add(key);
  return [...keys];
}

export function hasCompleteDynamicConfig(
  session: TaskSession | null,
  sessionModelsData: SessionModelsEntry | undefined,
  agents: Agent[],
): boolean {
  const required = requiredConfigKeys(session, agents);
  if (required.length === 0) return true;
  if (!sessionModelsData) return false;
  const available = new Set(sessionModelsData.configOptions.map((option) => option.id));
  // Flat-model-list agents (e.g. claude-opus) expose their models via the ACP
  // top-level `models` list and switch with session/set_model rather than a
  // SessionConfigOption(category="model"). For those the persisted runtime
  // config still records a "model" key, but it will never appear in
  // configOptions. Treat that key as satisfied when the session has a flat model
  // list — the selector renders fine from it.
  const hasFlatModelList = !!sessionModelsData.models.length;
  const catalogSettled = sessionModelsData.configOptionsSettled === true;
  const hasLegacyAgentConfig = catalogSettled && isLegacyAgentConfig(session, agents);
  // A settled catalog is authoritative for which non-agent config keys apply
  // to the selected model.
  const isUnadvertisedSettledKey = (key: string): boolean =>
    key !== AGENT_CONFIG_KEY && catalogSettled && !available.has(key);
  return required.every(
    (key) =>
      available.has(key) ||
      (key === AGENT_CONFIG_KEY && hasLegacyAgentConfig) ||
      (key === MODEL_CONFIG_KEY && hasFlatModelList) ||
      isUnadvertisedSettledKey(key),
  );
}

function resolveSessionState(
  sessionId: string | null,
  taskSessions: Record<string, TaskSession>,
  activeModels: Record<string, string>,
  sessionModelsData: SessionModelsEntry | undefined,
) {
  if (!sessionId) {
    return { session: null, activeModel: null, sessionModelsData: undefined };
  }
  return {
    session: taskSessions[sessionId] ?? null,
    activeModel: activeModels[sessionId] || null,
    sessionModelsData,
  };
}

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

// annotateFallbackOptions marks the fallback model so the user sees the
// session is not on the configured start model. It returns NEW option objects
// (never mutating the input or shared store state) and reuses the same
// translated suffix as the trigger label so the option and trigger cannot
// disagree. Returns the same array when no fallback note is active.
export function annotateFallbackOptions(
  options: ModelSelectorOption[],
  fallbackModel: string | undefined,
): ModelSelectorOption[] {
  if (!fallbackModel) return options;
  const suffix = t("settings:modelFallbackSuffix");
  return options.map((option) =>
    option.id === fallbackModel && !option.name.endsWith(suffix)
      ? { ...option, name: `${option.name} ${suffix}` }
      : option,
  );
}

export function buildModelOptions(
  availableModels: ModelSelectorOption[],
  currentModel: string | null,
): ModelSelectorOption[] {
  const options = [...availableModels];
  if (currentModel && !options.some((m) => m.id === currentModel)) {
    // The configured/active model is no longer advertised ("gone"). Keep it
    // visible but greyed out so the user is asked to pick a replacement —
    // never silently drop it.
    options.unshift({
      id: currentModel,
      name: currentModel,
      disabled: true,
      disabledReason: t("settings:startModelUnavailable"),
    });
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
  return err instanceof Error ? err.message : t("task:unknownError2");
}

/** Builds model/config change handlers with optimistic update + error toast + revert. */
function useModelChangeHandlers(
  configOptions: SelectConfigOption[],
  sessionModelsData: SessionModelsEntry | undefined,
) {
  const { t } = useTranslation();
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
      // A manual model change ends the fallback story — drop the note so the
      // picker stops labelling the model as a fallback.
      const fallbackModel =
        configId === sessionModelsData.configOptions.find(isModelConfigOption)?.id
          ? undefined
          : sessionModelsData.fallbackModel;
      setSessionModels(sid, {
        ...sessionModelsData,
        fallbackModel,
        currentModelId: nextCurrentModelId(sessionModelsData, configId, value),
        configOptions: updateConfigOptionValue(sessionModelsData.configOptions, configId, value),
      });
    },
    [sessionModelsData, setSessionModels],
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
        if (previousModels) setSessionModels(sid, previousModels);
        toast({
          title: t("task:failedToChangeModel"),
          description: describeError(err),
          variant: "error",
        });
      },
    [setActiveModel, setSessionModels, toast],
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
      // Non-config path: the next models_updated event would preserve the
      // fallback note (see session-models handler) — clear it locally now
      // that the user picked a model explicitly.
      if (sessionModelsData) {
        setSessionModels(sid, { ...sessionModelsData, fallbackModel: undefined });
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

// resolveModelSelectorInputs derives the model list, config options and
// current model from store state, independent of the hook lifecycle.
function resolveModelSelectorInputs({
  session,
  sessionModelsData,
  activeModel,
  settingsAgents,
  availableAgents,
  profileModel,
}: {
  session: TaskSession | null;
  sessionModelsData: SessionModelsEntry | undefined;
  activeModel: string | null;
  settingsAgents: Agent[];
  availableAgents: AvailableAgent[];
  profileModel: string | null;
}) {
  const usingAcpModels = !!sessionModelsData?.models?.length;
  const configOptions = usableConfigOptions(sessionModelsData?.configOptions);
  const modelConfig = configOptions.find(isModelConfigOption);
  const availableModels = resolveAvailableModels({
    modelConfig,
    usingAcpModels,
    sessionModels: sessionModelsData?.models ?? [],
    settingsAgents,
    profileId: session?.agent_profile_id,
    availableAgents,
  });
  const currentModel = resolveCurrentModel(
    activeModel,
    sessionModelsData?.currentModelId || null,
    resolveSnapshotModel(session?.agent_profile_snapshot),
    profileModel,
  );
  return { configOptions, currentModel, availableModels };
}

/** Resolves available models, config options and current model from store state. */
function useModelSelectorState(sessionId: string | null) {
  useSettingsData(true);

  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const taskSessions = useAppStore((state) => state.taskSessions.items);
  const activeModels = useAppStore((state) => state.activeModel.bySessionId);
  const { items: availableAgents } = useAvailableAgents();
  const selectedSessionModels = useAppStore((state) =>
    sessionId ? state.sessionModels.bySessionId[sessionId] : undefined,
  );
  const { session, activeModel, sessionModelsData } = resolveSessionState(
    sessionId,
    taskSessions,
    activeModels,
    selectedSessionModels,
  );
  const profileModel = useMemo(
    () => resolveProfileModel(session?.agent_profile_id, settingsAgents as Agent[]),
    [session?.agent_profile_id, settingsAgents],
  );

  const { configOptions, currentModel, availableModels } = resolveModelSelectorInputs({
    session,
    sessionModelsData,
    activeModel,
    settingsAgents: settingsAgents as Agent[],
    availableAgents,
    profileModel,
  });
  const modelOptions = useMemo(
    () =>
      annotateFallbackOptions(
        buildModelOptions(availableModels, currentModel),
        sessionModelsData?.fallbackModel,
      ),
    [availableModels, currentModel, sessionModelsData?.fallbackModel],
  );

  const { handleModelChange, handleConfigChange } = useModelChangeHandlers(
    configOptions,
    sessionModelsData,
  );

  return {
    currentModel,
    modelOptions,
    configOptions,
    configBaseline: sessionModelsData?.configBaseline,
    fallbackModel: sessionModelsData?.fallbackModel ?? null,
    configHydrated: hasCompleteDynamicConfig(session, sessionModelsData, settingsAgents as Agent[]),
    requiredKeys: requiredConfigKeys(session, settingsAgents as Agent[]),
    rawConfigOptionIds: (sessionModelsData?.configOptions ?? []).map((o) => o.id),
    handleModelChange,
    handleConfigChange,
  };
}

export const ModelSelector = memo(function ModelSelector({
  sessionId,
  triggerClassName,
}: ModelSelectorProps) {
  const { t } = useTranslation();
  const {
    currentModel,
    modelOptions,
    configOptions,
    configBaseline,
    fallbackModel,
    configHydrated,
    requiredKeys,
    rawConfigOptionIds,
    handleModelChange,
    handleConfigChange,
  } = useModelSelectorState(sessionId);
  const modelConfig = configOptions.find(isModelConfigOption);
  // Explicit "using fallback" signal: annotate the trigger so the user sees
  // the session is not on the configured start model.
  const currentModelSuffix = fallbackModel ? ` ${t("settings:modelFallbackSuffix")}` : undefined;

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

  if (isDebug()) {
    debug("render", {
      sessionId: sessionId ?? "",
      configHydrated,
      currentModel: currentModel ?? "",
      hasModelConfig: !!modelConfig,
      modelOptionsLen: modelOptions.length,
      configOptionIds: configOptions.map((o) => o.id),
      rawConfigOptionIds,
      requiredKeys,
      willHide: !sessionId || !configHydrated || (!currentModel && !modelConfig),
    });
  }
  if (!sessionId || !configHydrated || (!currentModel && !modelConfig)) return null;

  return (
    <ModelConfigSelector
      modelOptions={modelOptions}
      currentModel={currentModel}
      configOptions={configOptions}
      onModelChange={onModelChange}
      onConfigChange={onConfigChange}
      placeholder={t("common:model")}
      ariaLabel={t("task:sessionModelSettings")}
      variant="compact"
      popoverSide="top"
      triggerClassName={triggerClassName}
      triggerSummary="changed"
      configBaseline={configBaseline}
      currentModelSuffix={currentModelSuffix}
    />
  );
});
