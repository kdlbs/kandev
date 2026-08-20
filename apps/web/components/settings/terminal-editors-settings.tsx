"use client";

import { Separator } from "@kandev/ui/separator";
import { useTranslation } from "react-i18next";
import { EditorsSettings } from "@/components/settings/editors-settings";
import { TerminalSettings } from "@/components/settings/terminal-settings";
import { SettingsPageHeader } from "@/components/settings/settings-typography";

/** Terminal & Editors: the former Terminal and Editors pages as one page. */
export function TerminalEditorsSettings() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <SettingsPageHeader
        title={t("settings:terminalAndEditors")}
        description={t("settings:configureTheIncludedCodeEditorAnd")}
      />
      <Separator />
      <TerminalSettings />
      <Separator />
      <EditorsSettings embedded />
    </div>
  );
}
