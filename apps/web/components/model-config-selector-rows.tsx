"use client";

import { useTranslation } from "react-i18next";
import { IconCheck } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import * as selectorOptions from "@/lib/utils/selector-options";
import type { ModelSelectorOption } from "@/components/model-config-selector";
import { CommandItem } from "@kandev/ui/command";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export function ModelRow({
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
  const { t } = useTranslation();
  const item = (
    <CommandItem
      value={model.id}
      keywords={[model.name, model.description ?? "", model.id, ...(model.keywords ?? [])]}
      onSelect={() => !model.disabled && onSelect(model.id)}
      disabled={model.disabled}
      aria-label={model.name}
      data-testid={selected ? "model-config-selected-row" : undefined}
      className={selectorOptions.selectorOptionClassName(selected, model.disabled)}
    >
      <div className="flex min-w-0 flex-1 items-center">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate">{model.name}</span>
            {(model.source === "cli" || model.source === "acp") && (
              <span className="shrink-0 text-xs text-muted-foreground">
                {t(model.source === "cli" ? "agents:modelSourceCli" : "agents:modelSourceAcp")}
              </span>
            )}
          </div>
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
