"use client";

import type { GitLabTaskPreset } from "./quick-task-launcher";
import { IntegrationStartTaskMenu } from "@/components/integrations/integration-start-task-menu";
import { useTranslation } from "react-i18next";

export function StartTaskMenu({
  presets,
  onSelect,
}: {
  presets: GitLabTaskPreset[];
  onSelect: (preset: GitLabTaskPreset) => void;
}) {
  const { t } = useTranslation();
  return (
    <IntegrationStartTaskMenu
      presets={presets}
      onSelect={onSelect}
      triggerLabel={t("gitlab:task")}
      triggerAriaLabel={t("gitlab:createTask")}
    />
  );
}
