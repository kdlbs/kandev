"use client";

import { useTranslation } from "react-i18next";
import { IconBell } from "@tabler/icons-react";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { ChangelogNotificationCard } from "@/components/settings/changelog-notification-card";
import { ChangelogList } from "@/components/settings/changelog-list";

export function ChangelogSettings() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{t("common:changelog")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("settings:changelogPageDescription")}
        </p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconBell className="h-5 w-5" />}
        title={t("settings:notifications")}
        description={t("settings:changelogNotificationsDescription")}
      >
        <ChangelogNotificationCard />
      </SettingsSection>

      <Separator />

      <SettingsSection
        title={t("settings:changelogReleaseHistory")}
        description={t("settings:changelogReleaseHistoryDescription")}
      >
        <ChangelogList />
      </SettingsSection>
    </div>
  );
}
