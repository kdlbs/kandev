"use client";

import { useId } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import * as selectorOptions from "@/lib/utils/selector-options";
import type { ModelSelectorOption } from "@/components/model-config-selector";
import { CommandItem } from "@kandev/ui/command";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

function sourceLabelKeyFor(source?: string) {
  if (source === "cli") return "agents:modelSourceCli";
  if (source === "acp") return "agents:modelSourceAcp";
  return undefined;
}

function ModelRowLabels({
  model,
  detail,
  descriptionId,
  detailId,
}: {
  model: ModelSelectorOption;
  detail?: string;
  descriptionId: string;
  detailId: string;
}) {
  const { t } = useTranslation();
  const labelKey = sourceLabelKeyFor(model.source);
  return (
    <div className="min-w-0 flex-1">
      <div className="flex items-center gap-2">
        <span className="truncate">{model.name}</span>
        {labelKey && <span className="shrink-0 text-xs text-muted-foreground">{t(labelKey)}</span>}
      </div>
      {model.description && (
        <div
          id={descriptionId}
          className="truncate text-xs text-muted-foreground"
          title={model.description}
        >
          {model.description}
        </div>
      )}
      {detail && (
        <div
          id={detailId}
          className="truncate text-xs text-muted-foreground max-md:whitespace-normal max-md:break-words [@media(pointer:coarse)]:whitespace-normal [@media(pointer:coarse)]:break-words"
          title={detail}
          data-testid="model-row-context"
        >
          {detail}
        </div>
      )}
    </div>
  );
}

function ModelRowIndicator({ selected, loading }: { selected: boolean; loading: boolean }) {
  if (selected && loading) {
    return <Spinner aria-hidden="true" className="absolute right-2" />;
  }
  return (
    <IconCheck className={cn("absolute right-2 h-4 w-4", selected ? "opacity-100" : "opacity-0")} />
  );
}

export type ModelRowProps = {
  model: ModelSelectorOption;
  selected: boolean;
  loading: boolean;
  onSelect: (value: string) => void;
  /** Command identity; repeated provider values stay distinct occurrences. */
  itemValue?: string;
  /** Removed hierarchy that no longer has its own heading in the projection. */
  detail?: string;
};

export function ModelRow({ model, selected, loading, onSelect, itemValue, detail }: ModelRowProps) {
  const descriptionId = useId();
  const detailId = useId();
  const describedByIds = [model.description ? descriptionId : "", detail ? detailId : ""].filter(
    Boolean,
  );
  const item = (
    <CommandItem
      value={itemValue ?? model.id}
      keywords={[
        model.name,
        model.description ?? "",
        model.id,
        ...(model.keywords ?? []),
        detail ?? "",
      ]}
      onSelect={() => !model.disabled && onSelect(model.id)}
      disabled={model.disabled}
      aria-label={model.name}
      aria-describedby={describedByIds.length > 0 ? describedByIds.join(" ") : undefined}
      data-testid={selected ? "model-config-selected-row" : undefined}
      className={selectorOptions.selectorOptionClassName(selected, model.disabled)}
    >
      <div className="flex min-w-0 flex-1 items-center">
        <ModelRowLabels
          model={model}
          detail={detail}
          descriptionId={descriptionId}
          detailId={detailId}
        />
        {model.usageMultiplier && (
          <span className="shrink-0 text-xs text-muted-foreground">{model.usageMultiplier}</span>
        )}
      </div>
      <ModelRowIndicator selected={selected} loading={loading} />
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

/**
 * The last row of the model list while `allowCustomModel` is set and the
 * typed filter text has no exact match: selects the typed text verbatim.
 */
export function CustomModelRow({
  value,
  onSelect,
}: {
  value: string;
  onSelect: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <CommandItem
      value={value}
      data-testid="model-config-custom-row"
      onSelect={() => onSelect(value)}
      className={selectorOptions.selectorOptionClassName(false, false)}
    >
      <div className="min-w-0 flex-1 truncate">
        {t("agents:useCustomModelId", { model: value })}
      </div>
    </CommandItem>
  );
}
