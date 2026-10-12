"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useHideDisabledAgentProfilesInNav } from "@/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav";

export const HIDE_DISABLED_AGENT_PROFILES_SWITCH_ID = "hide-disabled-agent-profiles-in-nav";

/** The Agent options switch saves immediately and keeps the surface open. */
export function HideDisabledAgentProfilesSetting({
  isTouchTarget = false,
}: {
  isTouchTarget?: boolean;
}) {
  const { t } = useTranslation();
  const { hideDisabled, setHideDisabled } = useHideDisabledAgentProfilesInNav();
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="min-w-0 space-y-0.5">
        <Label htmlFor={HIDE_DISABLED_AGENT_PROFILES_SWITCH_ID}>
          {t("settings:hideDisabledAgentProfilesFromNav")}
        </Label>
        <p id="hide-disabled-agent-profiles-description" className="text-xs text-muted-foreground">
          {t("settings:hideDisabledAgentProfilesFromNavDescription")}
        </p>
      </div>
      <Label
        htmlFor={HIDE_DISABLED_AGENT_PROFILES_SWITCH_ID}
        data-testid={isTouchTarget ? "agent-options-switch-target" : undefined}
        className={
          isTouchTarget
            ? "flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center rounded-md"
            : "flex shrink-0 cursor-pointer items-center justify-center"
        }
      >
        <Switch
          id={HIDE_DISABLED_AGENT_PROFILES_SWITCH_ID}
          aria-describedby="hide-disabled-agent-profiles-description"
          checked={hideDisabled}
          onCheckedChange={setHideDisabled}
          className="shrink-0 cursor-pointer"
        />
      </Label>
    </div>
  );
}
