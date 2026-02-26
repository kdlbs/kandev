"use client";

import { IconBell } from "@tabler/icons-react";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { ChangelogNotificationCard } from "@/components/settings/changelog-notification-card";
import { ChangelogList } from "@/components/settings/changelog-list";

export function ChangelogSettings() {
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Changelog</h2>
        <p className="text-sm text-muted-foreground mt-1">
          View all releases and manage notification preferences
        </p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconBell className="h-5 w-5" />}
        title="Notifications"
        description="Control the release notes notification in the topbar"
      >
        <ChangelogNotificationCard />
      </SettingsSection>

      <Separator />

      <SettingsSection title="Release History" description="All versions and their release notes">
        <ChangelogList />
      </SettingsSection>
    </div>
  );
}
