"use client";

import { IconCpu } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { getExecutorIcon } from "@/lib/executor-icons";
import { EXECUTORS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/executors";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

type ExecutorsGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function ExecutorsGroup({ pathname, expanded, onToggle }: ExecutorsGroupProps) {
  const { t } = useTranslation();
  const executors = useAppStore((s) => s.executors.items);
  const allProfiles = executors.flatMap((executor) =>
    (executor.profiles ?? []).map((profile) => ({ ...profile, executorType: executor.type })),
  );

  return (
    <SettingsGroup
      label={t("common:executors")}
      icon={IconCpu}
      href={EXECUTORS_SETTINGS_HREF}
      isActive={pathname === EXECUTORS_SETTINGS_HREF}
      expanded={expanded}
      onToggle={onToggle}
    >
      {allProfiles.map((profile) => {
        const Icon = getExecutorIcon(profile.executorType);
        const profilePath = `${EXECUTORS_SETTINGS_HREF}/${profile.id}`;
        return (
          <SettingsLeaf
            key={profile.id}
            href={profilePath}
            label={profile.name}
            icon={Icon}
            isActive={pathname === profilePath}
            depth={1}
          />
        );
      })}
    </SettingsGroup>
  );
}
