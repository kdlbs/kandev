"use client";

import { IconCpu } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { getExecutorIcon } from "@/lib/executor-icons";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/executors";

type ExecutorsGroupProps = {
  pathname: string;
};

export function ExecutorsGroup({ pathname }: ExecutorsGroupProps) {
  const executors = useAppStore((s) => s.executors.items);
  const allProfiles = executors.flatMap((executor) =>
    (executor.profiles ?? []).map((profile) => ({ ...profile, executorType: executor.type })),
  );
  const isExecutors = pathname.startsWith(ROOT_HREF);

  return (
    <SettingsGroup
      label="Executors"
      icon={IconCpu}
      href={ROOT_HREF}
      isActive={pathname === ROOT_HREF}
      defaultExpanded={isExecutors}
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
