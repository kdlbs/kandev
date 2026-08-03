"use client";

import {
  IconActivity,
  IconArchive,
  IconDatabase,
  IconFileText,
  IconFlask,
  IconInfoCircle,
  IconRefresh,
  IconScale,
  IconServerCog,
  IconTrash,
  IconUsers,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/system";
const DEFAULT_HREF = `${ROOT_HREF}/status`;

/**
 * `href` is the route and stays a value; only `labelKey` is copy, resolved at
 * render so the nav follows a locale switch. Storing resolved labels here
 * would freeze them at the boot locale — and because these are SCREAMING_CASE
 * identifiers, `mode: "jsx-only"` skips them entirely, so lint reported none
 * of the ten.
 */
const BASE_ITEMS: Array<{ href: string; labelKey: string; icon: TablerIcon }> = [
  { href: `${ROOT_HREF}/status`, labelKey: "common:status", icon: IconActivity },
  { href: `${ROOT_HREF}/feature-toggles`, labelKey: "system:navFeatureToggles", icon: IconFlask },
  { href: `${ROOT_HREF}/database`, labelKey: "system:navDatabase", icon: IconDatabase },
  { href: `${ROOT_HREF}/backups`, labelKey: "system:navBackups", icon: IconArchive },
  { href: `${ROOT_HREF}/storage`, labelKey: "system:navStorage", icon: IconTrash },
  { href: `${ROOT_HREF}/logs`, labelKey: "system:navLogs", icon: IconFileText },
  { href: `${ROOT_HREF}/updates`, labelKey: "system:navUpdates", icon: IconRefresh },
  { href: `${ROOT_HREF}/about`, labelKey: "system:navAbout", icon: IconInfoCircle },
  { href: `${ROOT_HREF}/licenses`, labelKey: "system:navLicenses", icon: IconScale },
];

const AUTH_ITEMS: Array<{ href: string; labelKey: string; icon: TablerIcon }> = [
  { href: `${ROOT_HREF}/users`, labelKey: "system:navUsers", icon: IconUsers },
];

type SystemGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

/** null user (disabled/synthetic single-user mode) counts as admin for gating. */
function useIsAdmin(): boolean {
  const role = useAppStore((s) => s.auth.user?.role);
  return role === undefined || role === "admin";
}

export function SystemGroup({ pathname, expanded, onToggle }: SystemGroupProps) {
  const { t } = useTranslation();
  const authEnabled = useFeature("auth");
  const isAdmin = useIsAdmin();
  const items = authEnabled && isAdmin ? [...BASE_ITEMS, ...AUTH_ITEMS] : BASE_ITEMS;

  return (
    <SettingsGroup
      label={t("common:system")}
      icon={IconServerCog}
      href={DEFAULT_HREF}
      isActive={pathname.startsWith(ROOT_HREF)}
      expanded={expanded}
      onToggle={onToggle}
    >
      {items.map(({ href, labelKey, icon }) => (
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
