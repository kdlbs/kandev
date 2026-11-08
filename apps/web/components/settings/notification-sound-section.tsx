"use client";

import { useEffect, useId, useState } from "react";
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
import { NotificationPreferenceRow } from "./notification-preference-row";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";

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

function SoundPreviewButton({
  presetId,
  disabled,
}: {
  presetId: SoundPresetId;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            tabIndex={disabled ? 0 : -1}
            className="inline-flex"
            aria-label={disabled ? t("settings:previewSoundUnavailable") : undefined}
          >
            <Button
              variant="outline"
              size="icon"
              className="cursor-pointer"
              aria-label={t("settings:previewSound")}
              disabled={disabled}
              onClick={() => playSoundPreset(presetId)}
            >
              <IconVolume className="h-4 w-4" />
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>
          {disabled ? t("settings:previewSoundUnavailable") : t("settings:previewSound")}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export function NotificationSoundSection({
  onDirtyChange,
}: {
  onDirtyChange?: (isDirty: boolean) => void;
}) {
  const { t } = useTranslation();
  const soundToggleId = useId();
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

  const controlsDisabled = !prefs.enabled;
  return (
    <NotificationPreferenceRow
      label={t("settings:notificationSound")}
      labelFor={soundToggleId}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.notificationSound}
      isDirty={isDirty}
      data-testid="notification-sound-group"
      leadingControl={
        <label
          htmlFor={soundToggleId}
          data-testid="notification-sound-toggle-target"
          className="flex min-h-7 min-w-7 cursor-pointer items-center justify-center max-md:min-h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
        >
          <Switch
            id={soundToggleId}
            checked={prefs.enabled}
            data-settings-dirty={prefs.enabled !== saved.enabled}
            onCheckedChange={(enabled) => setPrefs({ ...prefs, enabled })}
            aria-label={t("settings:enableNotificationSound")}
            className="cursor-pointer"
          />
        </label>
      }
    >
      <Select
        value={prefs.presetId}
        disabled={controlsDisabled}
        onValueChange={(presetId) => {
          if (!isSoundPresetId(presetId)) return;
          setPrefs({ ...prefs, presetId });
          playSoundPreset(presetId);
        }}
      >
        <SelectTrigger
          className="min-w-0 flex-1 cursor-pointer md:w-44 md:flex-none"
          aria-label={t("settings:notificationSoundSelect")}
          data-settings-dirty={prefs.presetId !== saved.presetId}
          disabled={controlsDisabled}
        >
          <SelectValue>{t(SOUND_PRESET_LABEL_KEYS[prefs.presetId])}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          {SOUND_PRESETS.map((preset) => (
            <SelectItem key={preset.id} value={preset.id} className="cursor-pointer">
              {t(SOUND_PRESET_LABEL_KEYS[preset.id])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <SoundPreviewButton presetId={prefs.presetId} disabled={controlsDisabled} />
    </NotificationPreferenceRow>
  );
}
