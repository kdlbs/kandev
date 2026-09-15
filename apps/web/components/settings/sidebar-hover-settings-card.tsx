"use client";

import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { parseSidebarHoverDelay, type AppearanceState } from "./appearance-settings-state";
import { SettingsCard } from "./settings-card";
import { settingsControlClassName } from "./settings-control";
import { useSettingsTargetRegistration } from "./settings-target-provider";

export function SidebarHoverSettingsCard({
  draft,
  saved,
  updateDraft,
}: {
  draft: AppearanceState;
  saved: AppearanceState;
  updateDraft: (patch: Partial<AppearanceState>) => void;
}) {
  const { t } = useTranslation();
  const lastValidDelay = useRef(saved.sidebarHoverDelayMs);
  const delay = parseSidebarHoverDelay(draft.sidebarHoverDelayMs);
  useEffect(() => {
    if (delay !== null) lastValidDelay.current = String(delay);
  }, [delay]);
  const toggleTarget = useSettingsTargetRegistration(GENERAL_SETTINGS_TARGETS.sidebarHover);
  const delayTarget = useSettingsTargetRegistration(GENERAL_SETTINGS_TARGETS.sidebarHoverDelay);
  const enabledDirty = draft.sidebarHoverEnabled !== saved.sidebarHoverEnabled;
  const delayDirty = draft.sidebarHoverDelayMs !== saved.sidebarHoverDelayMs;
  return (
    <SettingsCard isDirty={enabledDirty || delayDirty} data-testid="sidebar-hover-settings-card">
      <CardContent className="space-y-4">
        <div ref={toggleTarget} className="flex items-center justify-between gap-4">
          <Label htmlFor="sidebar-hover-enabled">{t("settings:sidebarHoverEnabled")}</Label>
          <Switch
            id="sidebar-hover-enabled"
            checked={draft.sidebarHoverEnabled}
            data-settings-dirty={enabledDirty}
            onCheckedChange={(sidebarHoverEnabled) =>
              updateDraft({
                sidebarHoverEnabled,
                ...(!sidebarHoverEnabled && delay === null
                  ? { sidebarHoverDelayMs: lastValidDelay.current }
                  : {}),
              })
            }
            className="data-checked:bg-transparent data-unchecked:bg-transparent dark:data-unchecked:bg-transparent data-[size=default]:h-7 max-md:data-[size=default]:h-11 [@media(pointer:coarse)]:data-[size=default]:h-11 data-[size=default]:w-11 p-2 before:absolute before:left-2 before:top-1/2 before:h-[16.6px] before:w-7 before:-translate-y-1/2 before:rounded-full before:bg-input before:content-[''] data-checked:before:bg-primary dark:data-unchecked:before:bg-input/80 [&_[data-slot=switch-thumb]]:z-10"
          />
        </div>
        <div ref={delayTarget} className="space-y-2">
          <Label htmlFor="sidebar-hover-delay">{t("settings:sidebarHoverDelay")}</Label>
          <Input
            id="sidebar-hover-delay"
            type="number"
            inputMode="numeric"
            min={0}
            max={5000}
            step={1}
            value={draft.sidebarHoverDelayMs}
            disabled={!draft.sidebarHoverEnabled}
            aria-invalid={delay === null}
            aria-describedby={delay === null ? "sidebar-hover-delay-error" : "sidebar-hover-help"}
            data-settings-dirty={delayDirty}
            className={settingsControlClassName("md:max-w-40")}
            onChange={(event) => {
              const value = event.target.value;
              if (parseSidebarHoverDelay(value) !== null) lastValidDelay.current = value;
              updateDraft({ sidebarHoverDelayMs: value });
            }}
          />
          {delay === null && (
            <p id="sidebar-hover-delay-error" className="text-sm text-destructive" role="alert">
              {t("settings:sidebarHoverDelayError")}
            </p>
          )}
        </div>
        <p id="sidebar-hover-help" className="text-xs text-muted-foreground">
          {t("settings:sidebarHoverHelp")}
        </p>
      </CardContent>
    </SettingsCard>
  );
}
