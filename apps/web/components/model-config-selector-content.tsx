"use client";

import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconChevronLeft, IconChevronRight } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import * as selectorOptions from "@/lib/utils/selector-options";
import type { ModelSelectorOption, SelectConfigOption } from "@/components/model-config-selector";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Separator } from "@kandev/ui/separator";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

function currentOptionName(option: SelectConfigOption): string {
  return (
    option.options.find((item) => item.value === option.currentValue)?.name ?? option.currentValue
  );
}

function ModelRow({
  model,
  selected,
  loading,
  onSelect,
}: {
  model: ModelSelectorOption;
  selected: boolean;
  loading: boolean;
  onSelect: (value: string) => void;
}) {
  const item = (
    <CommandItem
      value={model.id}
      keywords={[model.name, model.description ?? "", model.id]}
      onSelect={() => !model.disabled && onSelect(model.id)}
      disabled={model.disabled}
      data-testid={selected ? "model-config-selected-row" : undefined}
      className={selectorOptions.selectorOptionClassName(selected, model.disabled)}
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
      {selected && loading ? (
        <Spinner aria-hidden="true" className="absolute right-2" />
      ) : (
        <IconCheck
          className={cn("absolute right-2 h-4 w-4", selected ? "opacity-100" : "opacity-0")}
        />
      )}
    </CommandItem>
  );
  // cmdk's CommandItem swallows pointer events with no native tooltip slot;
  // keep disabled items in a focusable wrapper so their unavailable reason is reachable.
  if (model.disabled && model.disabledReason) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            tabIndex={0}
            aria-label={`${model.name}: ${model.disabledReason}`}
            className="rounded outline-none focus-visible:ring-2 focus-visible:ring-ring/35"
          >
            {item}
          </div>
        </TooltipTrigger>
        <TooltipContent side="right">{model.disabledReason}</TooltipContent>
      </Tooltip>
    );
  }
  return item;
}

function ConfigOptionTrigger({
  option,
  onSelect,
  triggerRef,
}: {
  option: SelectConfigOption;
  onSelect: () => void;
  triggerRef?: (element: HTMLButtonElement | null) => void;
}) {
  return (
    <button
      type="button"
      ref={triggerRef}
      data-testid={`config-option-trigger-${option.id}`}
      className="flex min-h-9 w-full cursor-pointer items-center justify-between gap-3 rounded-md px-2.5 py-2 text-left text-xs/relaxed hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/35 focus-visible:outline-none"
      onClick={onSelect}
    >
      <span className="min-w-0 flex-1">
        <span className="block font-medium">{option.name}</span>
      </span>
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
  const { t } = useTranslation();
  const orderedOptions = selectorOptions.prioritizeValueOption(option.options, option.currentValue);
  return (
    <div className="flex min-h-0 flex-col gap-2">
      <button
        type="button"
        aria-label={t("agents:backToModelSettings", { name: option.name })}
        autoFocus
        className="flex min-h-9 w-full cursor-pointer items-center gap-2 rounded-md px-2 text-left text-xs/relaxed hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/35 focus-visible:outline-none"
        onClick={onBack}
      >
        <IconChevronLeft className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0">
          <span className="block truncate font-medium">{option.name}</span>
          {option.description && (
            <span className="block whitespace-normal text-muted-foreground">
              {option.description}
            </span>
          )}
        </span>
      </button>
      <div
        className="max-h-[min(18rem,calc(100dvh-8rem))] overflow-y-auto overscroll-contain pr-2"
        data-testid={`config-option-section-${option.id}`}
      >
        <div className="space-y-1">
          {orderedOptions.map((item, index) => {
            const descriptionId = item.description
              ? `config-option-value-description-${option.id}-${index}`
              : undefined;
            return (
              <Button
                key={item.value}
                type="button"
                aria-label={item.name}
                aria-describedby={descriptionId}
                variant={item.value === option.currentValue ? "secondary" : "ghost"}
                size="sm"
                className={selectorOptions.configClassName(item.value === option.currentValue)}
                disabled={!onChange}
                onClick={() => {
                  onChange?.(option.id, item.value);
                  onBack();
                }}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{item.name}</span>
                  {item.description && (
                    <span
                      id={descriptionId}
                      className="block whitespace-normal text-xs text-muted-foreground"
                    >
                      {item.description}
                    </span>
                  )}
                </span>
                <IconCheck
                  className={cn(
                    "ml-auto h-4 w-4 shrink-0",
                    item.value === option.currentValue ? "opacity-100" : "opacity-0",
                  )}
                />
              </Button>
            );
          })}
        </div>
      </div>
    </div>
  );
}

export type ModelConfigSelectorContentProps = {
  activeConfig: SelectConfigOption | undefined;
  modelOptions: ModelSelectorOption[];
  currentModelValue: string;
  extraConfigOptions: SelectConfigOption[];
  onModelSelect: (value: string) => void;
  onConfigSelect: (configId: string) => void;
  onConfigBack: () => void;
  onConfigChange?: (configId: string, value: string) => void;
  configOptionsLoading: boolean;
};

export function ModelConfigSelectorContent({
  activeConfig,
  modelOptions,
  currentModelValue,
  extraConfigOptions,
  onModelSelect,
  onConfigSelect,
  onConfigBack,
  onConfigChange,
  configOptionsLoading,
}: ModelConfigSelectorContentProps) {
  const { t } = useTranslation();
  const pendingFocusConfigId = useRef<string | null>(null);
  const triggerRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const showModelFilter = modelOptions.length > 5;
  const orderedModelOptions = selectorOptions.prioritizeIdOption(modelOptions, currentModelValue);

  useEffect(() => {
    if (activeConfig) return;
    const configId = pendingFocusConfigId.current;
    if (!configId) return;
    pendingFocusConfigId.current = null;
    triggerRefs.current[configId]?.focus();
  }, [activeConfig]);

  const returnToConfigTrigger = () => {
    if (activeConfig) {
      pendingFocusConfigId.current = activeConfig.id;
    }
    onConfigBack();
  };

  if (activeConfig) {
    return (
      <ConfigOptionSubSelector
        option={activeConfig}
        onBack={returnToConfigTrigger}
        onChange={onConfigChange}
      />
    );
  }

  return (
    <>
      <Command>
        {showModelFilter && <CommandInput placeholder={t("agents:filterModels")} className="h-8" />}
        <CommandList className="max-h-60">
          <CommandEmpty>{t("agents:noModelsFound")}</CommandEmpty>
          <CommandGroup heading={t("agents:modelHeading")}>
            {orderedModelOptions.map((model) => (
              <ModelRow
                key={model.id}
                model={model}
                selected={model.id === currentModelValue}
                loading={configOptionsLoading}
                onSelect={onModelSelect}
              />
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
      {configOptionsLoading ? (
        <>
          <Separator />
          <div
            className="flex min-h-9 items-center gap-2 px-2 text-xs text-muted-foreground"
            data-testid="model-config-options-loading"
            role="status"
            aria-label={t("agents:resolvingModelOptions")}
          >
            <Spinner aria-hidden="true" className="h-3.5 w-3.5" />
            <span aria-hidden="true">{t("agents:resolvingModelOptions")}</span>
          </div>
        </>
      ) : (
        extraConfigOptions.length > 0 && (
          <>
            <Separator />
            <ScrollArea className="max-h-40 pr-2">
              <div className="space-y-1">
                {extraConfigOptions.map((option) => (
                  <ConfigOptionTrigger
                    key={option.id}
                    option={option}
                    onSelect={() => onConfigSelect(option.id)}
                    triggerRef={(element) => {
                      triggerRefs.current[option.id] = element;
                    }}
                  />
                ))}
              </div>
            </ScrollArea>
          </>
        )
      )}
    </>
  );
}
