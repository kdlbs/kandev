"use client";

import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconChevronDown } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import { settingsControlClassName } from "@/components/settings/settings-control";
import { ModelConfigSelectorContent } from "@/components/model-config-selector-content";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export type ModelSelectorOption = {
  id: string;
  name: string;
  description?: string;
  usageMultiplier?: string;
  /** Unavailable models are dimmed and non-selectable with a reason tooltip. */
  disabled?: boolean;
  disabledReason?: string;
};

export type DynamicConfigOption = {
  type: string;
  id: string;
  name: string;
  description?: string;
  currentValue: string;
  category?: string;
  options?: { value: string; name: string; description?: string }[];
};

export type SelectConfigOption = DynamicConfigOption & {
  options: { value: string; name: string; description?: string }[];
};

type TriggerLabelOptions = {
  summary: "changed";
  configBaseline?: Record<string, string>;
  /** Appended to the trigger's model label (e.g. "(fallback)"). */
  currentModelSuffix?: string;
};

type TriggerDetail = {
  id: string;
  name: string;
  value: string;
};

const MODEL_CONFIG_CATEGORY = "model";
const MODE_CONFIG_CATEGORY = "mode";

export function isModelConfigOption(option: Pick<DynamicConfigOption, "id" | "category">): boolean {
  return option.id === MODEL_CONFIG_CATEGORY || option.category === MODEL_CONFIG_CATEGORY;
}

export function isModeConfigOption(option: Pick<DynamicConfigOption, "id" | "category">): boolean {
  return option.id === MODE_CONFIG_CATEGORY || option.category === MODE_CONFIG_CATEGORY;
}

export function usableConfigOptions(
  options: DynamicConfigOption[] | undefined,
): SelectConfigOption[] {
  return (options ?? []).filter(
    (option): option is SelectConfigOption =>
      option.type === "select" &&
      !isModeConfigOption(option) &&
      Array.isArray(option.options) &&
      option.options.length > 0,
  );
}

export function configOptionToModelOptions(
  option: SelectConfigOption | undefined,
): ModelSelectorOption[] {
  if (!option) return [];
  const seen = new Set<string>();
  return option.options.flatMap((item) => {
    if (seen.has(item.value)) return [];
    seen.add(item.value);
    return [
      {
        id: item.value,
        name: item.name,
        description: item.description ?? (item.value !== item.name ? item.value : undefined),
      },
    ];
  });
}

function currentOptionValue(option: DynamicConfigOption) {
  return option.options?.find((item) => item.value === option.currentValue);
}

function currentOptionName(option: DynamicConfigOption): string {
  return currentOptionValue(option)?.name ?? option.currentValue;
}

export function displayModelName(
  modelOptions: ModelSelectorOption[],
  currentModel: string,
): string {
  return modelOptions.find((m) => m.id === currentModel)?.name ?? currentModel;
}

export function triggerLabel(
  modelOptions: ModelSelectorOption[],
  currentModel: string,
  configOptions: DynamicConfigOption[],
  options?: TriggerLabelOptions,
): string {
  const modelConfig = configOptions.find(isModelConfigOption);
  const modelValue =
    (modelConfig ? currentOptionName(modelConfig) : displayModelName(modelOptions, currentModel)) +
    (options?.currentModelSuffix ?? "");
  const baseline = options?.configBaseline;
  const extras = configOptions
    .filter((option) => !isModelConfigOption(option))
    .filter(
      (option) =>
        !options ||
        baseline === undefined ||
        !Object.hasOwn(baseline, option.id) ||
        baseline[option.id] !== option.currentValue,
    )
    .map(currentOptionName)
    .filter(Boolean);
  return [modelValue, ...extras].join(" / ");
}

export function resolveTriggerLabel(
  modelOptions: ModelSelectorOption[],
  currentModel: string | null,
  modelConfig: DynamicConfigOption | undefined,
  configOptions: DynamicConfigOption[],
  options?: TriggerLabelOptions,
): string {
  const modelValue = currentModel || modelConfig?.currentValue;
  if (!modelValue) return "";
  return triggerLabel(modelOptions, modelValue, configOptions, options);
}

function triggerDetails(
  modelOptions: ModelSelectorOption[],
  currentModel: string | null,
  modelConfig: SelectConfigOption | undefined,
  extraConfigOptions: SelectConfigOption[],
  t: TFunction,
): TriggerDetail[] {
  let modelValue = "";
  if (modelConfig) {
    modelValue = currentOptionName(modelConfig);
  } else if (currentModel) {
    modelValue = displayModelName(modelOptions, currentModel);
  }
  const details = modelValue
    ? [
        {
          id: modelConfig?.id || MODEL_CONFIG_CATEGORY,
          name: modelConfig?.name || t("common:model"),
          value: modelValue,
        },
      ]
    : [];
  return details.concat(
    extraConfigOptions.map((option) => ({
      id: option.id,
      name: option.name || option.id,
      value: currentOptionName(option),
    })),
  );
}

export type ModelConfigSelectorProps = {
  modelOptions: ModelSelectorOption[];
  currentModel: string | null;
  configOptions?: DynamicConfigOption[];
  onModelChange: (modelId: string) => void;
  onConfigChange?: (configId: string, value: string) => void;
  disabled?: boolean;
  placeholder?: string;
  ariaLabel?: string;
  variant?: "compact" | "field";
  /** Aligns the picker with the trigger edge for context-specific settings surfaces. */
  popoverAlign?: "start" | "center" | "end";
  popoverSide?: "top" | "bottom";
  triggerClassName?: string;
  triggerSummary?: "all" | "changed";
  configBaseline?: Record<string, string>;
  /** Optional suffix appended to the trigger's model label (e.g. "(fallback)"). */
  currentModelSuffix?: string;

  /** Keeps the picker open while a caller resolves model-dependent options. */
  configOptionsLoading?: boolean;
  /** Keeps the picker open after model selection, even without existing options. */
  keepOpenOnModelChange?: boolean;

  /** Optional title tooltip on the trigger (e.g. explains a live-only note). */
  triggerTitle?: string;
};

type ModelConfigSelectorTriggerProps = Pick<
  ModelConfigSelectorProps,
  "ariaLabel" | "disabled" | "placeholder" | "triggerClassName" | "triggerTitle" | "variant"
> & {
  label: string;
  details?: TriggerDetail[];
};

function ModelConfigSelectorTrigger({
  ariaLabel,
  details,
  disabled,
  label,
  placeholder,
  triggerClassName,
  triggerTitle,
  variant,
}: ModelConfigSelectorTriggerProps) {
  const compact = variant === "compact";
  const baseClassName = compact
    ? "h-7 max-w-[min(18rem,70vw)] cursor-pointer gap-1 px-2 text-xs hover:bg-muted/40"
    : settingsControlClassName("w-full justify-between font-normal cursor-pointer");
  const trigger = (
    <PopoverTrigger asChild>
      <Button
        type="button"
        variant={compact ? "ghost" : "outline"}
        size={compact ? "sm" : "default"}
        className={cn(baseClassName, triggerClassName)}
        aria-label={ariaLabel}
        disabled={disabled}
        title={triggerTitle}
      >
        <span className="truncate">{label || placeholder}</span>
        <IconChevronDown className="h-3.5 w-3.5 shrink-0 opacity-70" />
      </Button>
    </PopoverTrigger>
  );
  if (!details?.length) return trigger;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent>
        <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-2 gap-y-1">
          {details.map((detail) => (
            <div key={detail.id} className="contents">
              <span className="font-medium">{detail.name}: </span>
              <span className="min-w-0 break-words">{detail.value}</span>
            </div>
          ))}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

function triggerLabelOptions(
  triggerSummary: "all" | "changed",
  configBaseline: Record<string, string> | undefined,
  currentModelSuffix?: string,
): TriggerLabelOptions | undefined {
  if (triggerSummary !== "changed") return undefined;
  return { summary: "changed", configBaseline, currentModelSuffix };
}

export const ModelConfigSelector = memo(function ModelConfigSelector({
  modelOptions,
  currentModel,
  configOptions = [],
  onModelChange,
  onConfigChange,
  disabled,
  placeholder,
  ariaLabel,
  variant = "field",
  popoverAlign = "end",
  popoverSide = "bottom",
  triggerClassName: customTriggerClassName,
  triggerSummary = "all",
  configBaseline,
  currentModelSuffix,
  configOptionsLoading = false,
  keepOpenOnModelChange = false,
}: ModelConfigSelectorProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [activeConfigId, setActiveConfigId] = useState<string | null>(null);
  const selectConfigOptions = usableConfigOptions(configOptions);
  const modelConfig = selectConfigOptions.find(isModelConfigOption);
  const extraConfigOptions = selectConfigOptions.filter((option) => !isModelConfigOption(option));
  const activeConfig = extraConfigOptions.find((option) => option.id === activeConfigId);
  const currentModelValue = modelConfig?.currentValue || currentModel || "";
  const label = resolveTriggerLabel(
    modelOptions,
    currentModel,
    modelConfig,
    configOptions,
    triggerLabelOptions(triggerSummary, configBaseline, currentModelSuffix),
  );
  const details =
    triggerSummary === "changed"
      ? triggerDetails(modelOptions, currentModel, modelConfig, extraConfigOptions, t)
      : undefined;
  // The fallback marker is live-only, so explain when it is active.
  const triggerTitle = currentModelSuffix ? t("settings:fallbackNoteLive") : undefined;

  const hasExtraConfigOptions = extraConfigOptions.length > 0;
  const onModelSelect = (value: string) => {
    if (!value) return;
    onModelChange(value);
    if (!keepOpenOnModelChange && !hasExtraConfigOptions && !configOptionsLoading) {
      setOpen(false);
    }
  };

  const onOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) {
      setActiveConfigId(null);
    }
  };

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <ModelConfigSelectorTrigger
        ariaLabel={ariaLabel ?? t("agents:modelSettings")}
        details={details}
        disabled={disabled}
        label={label}
        placeholder={placeholder ?? t("agents:selectModel")}
        triggerClassName={customTriggerClassName}
        triggerTitle={triggerTitle}
        variant={variant}
      />
      <PopoverContent
        align={popoverAlign}
        side={popoverSide}
        className="w-[min(24rem,calc(100vw-1rem))] max-h-[min(32rem,calc(100vh-1rem))] gap-2 overflow-hidden p-2"
      >
        <ModelConfigSelectorContent
          activeConfig={activeConfig}
          modelOptions={modelOptions}
          currentModelValue={currentModelValue}
          extraConfigOptions={extraConfigOptions}
          onModelSelect={onModelSelect}
          onConfigSelect={setActiveConfigId}
          onConfigBack={() => setActiveConfigId(null)}
          onConfigChange={onConfigChange}
          configOptionsLoading={configOptionsLoading}
        />
      </PopoverContent>
    </Popover>
  );
});
