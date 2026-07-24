"use client";

import { IconKey, IconShieldLock, IconUserCircle } from "@tabler/icons-react";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/account";
const DEFAULT_HREF = `${ROOT_HREF}/security`;

const ITEMS = [
  { href: `${ROOT_HREF}/security`, label: "Profile & Password", icon: IconShieldLock },
  { href: `${ROOT_HREF}/tokens`, label: "API Tokens", icon: IconKey },
];

type AccountGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function AccountGroup({ pathname, expanded, onToggle }: AccountGroupProps) {
  return (
    <SettingsGroup
      label="Account"
      icon={IconUserCircle}
      href={DEFAULT_HREF}
      isActive={pathname.startsWith(ROOT_HREF)}
      expanded={expanded}
      onToggle={onToggle}
    >
      {ITEMS.map(({ href, label, icon }) => (
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
