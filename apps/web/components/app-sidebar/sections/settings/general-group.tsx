"use client";

import { IconBell, IconCode, IconSettings } from "@tabler/icons-react";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const GENERAL_HREF = "/settings/general";
const NOTIFICATIONS_HREF = "/settings/general/notifications";
const EDITORS_HREF = "/settings/general/editors";

type GeneralGroupProps = {
  pathname: string;
};

export function GeneralGroup({ pathname }: GeneralGroupProps) {
  const isGeneral = pathname.startsWith(GENERAL_HREF);

  return (
    <SettingsGroup
      label="General"
      icon={IconSettings}
      href={GENERAL_HREF}
      isActive={pathname === GENERAL_HREF}
      defaultExpanded={isGeneral}
    >
      <SettingsLeaf
        href={NOTIFICATIONS_HREF}
        label="Notifications"
        icon={IconBell}
        isActive={pathname === NOTIFICATIONS_HREF}
        depth={1}
      />
      <SettingsLeaf
        href={EDITORS_HREF}
        label="Editors"
        icon={IconCode}
        isActive={pathname === EDITORS_HREF}
        depth={1}
      />
    </SettingsGroup>
  );
}
