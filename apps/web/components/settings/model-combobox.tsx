"use client";

import { useState } from "react";
import { IconSelector } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import type { ModelEntry } from "@/lib/types/http";

type ModelComboboxProps = {
  value: string;
  onChange: (value: string) => void;
  models: ModelEntry[];
  currentModelId?: string;
  placeholder?: string;
  disabled?: boolean;
};

/**
 * Searchable model picker. The user can only choose from the agent's
 * advertised model list — custom model IDs are not allowed for ACP agents
 * since the agent CLI is authoritative for what it will accept.
 */
export function ModelCombobox({
  value,
  onChange,
  models,
  currentModelId,
  placeholder = "Select a model",
  disabled,
}: ModelComboboxProps) {
  const [open, setOpen] = useState(false);
  const effectiveValue = value || currentModelId || "";
  const selectedModel = models.find((m) => m.id === effectiveValue);

  const copilotUsage = (model: ModelEntry): string | undefined => {
    const raw = model.meta?.copilotUsage;
    return typeof raw === "string" ? raw : undefined;
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className="w-full justify-between font-normal cursor-pointer"
          data-testid="profile-model-combobox-trigger"
        >
          {selectedModel ? (
            <span className="flex items-center gap-2 truncate">
              {selectedModel.name}
              {selectedModel.id === currentModelId && (
                <span className="text-muted-foreground">(default)</span>
              )}
              {copilotUsage(selectedModel) && (
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium">
                  {copilotUsage(selectedModel)}
                </span>
              )}
            </span>
          ) : (
            <span className="text-muted-foreground">{placeholder}</span>
          )}
          <IconSelector className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[--radix-popover-trigger-width] p-0"
        align="start"
        onWheel={(e) => e.stopPropagation()}
      >
        <Command>
          <CommandInput placeholder="Search models..." />
          <CommandList
            className="max-h-[min(60vh,24rem)] overflow-y-auto overscroll-contain"
            onWheel={(e) => e.stopPropagation()}
          >
            <CommandEmpty>No model found.</CommandEmpty>
            <CommandGroup>
              {models.map((model) => {
                const usage = copilotUsage(model);
                return (
                  <CommandItem
                    key={model.id}
                    value={`${model.id} ${model.name}`}
                    onSelect={() => {
                      onChange(model.id);
                      setOpen(false);
                    }}
                    data-checked={effectiveValue === model.id}
                    className="cursor-pointer"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 truncate">
                        <span className="truncate">{model.name}</span>
                        {model.id === currentModelId && (
                          <span className="text-muted-foreground text-xs">(default)</span>
                        )}
                      </div>
                      {model.description && model.description !== model.name && (
                        <p className="text-xs text-muted-foreground truncate">
                          {model.description}
                        </p>
                      )}
                    </div>
                    {usage && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium ml-2">
                        {usage}
                      </span>
                    )}
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
