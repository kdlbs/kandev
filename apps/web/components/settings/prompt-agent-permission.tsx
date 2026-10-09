"use client";

import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { SettingsRow } from "./settings-group";

export function PromptAgentPermission({
  allowed,
  saved,
  disabled,
  onChange,
}: {
  allowed: boolean;
  saved: boolean;
  disabled: boolean;
  onChange: (allowed: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsRow
      label={t("settings:promptAllowAgentEdits")}
      description={t("settings:promptAgentEditsDescription")}
      controlId="prompt-agent-edits"
      touchTarget="switch"
      isDirty={allowed !== saved}
      control={
        <Switch
          id="prompt-agent-edits"
          checked={allowed}
          disabled={disabled}
          onCheckedChange={onChange}
          data-settings-dirty={allowed !== saved}
          className="shrink-0 cursor-pointer"
        />
      }
    />
  );
}
