import {
  displayModelName,
  isModelConfigOption,
  type DynamicConfigOption,
  type ModelSelectorOption,
} from "@/components/model-config-selector";

export function resolveSnapshotModel(
  snapshot: Record<string, unknown> | null | undefined,
): string | null {
  return typeof snapshot?.model === "string" ? snapshot.model : null;
}

type ResolveSessionTabTitleArgs = {
  /** User-supplied session name; wins over every derived title when set. */
  customName?: string | null;
  /**
   * Workflow step name the session currently belongs to; used as the tab label
   * (paired with the numeric badge) so tabs are ordered and named by step flow.
   * Takes precedence over the agent/model-derived title, but not a user rename.
   */
  stepLabel?: string | null;
  agentLabel: string | null;
  activeModelId: string | null;
  currentModelId: string | null;
  snapshotModel: string | null;
  rank?: number | null;
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
  const extras = args.configOptions
    .filter((option) => !isModelConfigOption(option))
    .map((option) => optionName(option, option.currentValue))
    .filter(Boolean);
  return [modelLabel, ...extras].join(" / ");
}

function withRank(label: string | null, rank: number | null | undefined): string | null {
  if (!label) return null;
  if (!rank || rank < 1) return label;
  return `${label} #${rank}`;
}

export function resolveSessionTabTitle(args: ResolveSessionTabTitleArgs): string | null {
  if (args.customName) return args.customName;
  if (args.stepLabel) return withRank(args.stepLabel, args.rank);
  const liveModelId = args.activeModelId || args.currentModelId;
  const fallback =
    args.agentLabel ??
    resolveModelTitle(args, liveModelId) ??
    resolveModelTitle(args, args.snapshotModel);
  return withRank(fallback, args.rank);
}
