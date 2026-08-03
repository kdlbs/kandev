"use client";

import { IconKey, IconShieldLock, IconUserCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/account";
const DEFAULT_HREF = `${ROOT_HREF}/security`;

// `labelKey`, not `label`: this table is module scope, so a `t()` here would
// freeze at the boot locale (and a SCREAMING_CASE literal is invisible to lint).
const ITEMS = [
  { href: `${ROOT_HREF}/security`, labelKey: "sidebar:profileAndPassword", icon: IconShieldLock },
  { href: `${ROOT_HREF}/tokens`, labelKey: "sidebar:apiTokens", icon: IconKey },
];

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
      href={DEFAULT_HREF}
      isActive={pathname.startsWith(ROOT_HREF)}
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
