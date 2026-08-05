"use client";
import { IconSettings } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { GENERAL_SETTINGS_HOME, useGeneralNavItems } from "@/components/settings/general-nav";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

// The header leads to the group's first page, matching SystemGroup and
// AccountGroup. It used to point at bare `/settings/general`, which rendered a
// card grid over this same list.
const GENERAL_HREF = GENERAL_SETTINGS_HOME;

type GeneralGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function GeneralGroup({ pathname, expanded, onToggle }: GeneralGroupProps) {
  const navItems = useGeneralNavItems();
  const { t } = useTranslation();
  return (
    <SettingsGroup
      label={t("settings:general")}
      icon={IconSettings}
      href={GENERAL_HREF}
      // Never active: the header only links to the group's first page, so the
      // leaf for that page already carries the indicator. Marking both draws
      // two active rows for one location.
      isActive={false}
      expanded={expanded}
      onToggle={onToggle}
    >
      {navItems.map(({ href, label, icon }) => (
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
