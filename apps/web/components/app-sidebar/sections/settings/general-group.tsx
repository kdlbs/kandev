"use client";

import { IconBell, IconCode, IconSettings } from "@tabler/icons-react";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const GENERAL_HREF = "/settings/general";
const NOTIFICATIONS_HREF = "/settings/general/notifications";
const EDITORS_HREF = "/settings/general/editors";

type GeneralGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function GeneralGroup({ pathname, expanded, onToggle }: GeneralGroupProps) {
  return (
    <SettingsGroup
      label="General"
      icon={IconSettings}
      href={GENERAL_HREF}
      isActive={pathname === GENERAL_HREF}
      expanded={expanded}
      onToggle={onToggle}
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
