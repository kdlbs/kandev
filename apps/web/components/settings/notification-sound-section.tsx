"use client";

import { useState } from "react";
import { IconVolume } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  getSoundPreferences,
  isSoundPresetId,
  playSoundPreset,
  setSoundPreferences,
  SOUND_PRESETS,
  type SoundPreferences,
} from "@/lib/notifications/sound";

export function NotificationSoundSection() {
  const [prefs, setPrefs] = useState<SoundPreferences>(getSoundPreferences);

  const update = (next: SoundPreferences) => {
    setPrefs(next);
    setSoundPreferences(next);
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="text-base font-medium">Notification Sound</div>
          <p className="text-sm text-muted-foreground">
            Play a sound on this device when an agent needs your input.
          </p>
        </div>
        <Switch
          checked={prefs.enabled}
          onCheckedChange={(enabled) => update({ ...prefs, enabled })}
          aria-label="Enable notification sound"
          className="cursor-pointer"
        />
      </div>
      {prefs.enabled && (
        <div className="flex items-center gap-2">
          <Select
            value={prefs.presetId}
            onValueChange={(presetId) => {
              if (!isSoundPresetId(presetId)) return;
              update({ ...prefs, presetId });
              playSoundPreset(presetId);
            }}
          >
            <SelectTrigger className="w-44 cursor-pointer" aria-label="Notification sound">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SOUND_PRESETS.map((preset) => (
                <SelectItem key={preset.id} value={preset.id} className="cursor-pointer">
                  {preset.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon"
                  className="cursor-pointer"
                  aria-label="Preview sound"
                  onClick={() => playSoundPreset(prefs.presetId)}
                >
                  <IconVolume className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Preview sound</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      )}
    </div>
  );
}
