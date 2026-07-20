"use client";

import { useEffect, useState } from "react";
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
import { useSettingsSaveContributor } from "./settings-save-provider";

export function NotificationSoundSection({
  onDirtyChange,
}: {
  onDirtyChange?: (isDirty: boolean) => void;
}) {
  const [saved, setSaved] = useState<SoundPreferences>(getSoundPreferences);
  const [prefs, setPrefs] = useState<SoundPreferences>(saved);
  const revision = JSON.stringify(prefs);
  const isDirty = revision !== JSON.stringify(saved);

  useEffect(() => onDirtyChange?.(isDirty), [isDirty, onDirtyChange]);

  useSettingsSaveContributor({
    id: "general-notification-sound",
    revision,
    isDirty,
    save: () => {
      const submitted = prefs;
      setSoundPreferences(submitted);
      setSaved(submitted);
    },
    discard: () => setPrefs(saved),
  });

  return (
    <div
      className="space-y-4 rounded-md border p-4"
      data-settings-dirty={isDirty}
      data-testid="notification-sound-group"
    >
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="text-base font-medium">Notification Sound</div>
          <p className="text-sm text-muted-foreground">
            Play a sound on this device when an agent needs your input.
          </p>
        </div>
        <Switch
          checked={prefs.enabled}
          data-settings-dirty={prefs.enabled !== saved.enabled}
          onCheckedChange={(enabled) => setPrefs({ ...prefs, enabled })}
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
              setPrefs({ ...prefs, presetId });
              playSoundPreset(presetId);
            }}
          >
            <SelectTrigger
              className="w-44 cursor-pointer"
              aria-label="Notification sound"
              data-settings-dirty={prefs.presetId !== saved.presetId}
            >
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
