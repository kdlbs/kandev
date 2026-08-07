"use client";

import { IconKey, IconShieldLock, IconUserCircle } from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  ACCOUNT_DISCOVERY_DEFINITIONS,
  ACCOUNT_SECURITY_SETTINGS_HREF,
  ACCOUNT_SETTINGS_HREF,
} from "@/lib/settings-discovery/catalog/account";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ACCOUNT_ITEM_ICONS: Record<string, TablerIcon> = {
  "account-security": IconShieldLock,
  "account-tokens": IconKey,
};

const ITEMS = ACCOUNT_DISCOVERY_DEFINITIONS.filter(
  (item) => item.parentId === "account" && item.labelKey,
).map((item) => ({
  ...item,
  labelKey: item.labelKey as string,
  icon: ACCOUNT_ITEM_ICONS[item.id],
}));

type AccountGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function AccountGroup({ pathname, expanded, onToggle }: AccountGroupProps) {
  const { t } = useTranslation();
  return (
    <SettingsGroup
      label={t("sidebar:account")}
      icon={IconUserCircle}
      href={ACCOUNT_SECURITY_SETTINGS_HREF}
      isActive={pathname.startsWith(ACCOUNT_SETTINGS_HREF)}
      expanded={expanded}
      onToggle={onToggle}
    >
      {ITEMS.map(({ href, labelKey, icon }) => (
        <SettingsLeaf
          key={href}
          href={href}
          label={t(labelKey)}
          icon={icon}
          isActive={pathname === href}
          depth={1}
        />
      ))}
    </SettingsGroup>
  );
}
