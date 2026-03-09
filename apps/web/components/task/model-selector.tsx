"use client";

import { memo, useCallback, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import type { Agent, AgentProfile, AvailableAgent } from "@/lib/types/http";
import { setSessionModel } from "@/lib/api/domains/session-api";
import type { SessionModelEntry } from "@/lib/state/slices/session-runtime/types";

type ModelSelectorProps = {
  sessionId: string | null;
};

type ModelOption = {
  id: string;
  name: string;
  provider: string;
  context_window: number;
  is_default: boolean;
  description?: string;
  usageMultiplier?: string;
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
): ModelOption[] {
  if (!profileId) return [];
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (!profile) continue;
    const available = availableAgents.find((a: AvailableAgent) => a.name === agent.name);
    const models = available?.model_config?.available_models ?? [];
    // Static models don't include description — use model ID as subtitle when it differs from name
    return models.map((m) => ({
      ...m,
      description: m.id !== m.name ? m.id : undefined,
    }));
  }
  return [];
}

function sessionModelsToOptions(models: SessionModelEntry[]): ModelOption[] {
  return models.map((m) => ({
    id: m.modelId,
    name: m.name,
    provider: "",
    context_window: 0,
    is_default: false,
    description: m.description,
    usageMultiplier: m.usageMultiplier,
  }));
}

function buildModelOptions(
  availableModels: ModelOption[],
  currentModel: string | null,
): ModelOption[] {
  const options = [...availableModels];
  if (currentModel && !options.some((m) => m.id === currentModel)) {
    options.unshift({
      id: currentModel,
      name: currentModel,
      provider: "unknown",
      context_window: 0,
      is_default: false,
    });
  }
  return options;
}

/** Resolves available models and current model from store state. */
function useModelSelectorState(sessionId: string | null) {
  useSettingsData(true);

  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const taskSessions = useAppStore((state) => state.taskSessions.items);
  const activeModels = useAppStore((state) => state.activeModel.bySessionId);
  const setActiveModel = useAppStore((state) => state.setActiveModel);
  const { items: availableAgents } = useAvailableAgents();
  const sessionModelsData = useAppStore((state) =>
    sessionId ? state.sessionModels.bySessionId[sessionId] : undefined,
  );

  const session = sessionId ? (taskSessions[sessionId] ?? null) : null;
  const snapshotModel = resolveSnapshotModel(session?.agent_profile_snapshot);

  // Prefer dynamic ACP session models when available, fall back to static registry models
  const availableModels = sessionModelsData?.models?.length
    ? sessionModelsToOptions(sessionModelsData.models)
    : resolveStaticModels(settingsAgents as Agent[], session?.agent_profile_id, availableAgents);

  const activeModel = sessionId ? activeModels[sessionId] || null : null;
  const acpCurrentModel = sessionModelsData?.currentModelId || null;
  const currentModel = activeModel || acpCurrentModel || snapshotModel;
  const modelOptions = buildModelOptions(availableModels, currentModel);

  const handleModelChange = useCallback(
    (sid: string, modelId: string) => {
      setActiveModel(sid, modelId);
      setSessionModel(sid, modelId).catch((err) => {
        console.error("[ModelSelector] set-model API failed:", err);
      });
    },
    [setActiveModel],
  );

  return { currentModel, modelOptions, handleModelChange };
}

function modelToComboboxOption(model: ModelOption): ComboboxOption {
  return {
    value: model.id,
    label: model.name,
    description: model.description,
    renderLabel: () => (
      <>
        <div className="min-w-0 flex-1">
          <div className="truncate">{model.name}</div>
          {model.description && (
            <div className="text-xs text-muted-foreground truncate" title={model.description}>
              {model.description}
            </div>
          )}
        </div>
        {model.usageMultiplier && (
          <span className="text-xs text-muted-foreground shrink-0">{model.usageMultiplier}</span>
        )}
      </>
    ),
  };
}

export const ModelSelector = memo(function ModelSelector({ sessionId }: ModelSelectorProps) {
  const { currentModel, modelOptions, handleModelChange } = useModelSelectorState(sessionId);

  const comboboxOptions = useMemo(() => modelOptions.map(modelToComboboxOption), [modelOptions]);

  const onValueChange = useCallback(
    (value: string) => {
      // Don't allow deselecting — always keep a model selected
      if (!value || !sessionId) return;
      handleModelChange(sessionId, value);
    },
    [sessionId, handleModelChange],
  );

  if (!sessionId || !currentModel) return null;

  return (
    <Combobox
      options={comboboxOptions}
      value={currentModel}
      onValueChange={onValueChange}
      placeholder="Select model..."
      searchPlaceholder="Filter models..."
      emptyMessage="No models found."
      showSearch={modelOptions.length > 5}
      triggerClassName="h-7 gap-1 px-2 text-xs w-auto hover:bg-muted/40 whitespace-nowrap"
      className="min-w-[280px]"
      plainTrigger
      popoverSide="top"
      popoverAlign="end"
    />
  );
});
