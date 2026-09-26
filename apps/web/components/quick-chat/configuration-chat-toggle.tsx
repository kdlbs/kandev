"use client";

import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";

export function ConfigurationChatToggle({
  checked,
  disabled,
  onCheckedChange,
}: {
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <section
      className="flex min-h-11 items-center justify-between gap-4"
      aria-labelledby="config-chat-mode-label"
    >
      <div className="min-w-0">
        <h3 id="config-chat-mode-label" className="text-sm font-medium">
          {t("chat:configurationChat")}
        </h3>
        <p className="text-xs text-muted-foreground">{t("chat:configurationChatDescription")}</p>
      </div>
      <Switch
        aria-label={t("chat:configurationChat")}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
        className="shrink-0 cursor-pointer"
      />
    </section>
  );
}
