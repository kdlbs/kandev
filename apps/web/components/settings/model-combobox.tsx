'use client';

import { useState } from 'react';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Switch } from '@kandev/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import type { ModelEntry } from '@/lib/types/http';

type ModelComboboxProps = {
  value: string;
  onChange: (value: string) => void;
  models: ModelEntry[];
  defaultModel?: string;
  placeholder?: string;
};

export function ModelCombobox({
  value,
  onChange,
  models,
  defaultModel,
  placeholder = 'Select a model',
}: ModelComboboxProps) {
  // Determine if the current value is a custom model (not in the list)
  const isValueCustom = Boolean(value) && !models.some((m) => m.id === value);
  // Track user's explicit toggle choice; null means derive from value
  const [userToggle, setUserToggle] = useState<boolean | null>(null);
  // Derive custom mode: user toggle takes precedence, otherwise infer from value
  const isCustomMode = userToggle ?? isValueCustom;

  const handleModeChange = (custom: boolean) => {
    setUserToggle(custom);
    if (!custom && isValueCustom) {
      // Switching to select mode with a custom value - reset to default
      onChange(defaultModel || '');
    }
  };

  return (
    <div className="space-y-2">
      {isCustomMode ? (
        <Input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="Enter model ID (e.g., gpt-4o)"
        />
      ) : (
        <Select
          value={value || defaultModel}
          onValueChange={onChange}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder={placeholder} />
          </SelectTrigger>
          <SelectContent>
            {models.map((model) => (
              <SelectItem key={model.id} value={model.id}>
                {model.name}
                {model.id === defaultModel && (
                  <span className="ml-2 text-muted-foreground">(default)</span>
                )}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      <div className="flex items-center space-x-2">
        <Switch
          id="custom-model-mode"
          checked={isCustomMode}
          onCheckedChange={handleModeChange}
        />
        <Label htmlFor="custom-model-mode" className="text-xs text-muted-foreground">
          Custom model
        </Label>
      </div>
    </div>
  );
}
