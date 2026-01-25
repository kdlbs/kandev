'use client';

import { useState } from 'react';
import { IconRefresh, IconAlertCircle, IconSelector } from '@tabler/icons-react';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Checkbox } from '@kandev/ui/checkbox';
import { Button } from '@kandev/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Popover, PopoverContent, PopoverTrigger } from '@kandev/ui/popover';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@kandev/ui/command';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { Skeleton } from '@kandev/ui/skeleton';
import { useDynamicModels } from '@/hooks/domains/settings/use-dynamic-models';
import type { ModelEntry } from '@/lib/types/http';

type ModelComboboxProps = {
  value: string;
  onChange: (value: string) => void;
  models: ModelEntry[];
  defaultModel?: string;
  placeholder?: string;
  agentName?: string;
  supportsDynamicModels?: boolean;
};

export function ModelCombobox({
  value,
  onChange,
  models: staticModels,
  defaultModel,
  placeholder = 'Select a model',
  agentName,
  supportsDynamicModels = false,
}: ModelComboboxProps) {
  const {
    models,
    isLoading,
    error,
    refresh,
  } = useDynamicModels(agentName, staticModels, supportsDynamicModels);

  const [open, setOpen] = useState(false);

  // Initialize customValue with the current value if it's not in the static models list
  // (we use staticModels for init since dynamic models haven't loaded yet)
  const [customValue, setCustomValue] = useState(() => {
    if (value && !staticModels.some((m) => m.id === value)) {
      return value;
    }
    return '';
  });

  // Determine if the current value is a custom model (not in the list)
  const isValueCustom = Boolean(value) && !models.some((m) => m.id === value);
  // Track user's explicit toggle choice; null means derive from value
  const [userToggle, setUserToggle] = useState<boolean | null>(null);
  // Derive custom mode: user toggle takes precedence, otherwise infer from value
  const isCustomMode = userToggle ?? isValueCustom;

  const handleModeChange = (custom: boolean) => {
    setUserToggle(custom);
    if (custom) {
      // Switching to custom mode - use preserved custom value
      if (customValue) {
        onChange(customValue);
      }
    } else {
      // Switching to select mode - preserve current value if it's custom
      if (isValueCustom && value) {
        setCustomValue(value);
      }
      onChange(defaultModel || '');
    }
  };

  const handleCustomValueChange = (newValue: string) => {
    setCustomValue(newValue);
    // Ensure we stay in custom mode when user is typing in the custom input
    if (userToggle !== true) {
      setUserToggle(true);
    }
    onChange(newValue);
  };

  const handleRefresh = async () => {
    await refresh();
  };

  const selectedModel = models.find((m) => m.id === (value || defaultModel));

  // Render the dropdown/combobox for model selection
  const renderModelSelector = () => {
    // Use searchable combobox for dynamic models (typically many options)
    if (supportsDynamicModels) {
      return (
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              variant="outline"
              role="combobox"
              aria-expanded={open}
              className="w-full justify-between font-normal"
              disabled={isCustomMode}
            >
              {selectedModel ? (
                <span className="flex items-center gap-2 truncate">
                  {selectedModel.name}
                  {selectedModel.id === defaultModel && (
                    <span className="text-muted-foreground">(default)</span>
                  )}
                  {selectedModel.provider && (
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium">
                      {selectedModel.provider}
                    </span>
                  )}
                </span>
              ) : (
                <span className="text-muted-foreground">{placeholder}</span>
              )}
              <IconSelector className="ml-2 h-4 w-4 shrink-0 opacity-50" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
            <Command>
              <CommandInput placeholder="Search models..." />
              <CommandList>
                <CommandEmpty>No model found.</CommandEmpty>
                <CommandGroup>
                  {models.map((model) => (
                    <CommandItem
                      key={model.id}
                      value={model.id}
                      onSelect={(currentValue) => {
                        onChange(currentValue);
                        setOpen(false);
                      }}
                      data-checked={value === model.id || (!value && model.id === defaultModel)}
                    >
                      <span className="flex-1 truncate">
                        {model.name}
                        {model.id === defaultModel && (
                          <span className="text-muted-foreground text-xs ml-1">(default)</span>
                        )}
                      </span>
                      {model.provider && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium mr-1">
                          {model.provider}
                        </span>
                      )}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
      );
    }

    // Use simple select for static models (fewer options)
    return (
      <Select
        value={value || defaultModel}
        onValueChange={onChange}
        disabled={isCustomMode}
      >
        <SelectTrigger className="w-full">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {models.map((model) => (
            <SelectItem key={model.id} value={model.id}>
              <span className="flex items-center gap-2">
                {model.name}
                {model.id === defaultModel && (
                  <span className="text-muted-foreground">(default)</span>
                )}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  };

  // Show full-row skeleton while loading dynamic models
  if (supportsDynamicModels && isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-4 w-32 ml-auto" />
      </div>
    );
  }

  return (
    <div className="flex items-start gap-2">
      {/* Left side: Model dropdown/combobox + refresh button */}
      <div className="flex flex-1 items-center gap-2">
        <div className="flex-1">
          {renderModelSelector()}
        </div>

        {/* Refresh button for dynamic models */}
        {supportsDynamicModels && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                onClick={handleRefresh}
                disabled={isCustomMode}
              >
                <IconRefresh className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              <p>Refresh models from CLI</p>
            </TooltipContent>
          </Tooltip>
        )}

        {/* Error indicator */}
        {error && (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="flex items-center">
                <IconAlertCircle className="h-4 w-4 text-amber-500" />
              </div>
            </TooltipTrigger>
            <TooltipContent>
              <p className="max-w-xs">
                Failed to fetch dynamic models: {error}
                <br />
                Showing static fallback models.
              </p>
            </TooltipContent>
          </Tooltip>
        )}
      </div>

      {/* Right side: Custom model input + checkbox */}
      <div className="flex-1 space-y-2">
        <Input
          value={customValue}
          onChange={(e) => handleCustomValueChange(e.target.value)}
          placeholder="Custom model ID"
          disabled={!isCustomMode}
        />
        <div className="flex items-center space-x-2">
          <Checkbox
            id="custom-model-mode"
            checked={isCustomMode}
            onCheckedChange={(checked) => handleModeChange(checked === true)}
          />
          <Label htmlFor="custom-model-mode" className="text-xs text-muted-foreground">
            Use custom model
          </Label>
        </div>
      </div>
    </div>
  );
}
