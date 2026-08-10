"use client";

import { useEffect, useState } from "react";
import { IconVolume } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
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
  type SoundPresetId,
} from "@/lib/notifications/sound";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { SettingsTarget } from "./settings-target";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/general";

/**
 * Catalog keys for the preset names, keyed by the preset id — the id is the
 * persisted sentinel and must never be translated. Keys, not `t()` calls: a
 * module-scope `t` would freeze the names at the boot locale (docs/i18n.md).
 */
const SOUND_PRESET_LABEL_KEYS: Record<SoundPresetId, string> = {
  plim: "settings:soundPresetPlim",
  chime: "settings:soundPresetChime",
  ding: "settings:soundPresetDing",
  pop: "settings:soundPresetPop",
};

export function NotificationSoundSection({
  onDirtyChange,
}: {
  onDirtyChange?: (isDirty: boolean) => void;
}) {
  const { t } = useTranslation();
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
    <SettingsTarget
      targetId={GENERAL_SETTINGS_TARGETS.notificationSound}
      className="space-y-4 rounded-md border p-4"
      data-settings-dirty={isDirty}
      data-testid="notification-sound-group"
    >
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="text-sm font-medium">{t("settings:notificationSound")}</div>
          <p className="text-xs text-muted-foreground">
            {t("settings:notificationSoundDescription")}
          </p>
        </div>
        <Switch
          checked={prefs.enabled}
          data-settings-dirty={prefs.enabled !== saved.enabled}
          onCheckedChange={(enabled) => setPrefs({ ...prefs, enabled })}
          aria-label={t("settings:enableNotificationSound")}
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
              aria-label={t("settings:notificationSoundSelect")}
              data-settings-dirty={prefs.presetId !== saved.presetId}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SOUND_PRESETS.map((preset) => (
                <SelectItem key={preset.id} value={preset.id} className="cursor-pointer">
                  {t(SOUND_PRESET_LABEL_KEYS[preset.id])}
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
                  aria-label={t("settings:previewSound")}
                  onClick={() => playSoundPreset(prefs.presetId)}
                >
                  <IconVolume className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("settings:previewSound")}</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      )}
    </SettingsTarget>
  );
}
