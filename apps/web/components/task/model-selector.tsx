"use client";

import { memo, useCallback } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
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
    return available?.model_config?.available_models ?? [];
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
        // TODO: remove debug log
        console.log("[ModelSelector] set-model API failed:", err);
      });
    },
    [setActiveModel],
  );

  return { currentModel, modelOptions, handleModelChange };
}

export const ModelSelector = memo(function ModelSelector({ sessionId }: ModelSelectorProps) {
  const { currentModel, modelOptions, handleModelChange } = useModelSelectorState(sessionId);

  if (!sessionId || !currentModel) return null;

  const displayName = modelOptions.find((m) => m.id === currentModel)?.name || currentModel;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1 px-2 cursor-pointer hover:bg-muted/40 whitespace-nowrap"
        >
          <span className="text-xs">{displayName}</span>
          <IconChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" className="min-w-[280px]">
        {modelOptions.map((model) => (
          <DropdownMenuItem
            key={model.id}
            onClick={() => handleModelChange(sessionId, model.id)}
            className={model.id === currentModel ? "bg-muted" : ""}
          >
            <div className="flex items-center justify-between w-full gap-2">
              <div>
                <div>{model.name}</div>
                {model.description && (
                  <div className="text-xs text-muted-foreground">{model.description}</div>
                )}
              </div>
              {model.usageMultiplier && (
                <span className="text-xs text-muted-foreground shrink-0">
                  {model.usageMultiplier}
                </span>
              )}
            </div>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
});
