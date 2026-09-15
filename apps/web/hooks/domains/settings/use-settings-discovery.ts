"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { resolveSettingsDiscovery } from "@/lib/settings-discovery/resolve";

export function useSettingsDiscovery() {
  const { t } = useTranslation();
  const authEnabled = useFeature("auth");
  const multiTenancyEnabled = useFeature("multiTenancy");
  const authMode = useAppStore((state) => state.auth.mode);
  const role = useAppStore((state) => state.auth.user?.role);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const agents = useAppStore((state) => state.settingsAgents.items);
  const executors = useAppStore((state) => state.executors.items);
  const showAccount = authEnabled && authMode === "enabled";
  const showUsers = authEnabled && (role === undefined || role === "admin");
  // Operator-only in practice, but that answer costs a request; the feature
  // gate keeps the entry out of search on installs that never enabled it, and
  // the page turns a non-operator away.
  const showOrganizations = authEnabled && multiTenancyEnabled;

  return useMemo(
    () =>
      resolveSettingsDiscovery({
        t,
        showAccount,
        showUsers,
        showOrganizations,
        workspaces,
        agents,
        executors,
      }),
    [agents, executors, showAccount, showOrganizations, showUsers, t, workspaces],
  );
}
