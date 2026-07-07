"use client";

import { memo, useState } from "react";
import { IconCheck, IconChevronDown, IconChevronLeft, IconChevronRight } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Separator } from "@kandev/ui/separator";

export type ModelSelectorOption = {
  id: string;
  name: string;
  description?: string;
  usageMultiplier?: string;
};

export type DynamicConfigOption = {
  type: string;
  id: string;
  name: string;
  currentValue: string;
  category?: string;
  options?: { value: string; name: string }[];
};

export type SelectConfigOption = DynamicConfigOption & {
  options: { value: string; name: string }[];
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
  return option.options.map((item) => ({
    id: item.value,
    name: item.name,
    description: item.value !== item.name ? item.value : undefined,
  }));
}

function currentOptionName(option: DynamicConfigOption): string {
  return (
    option.options?.find((item) => item.value === option.currentValue)?.name ?? option.currentValue
  );
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
): string {
  const modelConfig = configOptions.find(isModelConfigOption);
  const modelValue = modelConfig
    ? currentOptionName(modelConfig)
    : displayModelName(modelOptions, currentModel);
  const extras = configOptions
    .filter((option) => !isModelConfigOption(option))
    .map(currentOptionName)
    .filter(Boolean);
  return [modelValue, ...extras].join(" / ");
}

export function resolveTriggerLabel(
  modelOptions: ModelSelectorOption[],
  currentModel: string | null,
  modelConfig: DynamicConfigOption | undefined,
  configOptions: DynamicConfigOption[],
): string {
  const modelValue = currentModel || modelConfig?.currentValue;
  if (!modelValue) return "";
  return triggerLabel(modelOptions, modelValue, configOptions);
}

function ModelRow({
  model,
  selected,
  onSelect,
}: {
  model: ModelSelectorOption;
  selected: boolean;
  onSelect: (value: string) => void;
}) {
  return (
    <CommandItem
      value={model.id}
      keywords={[model.name, model.description ?? "", model.id]}
      onSelect={() => onSelect(model.id)}
      className="relative pr-7"
    >
      <div className="flex min-w-0 flex-1 items-center">
        <div className="min-w-0 flex-1">
          <div className="truncate">{model.name}</div>
          {model.description && (
            <div className="truncate text-xs text-muted-foreground" title={model.description}>
              {model.description}
            </div>
          )}
        </div>
        {model.usageMultiplier && (
          <span className="shrink-0 text-xs text-muted-foreground">{model.usageMultiplier}</span>
        )}
      </div>
      <IconCheck
        className={cn("absolute right-2 h-4 w-4", selected ? "opacity-100" : "opacity-0")}
      />
    </CommandItem>
  );
}

function ConfigOptionTrigger({
  option,
  onSelect,
}: {
  option: SelectConfigOption;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      data-testid={`config-option-trigger-${option.id}`}
      className="flex min-h-9 w-full cursor-pointer items-center justify-between gap-3 rounded-md px-2.5 py-2 text-left text-xs/relaxed hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/35 focus-visible:outline-none"
      onClick={onSelect}
    >
      <span className="font-medium">{option.name}</span>
      <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
        <span className="truncate">{currentOptionName(option)}</span>
        <IconChevronRight className="h-3.5 w-3.5 shrink-0" />
      </span>
    </button>
  );
}

function ConfigOptionSubSelector({
  option,
  onBack,
  onChange,
}: {
  option: SelectConfigOption;
  onBack: () => void;
  onChange?: (configId: string, value: string) => void;
}) {
  return (
    <div className="flex min-h-0 flex-col gap-2">
      <button
        type="button"
        aria-label={`Back to model settings from ${option.name}`}
        className="flex min-h-9 w-full cursor-pointer items-center gap-2 rounded-md px-2 text-left text-xs/relaxed hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/35 focus-visible:outline-none"
        onClick={onBack}
      >
        <IconChevronLeft className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate font-medium">{option.name}</span>
      </button>
      <ScrollArea
        className="max-h-[min(18rem,calc(100vh-8rem))] pr-2"
        data-testid={`config-option-section-${option.id}`}
      >
        <div className="space-y-1">
          {option.options.map((item) => (
            <Button
              key={item.value}
              type="button"
              variant={item.value === option.currentValue ? "secondary" : "ghost"}
              size="sm"
              className="h-9 w-full min-w-0 cursor-pointer justify-start px-2 text-left"
              disabled={!onChange}
              onClick={() => onChange?.(option.id, item.value)}
            >
              <span className="truncate">{item.name}</span>
              <IconCheck
                className={cn(
                  "ml-auto h-4 w-4 shrink-0",
                  item.value === option.currentValue ? "opacity-100" : "opacity-0",
                )}
              />
            </Button>
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

type ModelConfigSelectorContentProps = {
  activeConfig: SelectConfigOption | undefined;
  modelOptions: ModelSelectorOption[];
  currentModelValue: string;
  extraConfigOptions: SelectConfigOption[];
  onModelSelect: (value: string) => void;
  onConfigSelect: (configId: string) => void;
  onConfigBack: () => void;
  onConfigChange?: (configId: string, value: string) => void;
};

function ModelConfigSelectorContent({
  activeConfig,
  modelOptions,
  currentModelValue,
  extraConfigOptions,
  onModelSelect,
  onConfigSelect,
  onConfigBack,
  onConfigChange,
}: ModelConfigSelectorContentProps) {
  if (activeConfig) {
    return (
      <ConfigOptionSubSelector
        option={activeConfig}
        onBack={onConfigBack}
        onChange={onConfigChange}
      />
    );
  }

  return (
    <>
      <Command>
        <CommandInput placeholder="Filter models..." className="h-8" />
        <CommandList className="max-h-60">
          <CommandEmpty>No models found.</CommandEmpty>
          <CommandGroup heading="Model">
            {modelOptions.map((model) => (
              <ModelRow
                key={model.id}
                model={model}
                selected={model.id === currentModelValue}
                onSelect={onModelSelect}
              />
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
      {extraConfigOptions.length > 0 && (
        <>
          <Separator />
          <ScrollArea className="max-h-40 pr-2">
            <div className="space-y-1">
              {extraConfigOptions.map((option) => (
                <ConfigOptionTrigger
                  key={option.id}
                  option={option}
                  onSelect={() => onConfigSelect(option.id)}
                />
              ))}
            </div>
          </ScrollArea>
        </>
      )}
    </>
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
  popoverSide?: "top" | "bottom";
  triggerClassName?: string;
};

export const ModelConfigSelector = memo(function ModelConfigSelector({
  modelOptions,
  currentModel,
  configOptions = [],
  onModelChange,
  onConfigChange,
  disabled,
  placeholder = "Select model...",
  ariaLabel = "Model settings",
  variant = "field",
  popoverSide = "bottom",
  triggerClassName: customTriggerClassName,
}: ModelConfigSelectorProps) {
  const [open, setOpen] = useState(false);
  const [activeConfigId, setActiveConfigId] = useState<string | null>(null);
  const selectConfigOptions = usableConfigOptions(configOptions);
  const modelConfig = selectConfigOptions.find(isModelConfigOption);
  const extraConfigOptions = selectConfigOptions.filter((option) => !isModelConfigOption(option));
  const activeConfig = extraConfigOptions.find((option) => option.id === activeConfigId);
  const currentModelValue = modelConfig?.currentValue || currentModel || "";
  const label = resolveTriggerLabel(modelOptions, currentModel, modelConfig, configOptions);

  const hasExtraConfigOptions = extraConfigOptions.length > 0;
  const onModelSelect = (value: string) => {
    if (!value) return;
    onModelChange(value);
    if (!hasExtraConfigOptions) {
      setOpen(false);
    }
  };

  const baseTriggerClassName =
    variant === "compact"
      ? "h-7 max-w-[min(18rem,70vw)] cursor-pointer gap-1 px-2 text-xs hover:bg-muted/40"
      : "w-full justify-between font-normal cursor-pointer";
  const onOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) {
      setActiveConfigId(null);
    }
  };

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant={variant === "compact" ? "ghost" : "outline"}
          size={variant === "compact" ? "sm" : "default"}
          className={cn(baseTriggerClassName, customTriggerClassName)}
          aria-label={ariaLabel}
          disabled={disabled}
        >
          <span className="truncate">{label || placeholder}</span>
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 opacity-70" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
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
        />
      </PopoverContent>
    </Popover>
  );
});
