"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import {
  EXECUTOR_TYPE_MAP,
  executorTypeLabel,
} from "@/app/settings/executors/new/[type]/executor-types";
import { useAppStore } from "@/components/state-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import { integrationTitle } from "./settings-breadcrumb-labels";
import {
  resolveSettingsBreadcrumbs,
  type CrumbValues,
  type SettingsBreadcrumbs,
} from "./settings-breadcrumbs";

/**
 * Resolves the settings breadcrumb for `pathname`.
 *
 * The route table in `settings-breadcrumbs.ts` says *which* record names a
 * crumb, and this hook is the only place that knows *where those names live*.
 *
 * Every lookup that is scoped by a parent in the URL checks that parent. A name
 * read from the wrong workspace or the wrong executor would put a label on
 * screen that the route never referred to, which is the same failure the
 * integration copy action had: plausible, wrong, and silent.
 */
export function useSettingsBreadcrumbs(pathname: string): SettingsBreadcrumbs {
  const { t } = useTranslation();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const availableAgents = useAppStore((s) => s.availableAgents.items);
  const settingsAgents = useAppStore((s) => s.settingsAgents.items);
  const executors = useAppStore((s) => s.executors.items);
  const automations = useAppStore((s) => s.automations.items);

  const values = useMemo<CrumbValues>(
    () => ({
      workspaceName: ({ workspaceId }) =>
        workspaces.find((workspace) => workspace.id === workspaceId)?.name ?? null,

      agentDisplayName: ({ agentName }) =>
        availableAgents.find((agent) => agent.name === agentName)?.display_name ?? null,

      agentProfileName: ({ agentName, agentProfileId }) => {
        if (!agentProfileId) return null;
        const scoped = agentName
          ? settingsAgents.filter((agent) => agent.name === agentName)
          : settingsAgents;
        return (
          scoped.flatMap((agent) => agent.profiles).find((profile) => profile.id === agentProfileId)
            ?.name ?? null
        );
      },

      automationName: ({ workspaceId, automationId }) => {
        if (!automationId) return null;
        const automation = automations.find((item) => item.id === automationId);
        // The store holds one workspace's automations at a time (`useAutomations`
        // refetches on switch), so a hit from another workspace is stale data.
        if (!automation || (workspaceId && automation.workspace_id !== workspaceId)) return null;
        return automation.name;
      },

      executorName: ({ executorId }) =>
        executors.find((executor) => executor.id === executorId)?.name ?? null,

      executorProfileName: ({ executorId, executorProfileId }) => {
        if (!executorProfileId) return null;
        for (const executor of executors) {
          if (executorId && executor.id !== executorId) continue;
          const profile = executor.profiles?.find((item) => item.id === executorProfileId);
          if (profile) return profile.name;
        }
        return null;
      },

      // The same label the create page's own header shows.
      executorTypeTitle: ({ executorType }) => {
        const info = executorType ? EXECUTOR_TYPE_MAP[executorType] : undefined;
        if (!info) return null;
        return t("executors:newTypeProfile", { type: executorTypeLabel(info, t) });
      },

      integrationTitle: ({ integrationSlug }) => integrationTitle(integrationSlug),

      pluginName: ({ pluginId }) =>
        pluginId ? (pluginRegistry.getPluginName(pluginId) ?? null) : null,
    }),
    [workspaces, availableAgents, settingsAgents, executors, automations, t],
  );

  return resolveSettingsBreadcrumbs(pathname, t, values);
}
