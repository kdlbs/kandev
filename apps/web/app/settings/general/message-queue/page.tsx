"use client";

import { useTranslation } from "react-i18next";
import { MessageQueueSettings } from "@/components/settings/system/message-queue-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function MessageQueueSettingsPage() {
  const { t } = useTranslation();
  return (
    <SystemPageShell
      title={t("system:messageQueueTitle")}
      description={t("system:messageQueueDescription")}
    >
      <MessageQueueSettings />
    </SystemPageShell>
  );
}
