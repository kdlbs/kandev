"use client";

import { useTranslation } from "react-i18next";
import { IconCheck, IconChevronLeft, IconChevronRight } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import * as selectorOptions from "@/lib/utils/selector-options";
import type {
  ModelCatalogComposition,
  ModelCatalogDimension,
  ModelCatalogDraft,
} from "@kandev/types/model-catalog";
import {
  composeCatalogSelection,
  dimensionLabel,
  invalidDraftDimensions,
  isDimensionChoiceAvailable,
} from "@/lib/model-catalog-selection";
import type {
  CandidateOptionsState,
  ConfigurationOption,
} from "@/hooks/use-model-catalog-navigation";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";

// Phone and coarse-pointer surfaces keep 44px targets through the 768px layout
// boundary; fine-pointer desktop controls use the shared 28px density.
const TOUCH_TARGET_CLASS = "min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";

function chosenLabel(
  dimension: ModelCatalogDimension,
  draft: ModelCatalogDraft,
): string | undefined {
  const value = draft[dimension.id];
  if (!value) return undefined;
  return dimension.choices.find((choice) => choice.value === value)?.label ?? value;
}

function CompositionBackButton({
  label,
  onBack,
  testId,
}: {
  label: string;
  onBack: () => void;
  testId: string;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      data-testid={testId}
      autoFocus
      className={cn(
        "flex w-full cursor-pointer items-center gap-2 rounded-md px-2 text-left text-xs/relaxed hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/35 focus-visible:outline-none",
        TOUCH_TARGET_CLASS,
      )}
      onClick={onBack}
    >
      <IconChevronLeft className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate font-medium">{label}</span>
    </button>
  );
}

function compositionErrorMessage(
  status: "complete" | "incomplete" | "invalid" | "ambiguous",
  hasInvalid: boolean,
  t: (key: string) => string,
): string | undefined {
  if (status === "ambiguous") return t("agents:modelConfigurationAmbiguous");
  if (status === "invalid" || hasInvalid) return t("agents:modelConfigurationInvalid");
  if (status === "incomplete") return t("agents:modelConfigurationIncomplete");
  return undefined;
}

function ProviderOptionRow({
  option,
  value,
  onChange,
}: {
  option: ConfigurationOption;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div
      className="flex items-center justify-between gap-3 px-2"
      data-testid={`model-provider-option-${option.id}`}
    >
      <span className="min-w-0">
        <span className="block text-xs font-medium">{option.name || option.id}</span>
        {option.description && (
          <span className="block text-xs text-muted-foreground">{option.description}</span>
        )}
      </span>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger
          aria-label={option.name || option.id}
          data-testid={`model-provider-option-select-${option.id}`}
          className={cn("w-auto min-w-24 cursor-pointer text-xs", TOUCH_TARGET_CLASS)}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {(option.options ?? []).map((choice) => (
            <SelectItem key={choice.value} value={choice.value} className="cursor-pointer">
              <span className="flex flex-col">
                <span>{choice.name || choice.value}</span>
                {choice.description && (
                  <span className="text-xs text-muted-foreground">{choice.description}</span>
                )}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function ProviderOptionsSection({
  state,
  values,
  onChange,
  onRetry,
}: {
  state: CandidateOptionsState;
  values: Record<string, string>;
  onChange: (optionId: string, value: string) => void;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  if (state.status === "idle" || (state.status === "ready" && state.options.length === 0)) {
    return null;
  }
  return (
    <div className="space-y-1 border-t pt-2" data-testid="model-provider-options">
      <p className="px-2 text-xs font-medium text-muted-foreground">
        {t("agents:providerOptionsHeading")}
      </p>
      {state.status === "loading" && (
        <div
          className="flex min-h-7 items-center gap-2 px-2 text-xs text-muted-foreground max-md:min-h-11"
          role="status"
          data-testid="model-provider-options-loading"
        >
          <Spinner aria-hidden="true" className="h-3.5 w-3.5" />
          <span>{t("agents:loadingProviderOptions")}</span>
        </div>
      )}
      {state.status === "failed" && (
        <div
          className="flex flex-wrap items-center gap-2 px-2"
          data-testid="model-provider-options-failed"
        >
          <p className="text-xs text-destructive" role="alert">
            {t("agents:providerOptionsFailed")}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={cn("cursor-pointer px-2 text-xs", TOUCH_TARGET_CLASS)}
            data-testid="model-provider-options-retry"
            onClick={onRetry}
          >
            {t("agents:retry")}
          </Button>
        </div>
      )}
      {state.status === "ready" &&
        state.options.map((option) => (
          <ProviderOptionRow
            key={option.id}
            option={option}
            value={values[option.id] ?? option.currentValue}
            onChange={(value) => onChange(option.id, value)}
          />
        ))}
    </div>
  );
}

export type CompositionDimensionListProps = {
  composition: ModelCatalogComposition;
  draft: ModelCatalogDraft;
  onSelectDimension: (dimensionId: string) => void;
  onBack: () => void;
  onApply: () => void;
  applyEnabled: boolean;
  candidateOptions: CandidateOptionsState;
  candidateOptionValues: Record<string, string>;
  onCandidateOptionChange: (optionId: string, value: string) => void;
  onRetryCandidateOptions: () => void;
};

/**
 * Desktop/phone composition body: one row per catalog dimension, the candidate
 * model's real provider options, and the single explicit Apply action. The
 * renderer never recognizes a vendor dimension id.
 */
export function CompositionDimensionList({
  composition,
  draft,
  onSelectDimension,
  onBack,
  onApply,
  applyEnabled,
  candidateOptions,
  candidateOptionValues,
  onCandidateOptionChange,
  onRetryCandidateOptions,
}: CompositionDimensionListProps) {
  const { t } = useTranslation();
  const invalid = new Set(invalidDraftDimensions(composition, draft));
  const resolution = composeCatalogSelection(composition, draft);
  const error = compositionErrorMessage(resolution.status, invalid.size > 0, t);

  return (
    <div className="flex min-h-0 flex-col gap-2" data-testid="model-composition-view">
      <CompositionBackButton
        label={t("agents:backToModelSettings", { name: composition.label || composition.id })}
        onBack={onBack}
        testId="model-composition-back"
      />
      <div className="space-y-1">
        {composition.dimensions.map((dimension) => {
          const value = chosenLabel(dimension, draft);
          const invalidDimension = invalid.has(dimension.id);
          return (
            <button
              key={dimension.id}
              type="button"
              data-testid={`model-composition-dimension-${dimension.id}`}
              aria-invalid={invalidDimension || undefined}
              aria-label={
                value
                  ? t("agents:modelConfigurationDimensionValue", {
                      name: dimensionLabel(dimension),
                      value,
                    })
                  : t("agents:modelConfigurationChooseDimension", {
                      name: dimensionLabel(dimension),
                    })
              }
              className={cn(
                "flex w-full cursor-pointer items-center justify-between gap-3 rounded-md px-2.5 py-2 text-left text-xs/relaxed hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/35 focus-visible:outline-none",
                TOUCH_TARGET_CLASS,
                invalidDimension && "text-destructive",
              )}
              onClick={() => onSelectDimension(dimension.id)}
            >
              <span className="min-w-0 flex-1">
                <span className="block font-medium">{dimensionLabel(dimension)}</span>
              </span>
              <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
                <span className="truncate">{value ?? t("agents:modelConfigurationNotChosen")}</span>
                <IconChevronRight className="h-3.5 w-3.5 shrink-0" />
              </span>
            </button>
          );
        })}
      </div>
      <ProviderOptionsSection
        state={candidateOptions}
        values={candidateOptionValues}
        onChange={onCandidateOptionChange}
        onRetry={onRetryCandidateOptions}
      />
      {error && (
        <p
          className="px-2 text-xs text-destructive"
          role="alert"
          data-testid="model-composition-error"
        >
          {error}
        </p>
      )}
      <div className="sticky bottom-0 -mx-2 mt-1 bg-background/95 px-2 pb-[env(safe-area-inset-bottom)] pt-1 backdrop-blur">
        <Button
          type="button"
          size="sm"
          className={cn("w-full cursor-pointer", TOUCH_TARGET_CLASS)}
          disabled={!applyEnabled}
          data-testid="model-composition-apply"
          onClick={onApply}
        >
          {t("agents:useModelConfiguration")}
        </Button>
      </div>
    </div>
  );
}

export type CompositionDimensionChoicesProps = {
  composition: ModelCatalogComposition;
  dimensionId: string;
  draft: ModelCatalogDraft;
  onChange: (value: string) => void;
  onBack: () => void;
};

/** Choice list that replaces the composition body for one dimension. */
export function CompositionDimensionChoices({
  composition,
  dimensionId,
  draft,
  onChange,
  onBack,
}: CompositionDimensionChoicesProps) {
  const { t } = useTranslation();
  const dimension = composition.dimensions.find((entry) => entry.id === dimensionId);
  if (!dimension) return null;
  const selected = draft[dimension.id];
  const choices = selectorOptions.prioritizeValueOption(dimension.choices, selected);
  return (
    <div className="flex min-h-0 flex-col gap-2" data-testid="model-composition-choices">
      <CompositionBackButton
        label={t("agents:backToModelSettings", { name: dimensionLabel(dimension) })}
        onBack={onBack}
        testId="model-composition-choices-back"
      />
      <div className="max-h-[min(18rem,calc(100dvh-10rem))] space-y-1 overflow-y-auto overscroll-contain pr-2">
        {choices.map((choice) => {
          const available = isDimensionChoiceAvailable(composition, dimension.id, choice.value);
          const isSelected = choice.value === selected;
          return (
            <Button
              key={choice.value}
              type="button"
              variant={isSelected ? "secondary" : "ghost"}
              size="sm"
              data-testid={`model-composition-choice-${dimension.id}-${choice.value}`}
              aria-pressed={isSelected}
              aria-label={choice.label}
              disabled={!available}
              className={cn(
                "h-auto w-full min-w-0 cursor-pointer justify-start border border-transparent px-2 py-2 text-left",
                TOUCH_TARGET_CLASS,
                isSelected && "border-primary/50 bg-card font-medium hover:bg-card",
              )}
              onClick={() => {
                onChange(choice.value);
                onBack();
              }}
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate">{choice.label}</span>
                {choice.description && (
                  <span className="block whitespace-normal text-xs text-muted-foreground">
                    {choice.description}
                  </span>
                )}
              </span>
              <IconCheck
                className={cn("ml-auto h-4 w-4 shrink-0", isSelected ? "opacity-100" : "opacity-0")}
              />
            </Button>
          );
        })}
      </div>
    </div>
  );
}
