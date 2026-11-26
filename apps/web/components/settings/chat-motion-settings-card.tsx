"use client";

import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";
import { SETTINGS_SWITCH_CLASS_NAME } from "./settings-control";

export function ChatMotionSettingsCard({
  enabled,
  isDirty,
  onChange,
}: {
  enabled: boolean;
  isDirty: boolean;
  onChange: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsRow
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.chatMotion}
      data-testid="chat-motion-settings-card"
      label={t("settings:chatAnimations")}
      description={t("settings:chatAnimationsDescription")}
      controlId="animate-chat"
      touchTarget="switch"
      controlWrapperTestId="chat-motion-toggle-row"
      control={
        <Switch
          id="animate-chat"
          checked={enabled}
          onCheckedChange={onChange}
          data-settings-dirty={isDirty}
          className={SETTINGS_SWITCH_CLASS_NAME}
        />
      }
    />
  );
}
