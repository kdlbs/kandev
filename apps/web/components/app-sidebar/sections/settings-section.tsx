"use client";

import { usePathname } from "next/navigation";
import { IconSettings } from "@tabler/icons-react";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";
import { SettingsTree } from "./settings/settings-tree";

type SettingsSectionProps = {
  collapsed: boolean;
};

export function SettingsSection({ collapsed }: SettingsSectionProps) {
  const pathname = usePathname();

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.settings}
      label="Settings"
      collapsed={collapsed}
      icon={IconSettings}
    >
      <SettingsTree pathname={pathname} />
    </AppSidebarSection>
  );
}
