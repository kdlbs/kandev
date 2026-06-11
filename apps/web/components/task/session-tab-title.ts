import {
  displayModelName,
  isModelConfigOption,
  type DynamicConfigOption,
  type ModelSelectorOption,
} from "@/components/model-config-selector";

type ResolveSessionTabTitleArgs = {
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
  const modelLabel = modelConfig
    ? optionName(modelConfig, modelId)
    : displayModelName(args.modelOptions, modelId);
  const extras = args.configOptions
    .filter((option) => !isModelConfigOption(option))
    .map((option) => optionName(option, option.currentValue))
    .filter(Boolean);
  return [modelLabel, ...extras].join(" / ");
}

export function resolveSessionTabTitle(args: ResolveSessionTabTitleArgs): string | null {
  const liveModelId = args.activeModelId || args.currentModelId;
  return (
    resolveModelTitle(args, liveModelId) ??
    args.agentLabel ??
    resolveModelTitle(args, args.snapshotModel)
  );
}
