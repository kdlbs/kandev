"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { defaultFilter } from "cmdk";

import { cn } from "@/lib/utils";
import * as selectorOptions from "@/lib/utils/selector-options";
import type {
  ModelCatalogNavigation,
  ModelSelectorOption,
  SelectConfigOption,
} from "@/components/model-config-selector";
import { buildCatalogTree } from "@/lib/model-catalog-selection";
import type { ModelCatalog } from "@kandev/types/model-catalog";
import {
  CompositionDimensionChoices,
  CompositionDimensionList,
} from "@/components/model-catalog-composition";
import { CatalogCommandGroups } from "@/components/model-catalog-groups";
import { CustomModelRow, ModelRow } from "@/components/model-config-selector-rows";
import { Button } from "@kandev/ui/button";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandList } from "@kandev/ui/command";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Separator } from "@kandev/ui/separator";
import { Spinner } from "@kandev/ui/spinner";

function currentOptionName(option: SelectConfigOption): string {
  return (
    option.options.find((item) => item.value === option.currentValue)?.name ?? option.currentValue
  );
}

/** Tracks the typed filter text and whether it warrants a custom-model row. */
function useModelCustomRow(modelOptions: ModelSelectorOption[], allowCustomModel: boolean) {
  const [filterText, setFilterText] = useState("");
  const trimmedFilter = filterText.trim();
  const showCustomRow =
    allowCustomModel &&
    trimmedFilter.length > 0 &&
    !modelOptions.some((model) => model.id === trimmedFilter);
  return { filterText, setFilterText, trimmedFilter, showCustomRow };
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
  providerIcon?: ReactNode;
  activeConfig: SelectConfigOption | undefined;
  modelOptions: ModelSelectorOption[];
  currentModelValue: string;
  extraConfigOptions: SelectConfigOption[];
  onModelSelect: (value: string) => void;
  onConfigSelect: (configId: string) => void;
  onConfigBack: () => void;
  onConfigChange?: (configId: string, value: string) => void;
  configOptionsLoading: boolean;
  /** Shows the filter input regardless of model count and offers a
   *  "use this as a model ID" row for text with no exact match. */
  allowCustomModel?: boolean;
  /** Optional provider catalog; its absence keeps the flat list unchanged. */
  catalog?: ModelCatalog;
  /** Navigation and draft state owned by the selector so viewport changes keep it. */
  catalogNavigation?: ModelCatalogNavigation;
  /** Uses the host sheet's single scroll region instead of internal scrollers. */
  embedded?: boolean;
};

/**
 * cmdk also scores an item's `value`, which catalog rows use as an opaque
 * occurrence identity. Score only the declared keywords so prefixes such as
 * `row:`/`composition:` and the counted suffix never match unrelated models.
 */
function catalogCommandFilter(value: string, search: string, keywords?: string[]): number {
  return defaultFilter(keywords && keywords.length > 0 ? keywords.join(" ") : value, search, []);
}

/** Adds provider branding while preserving the model group label. */
function ModelFilterInput({
  value,
  onValueChange,
}: {
  value: string;
  onValueChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <CommandInput
      placeholder={t("agents:filterModels")}
      className="h-8"
      value={value}
      onValueChange={onValueChange}
    />
  );
}

function ModelOptionsLoadingRow() {
  const { t } = useTranslation();
  return (
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
  );
}

function ModelGroupHeading({ providerIcon }: { providerIcon?: ReactNode }) {
  const { t } = useTranslation();
  return (
    <span className="flex items-center gap-1.5">
      {providerIcon}
      {t("agents:modelHeading")}
    </span>
  );
}

function renderActiveSubView({
  activeConfig,
  catalogNavigation,
  onConfigBack,
  onConfigChange,
}: {
  activeConfig: SelectConfigOption | undefined;
  catalogNavigation: ModelCatalogNavigation | undefined;
  onConfigBack: () => void;
  onConfigChange?: (configId: string, value: string) => void;
}) {
  if (activeConfig) {
    return (
      <ConfigOptionSubSelector
        option={activeConfig}
        onBack={onConfigBack}
        onChange={onConfigChange}
      />
    );
  }
  const activeComposition = catalogNavigation?.activeComposition;
  if (activeComposition && catalogNavigation?.activeDimensionId) {
    return (
      <CompositionDimensionChoices
        composition={activeComposition}
        dimensionId={catalogNavigation.activeDimensionId}
        draft={catalogNavigation.draft}
        onChange={(value) =>
          catalogNavigation.setDimension(catalogNavigation.activeDimensionId ?? "", value)
        }
        onBack={catalogNavigation.closeDimension}
      />
    );
  }
  if (activeComposition && catalogNavigation) {
    return (
      <CompositionDimensionList
        composition={activeComposition}
        draft={catalogNavigation.draft}
        onSelectDimension={catalogNavigation.openDimension}
        onBack={catalogNavigation.closeComposition}
        onApply={catalogNavigation.apply}
        applyEnabled={catalogNavigation.applyEnabled}
        candidateOptions={catalogNavigation.candidateOptions}
        candidateOptionValues={catalogNavigation.candidateOptionValues}
        onCandidateOptionChange={catalogNavigation.setCandidateOption}
        onRetryCandidateOptions={catalogNavigation.retryCandidateOptions}
      />
    );
  }
  return null;
}

type ModelListBodyProps = {
  providerIcon?: ReactNode;
  catalog?: ModelCatalog;
  catalogNavigation?: ModelCatalogNavigation;
  catalogTree: ReturnType<typeof buildCatalogTree> | undefined;
  orderedModelOptions: ModelSelectorOption[];
  currentModelValue: string;
  configOptionsLoading: boolean;
  embedded: boolean;
  allowCustomModel: boolean;
  showCustomRow: boolean;
  trimmedFilter: string;
  filterText: string;
  onFilterChange: (value: string) => void;
  onModelSelect: (value: string) => void;
};

function ModelListBody({
  providerIcon,
  catalog,
  catalogNavigation,
  catalogTree,
  orderedModelOptions,
  currentModelValue,
  configOptionsLoading,
  embedded,
  allowCustomModel,
  showCustomRow,
  trimmedFilter,
  filterText,
  onFilterChange,
  onModelSelect,
}: ModelListBodyProps) {
  const { t } = useTranslation();
  return (
    <Command filter={catalogCommandFilter}>
      {(allowCustomModel || orderedModelOptions.length > 5 || catalogTree) && (
        <ModelFilterInput value={filterText} onValueChange={onFilterChange} />
      )}
      <CommandList className={cn(catalogTree && "max-h-none", !embedded && "max-h-60")}>
        <CommandEmpty>{t("agents:noModelsFound")}</CommandEmpty>
        {catalogTree && catalogNavigation ? (
          <>
            <CatalogCommandGroups
              tree={catalogTree}
              heading={<ModelGroupHeading providerIcon={providerIcon} />}
              compositions={catalog?.compositions}
              currentModel={currentModelValue}
              loading={configOptionsLoading}
              onSelectModel={onModelSelect}
              onOpenComposition={(group) => catalogNavigation.openComposition(group)}
            />
            {showCustomRow && (
              <CommandGroup>
                <CustomModelRow value={trimmedFilter} onSelect={onModelSelect} />
              </CommandGroup>
            )}
            {orderedModelOptions
              .filter((model) => model.disabled && model.id === currentModelValue)
              .map((model) => (
                <CommandGroup key={model.id}>
                  <ModelRow
                    model={model}
                    selected
                    loading={configOptionsLoading}
                    onSelect={onModelSelect}
                  />
                </CommandGroup>
              ))}
          </>
        ) : (
          <CommandGroup heading={<ModelGroupHeading providerIcon={providerIcon} />}>
            {orderedModelOptions.map((model) => (
              <ModelRow
                key={model.id}
                model={model}
                selected={model.id === currentModelValue}
                loading={configOptionsLoading}
                onSelect={onModelSelect}
              />
            ))}
            {showCustomRow && <CustomModelRow value={trimmedFilter} onSelect={onModelSelect} />}
          </CommandGroup>
        )}
      </CommandList>
    </Command>
  );
}

export function ModelConfigSelectorContent({
  providerIcon,
  activeConfig,
  modelOptions,
  currentModelValue,
  extraConfigOptions,
  onModelSelect,
  onConfigSelect,
  onConfigBack,
  onConfigChange,
  configOptionsLoading,
  allowCustomModel = false,
  catalog,
  catalogNavigation,
  embedded = false,
}: ModelConfigSelectorContentProps) {
  const catalogTree = useMemo(
    () => (catalog ? buildCatalogTree(catalog.nodes) : undefined),
    [catalog],
  );
  const pendingFocusConfigId = useRef<string | null>(null);
  const triggerRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const orderedModelOptions = selectorOptions.prioritizeIdOption(modelOptions, currentModelValue);
  const { filterText, setFilterText, trimmedFilter, showCustomRow } = useModelCustomRow(
    orderedModelOptions,
    allowCustomModel,
  );

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

  const activeView = renderActiveSubView({
    activeConfig,
    catalogNavigation,
    onConfigBack: returnToConfigTrigger,
    onConfigChange,
  });
  if (activeView) return activeView;

  return (
    <>
      <ModelListBody
        providerIcon={providerIcon}
        catalog={catalog}
        catalogNavigation={catalogNavigation}
        catalogTree={catalogTree}
        orderedModelOptions={orderedModelOptions}
        currentModelValue={currentModelValue}
        configOptionsLoading={configOptionsLoading}
        embedded={embedded}
        allowCustomModel={allowCustomModel}
        showCustomRow={showCustomRow}
        trimmedFilter={trimmedFilter}
        filterText={filterText}
        onFilterChange={setFilterText}
        onModelSelect={onModelSelect}
      />
      {configOptionsLoading ? (
        <ModelOptionsLoadingRow />
      ) : (
        extraConfigOptions.length > 0 && (
          <>
            <Separator />
            <ScrollArea className={embedded ? "pr-2" : "max-h-40 pr-2"}>
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
