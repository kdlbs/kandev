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
import {
  SYSTEM_DISCOVERY_DEFINITIONS,
  SYSTEM_SETTINGS_HREF,
  SYSTEM_STATUS_SETTINGS_HREF,
} from "@/lib/settings-discovery/catalog/system";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const SYSTEM_ITEM_ICONS: Record<string, TablerIcon> = {
  "system-status": IconActivity,
  "system-feature-toggles": IconFlask,
  "system-database": IconDatabase,
  "system-backups": IconArchive,
  "system-storage": IconTrash,
  "system-logs": IconFileText,
  "system-updates": IconRefresh,
  "system-about": IconInfoCircle,
  "system-licenses": IconScale,
  "system-users": IconUsers,
};

const SYSTEM_ITEMS = SYSTEM_DISCOVERY_DEFINITIONS.filter(
  (item) => item.parentId === "system" && item.labelKey,
).map((item) => ({
  ...item,
  labelKey: item.labelKey as string,
  icon: SYSTEM_ITEM_ICONS[item.id],
}));

const BASE_ITEMS = SYSTEM_ITEMS.filter((item) => item.requires !== "users");
const AUTH_ITEMS = SYSTEM_ITEMS.filter((item) => item.requires === "users");

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
      href={SYSTEM_STATUS_SETTINGS_HREF}
      // Never active: every /settings/system route has a leaf of its own, so the
      // header would double the indicator (Status was the visible case).
      isActive={false}
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
