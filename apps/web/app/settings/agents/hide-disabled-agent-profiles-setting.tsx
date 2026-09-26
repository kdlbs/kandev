"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useHideDisabledAgentProfilesInNav } from "@/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav";

/**
 * Row for the "Hide disabled agent profiles from left panel navigation"
 * setting on `/settings/agents`. Saves immediately on toggle — the agents
 * page has no settings-floating-save bar, and its profile enabled toggles
 * are immediate-save too (`useProfileEnabledToggle`).
 */
export function HideDisabledAgentProfilesSetting() {
  const { t } = useTranslation();
  const { hideDisabled, setHideDisabled } = useHideDisabledAgentProfilesInNav();
  return (
    <div className="flex min-h-11 items-center justify-between gap-4 rounded-lg border p-4">
      <div className="min-w-0 space-y-0.5">
        <Label htmlFor="hide-disabled-agent-profiles-in-nav">
          {t("settings:hideDisabledAgentProfilesFromNav")}
        </Label>
        <p className="text-xs text-muted-foreground">
          {t("settings:hideDisabledAgentProfilesFromNavDescription")}
        </p>
      </div>
      <Switch
        id="hide-disabled-agent-profiles-in-nav"
        checked={hideDisabled}
        onCheckedChange={setHideDisabled}
        className="shrink-0 cursor-pointer"
      />
    </div>
  );
}
