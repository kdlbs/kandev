"use client";

import { IconSettings } from "@tabler/icons-react";
import { GENERAL_NAV_ITEMS } from "@/components/settings/general-nav";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const GENERAL_HREF = "/settings/general";

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
      {GENERAL_NAV_ITEMS.map(({ href, label, icon }) => (
        <SettingsLeaf
          key={href}
          href={href}
          label={label}
          icon={icon}
          isActive={pathname === href}
          depth={1}
        />
      ))}
    </SettingsGroup>
  );
}
