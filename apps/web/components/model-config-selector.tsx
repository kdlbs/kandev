"use client";

import { memo, useCallback, useState } from "react";
import { IconCheck, IconChevronDown } from "@tabler/icons-react";

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

function displayModelName(modelOptions: ModelSelectorOption[], currentModel: string): string {
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

function ConfigOptionSection({
  option,
  onChange,
}: {
  option: SelectConfigOption;
  onChange?: (configId: string, value: string) => void;
}) {
  return (
    <div className="space-y-1.5">
      <div className="text-[0.6875rem] font-medium uppercase tracking-wide text-muted-foreground">
        {option.name}
      </div>
      <div className="grid grid-cols-2 gap-1">
        {option.options.map((item) => (
          <Button
            key={item.value}
            type="button"
            variant={item.value === option.currentValue ? "secondary" : "ghost"}
            size="sm"
            className="h-9 min-w-0 justify-start px-2 text-left"
            disabled={!onChange}
            onClick={() => onChange?.(option.id, item.value)}
          >
            <span className="truncate">{item.name}</span>
          </Button>
        ))}
      </div>
    </div>
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
}: ModelConfigSelectorProps) {
  const [open, setOpen] = useState(false);
  const selectConfigOptions = usableConfigOptions(configOptions);
  const modelConfig = selectConfigOptions.find(isModelConfigOption);
  const extraConfigOptions = selectConfigOptions.filter((option) => !isModelConfigOption(option));
  const currentModelValue = modelConfig?.currentValue || currentModel || "";
  const label = resolveTriggerLabel(modelOptions, currentModel, modelConfig, configOptions);

  const onModelSelect = useCallback(
    (value: string) => {
      if (!value) return;
      onModelChange(value);
    },
    [onModelChange],
  );

  const triggerClassName =
    variant === "compact"
      ? "h-7 max-w-[min(18rem,70vw)] cursor-pointer gap-1 px-2 text-xs hover:bg-muted/40"
      : "w-full justify-between font-normal cursor-pointer";

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant={variant === "compact" ? "ghost" : "outline"}
          size={variant === "compact" ? "sm" : "default"}
          className={triggerClassName}
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
        className="w-[min(24rem,calc(100vw-1rem))] gap-2 p-2"
      >
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
            <ScrollArea className="max-h-56 pr-2">
              <div className="space-y-3">
                {extraConfigOptions.map((option) => (
                  <ConfigOptionSection key={option.id} option={option} onChange={onConfigChange} />
                ))}
              </div>
            </ScrollArea>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
});
