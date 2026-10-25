import {
  displayModelName,
  isModelConfigOption,
  usableConfigOptions,
  type DynamicConfigOption,
  type ModelSelectorOption,
} from "@/components/model-config-selector";
import type { AppState } from "@/lib/state/store";

type ResolveSessionTabTitleArgs = {
  /** User-supplied session name; wins over every derived title when set. */
  customName?: string | null;
  agentLabel: string | null;
  activeModelId: string | null;
  currentModelId: string | null;
  snapshotModel: string | null;
  modelOptions: ModelSelectorOption[];
  configOptions: DynamicConfigOption[];
};

function optionName(option: DynamicConfigOption, value: string): string {
  return option.options?.find((item) => item.value === value)?.name ?? value;
}

function resolveModelTitle(
  args: ResolveSessionTabTitleArgs,
  modelId: string | null,
): string | null {
  if (!modelId) return null;

  const modelConfig = args.configOptions.find(isModelConfigOption);
  let modelLabel = displayModelName(args.modelOptions, modelId);
  if (modelConfig) {
    // Use caller-supplied modelId, not modelConfig.currentValue, so live
    // active/current model switches are reflected immediately in the tab title.
    modelLabel = optionName(modelConfig, modelId);
  }
  return modelLabel;
}

export function resolveSessionTabTitle(args: ResolveSessionTabTitleArgs): string | null {
  if (args.customName) return args.customName;
  const modelConfig = args.configOptions.find(isModelConfigOption);
  const currentModelId = args.currentModelId || modelConfig?.currentValue || null;
  return (
    resolveModelTitle(args, currentModelId) ??
    resolveModelTitle(args, args.activeModelId) ??
    args.agentLabel ??
    resolveModelTitle(args, args.snapshotModel)
  );
}

function resolveAgentLabel(state: AppState, agentProfileId?: string): string | null {
  if (!agentProfileId) return null;
  const profile = state.agentProfiles.items.find((item) => item.id === agentProfileId);
  if (!profile) return null;
  const parts = profile.label.split(" \u2022 ");
  return parts[1] || parts[0] || profile.label;
}

function profileSnapshotModel(session: AppState["taskSessions"]["items"][string]): string | null {
  return typeof session?.agent_profile_snapshot?.model === "string"
    ? session.agent_profile_snapshot.model
    : null;
}

/** Selects the same live label used by task session tabs for one loaded session. */
export function selectSessionTabTitle(state: AppState, sessionId: string): string | null {
  const session = state.taskSessions.items[sessionId];
  if (!session) return null;

  const sessionModels = state.sessionModels.bySessionId[sessionId];

  return resolveSessionTabTitle({
    customName: session.name ?? null,
    agentLabel: resolveAgentLabel(state, session.agent_profile_id),
    activeModelId: state.activeModel.bySessionId[sessionId] || null,
    currentModelId: sessionModels?.currentModelId || null,
    snapshotModel: profileSnapshotModel(session),
    modelOptions:
      sessionModels?.models.map((model) => ({
        id: model.modelId,
        name: model.name,
        description: model.description,
        usageMultiplier: model.usageMultiplier,
      })) ?? [],
    configOptions: usableConfigOptions(sessionModels?.configOptions),
  });
}
