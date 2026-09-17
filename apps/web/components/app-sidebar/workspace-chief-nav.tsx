"use client";

import { OrchestrationNav } from "./orchestration-nav";
import { useKanbanOnboardingComplete } from "@/hooks/use-kanban-onboarding-complete";
import { IconMessageCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useWorkspaceAgents } from "@/hooks/domains/office/use-workspace-agents";
import { openAgentConversation } from "@/lib/api/domains/workspace-agents-api";
import { useRouter } from "@/lib/routing/client-router";
import { workspaceSettingsHref } from "@/lib/settings/workspace-settings-tabs";
import { toast } from "@/lib/toast/sonner";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

export function WorkspaceChiefNav({
  collapsed = false,
  onNavigate,
}: {
  collapsed?: boolean;
  onNavigate?: () => void;
}) {
  const onboarded = useKanbanOnboardingComplete();
  const orchestration = useAppStore((s) => s.features.orchestration);
  const enabled = useAppStore((s) => s.features.office);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  if (orchestration && workspaceId)
    return (
      <OrchestrationNav
        key={workspaceId}
        workspaceId={workspaceId}
        collapsed={collapsed}
        onNavigate={onNavigate}
      />
    );
  return enabled && onboarded && workspaceId ? (
    <ChiefLink
      key={workspaceId}
      workspaceId={workspaceId}
      collapsed={collapsed}
      onNavigate={onNavigate}
    />
  ) : null;
}
function ChiefLink({
  workspaceId,
  collapsed,
  onNavigate,
}: {
  workspaceId: string;
  collapsed: boolean;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const { agents, chiefId, loading, error } = useWorkspaceAgents(workspaceId);
  const chief = agents.find((a) => a.id === chiefId);
  const open = async () => {
    try {
      const result = await openAgentConversation(workspaceId, chiefId);
      onNavigate?.();
      router.push(
        `/workspace/conversations/${result.channel.task_id}?workspaceId=${encodeURIComponent(workspaceId)}`,
      );
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };
  if (loading) return null;
  return (
    <AppSidebarNavItem
      icon={IconMessageCircle}
      collapsed={collapsed}
      label={chief ? chief.name : t(error ? "office:workspaceAgents" : "office:setupChief")}
      href={chief ? undefined : workspaceSettingsHref(workspaceId, "agents")}
      onClick={chief ? open : undefined}
    />
  );
}
