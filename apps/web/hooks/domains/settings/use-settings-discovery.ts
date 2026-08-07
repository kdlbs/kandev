"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { resolveSettingsDiscovery } from "@/lib/settings-discovery/resolve";

export function useSettingsDiscovery() {
  const { t } = useTranslation();
  const authEnabled = useFeature("auth");
  const authMode = useAppStore((state) => state.auth.mode);
  const role = useAppStore((state) => state.auth.user?.role);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const agents = useAppStore((state) => state.settingsAgents.items);
  const executors = useAppStore((state) => state.executors.items);
  const showAccount = authEnabled && authMode === "enabled";
  const showUsers = authEnabled && (role === undefined || role === "admin");

  return useMemo(
    () =>
      resolveSettingsDiscovery({
        t,
        showAccount,
        showUsers,
        workspaces,
        agents,
        executors,
      }),
    [agents, executors, showAccount, showUsers, t, workspaces],
  );
}
