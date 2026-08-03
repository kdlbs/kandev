"use client";

import { useTranslation } from "react-i18next";
import { StorageMaintenanceSettings } from "@/components/settings/system/storage/storage-maintenance-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function StoragePage() {
  const { t } = useTranslation();
  return (
    <SystemPageShell title={t("system:storageTitle")} description={t("system:storageDescription")}>
      <StorageMaintenanceSettings />
    </SystemPageShell>
  );
}
