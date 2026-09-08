import {
  displayModelName,
  isModelConfigOption,
  type DynamicConfigOption,
  type ModelSelectorOption,
} from "@/components/model-config-selector";

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
