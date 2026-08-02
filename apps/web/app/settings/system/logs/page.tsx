"use client";

import { useTranslation } from "react-i18next";
import { LogViewer } from "@/components/settings/system/log-viewer";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function SystemLogsPage() {
  const { t } = useTranslation();
  return (
    <SystemPageShell
      title={t("settings:logsPageTitle")}
      description={t("settings:logsPageDescription")}
    >
      <LogViewer />
    </SystemPageShell>
  );
}
