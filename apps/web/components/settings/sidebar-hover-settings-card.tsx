"use client";

import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { parseSidebarHoverDelay, type AppearanceState } from "./appearance-settings-state";
import { SettingsRow } from "./settings-group";
import { settingsControlClassName } from "./settings-control";

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
  const enabledDirty = draft.sidebarHoverEnabled !== saved.sidebarHoverEnabled;
  const delayDirty = draft.sidebarHoverDelayMs !== saved.sidebarHoverDelayMs;
  return (
    <div data-testid="sidebar-hover-settings-card" data-settings-dirty={enabledDirty || delayDirty}>
      <SettingsRow
        label={t("settings:sidebarHoverEnabled")}
        controlId="sidebar-hover-enabled"
        discoveryTargetId={GENERAL_SETTINGS_TARGETS.sidebarHover}
        isDirty={enabledDirty}
        touchTarget="switch"
        control={
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
        }
      />
      <SettingsRow
        label={t("settings:sidebarHoverDelay")}
        description={
          delay === null ? (
            <span id="sidebar-hover-delay-error" className="text-destructive" role="alert">
              {t("settings:sidebarHoverDelayError")}
            </span>
          ) : (
            <span id="sidebar-hover-help">{t("settings:sidebarHoverHelp")}</span>
          )
        }
        controlId="sidebar-hover-delay"
        discoveryTargetId={GENERAL_SETTINGS_TARGETS.sidebarHoverDelay}
        isDirty={delayDirty}
        control={
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
        }
      />
    </div>
  );
}
