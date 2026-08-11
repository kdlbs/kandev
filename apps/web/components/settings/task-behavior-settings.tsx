"use client";

import { IconMessage } from "@tabler/icons-react";
import { Separator } from "@kandev/ui/separator";
import { useTranslation } from "react-i18next";
import { TaskActionsSettings } from "@/components/settings/general-settings";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsTarget } from "@/components/settings/settings-target";
import { MessageQueueSettings } from "@/components/settings/system/message-queue-settings";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";

/** Task behavior: the former Task Actions and Message Queue pages as one page. */
export function TaskBehaviorSettings() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <TaskActionsSettings />
      <Separator />
      <SettingsTarget targetId={GENERAL_SETTINGS_TARGETS.messageQueue}>
        <SettingsSection
          icon={<IconMessage className="h-5 w-5" />}
          title={t("system:messageQueueTitle")}
          description={t("system:messageQueueDescription")}
        >
          <MessageQueueSettings />
        </SettingsSection>
      </SettingsTarget>
    </div>
  );
}
