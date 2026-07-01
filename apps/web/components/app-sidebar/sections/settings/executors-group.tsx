"use client";

import { IconCpu } from "@tabler/icons-react";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { getExecutorIcon } from "@/lib/executor-icons";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/executors";

type ExecutorsGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function ExecutorsGroup({ pathname, expanded, onToggle }: ExecutorsGroupProps) {
  const { executors } = useSettingsData(true);
  const allProfiles = executors.flatMap((executor) =>
    (executor.profiles ?? []).map((profile) => ({ ...profile, executorType: executor.type })),
  );

  return (
    <SettingsGroup
      label="Executors"
      icon={IconCpu}
      href={ROOT_HREF}
      isActive={pathname === ROOT_HREF}
      expanded={expanded}
      onToggle={onToggle}
    >
      {allProfiles.map((profile) => {
        const Icon = getExecutorIcon(profile.executorType);
        const profilePath = `${ROOT_HREF}/${profile.id}`;
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
