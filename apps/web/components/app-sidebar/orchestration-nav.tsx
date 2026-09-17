import { AgentAvatar } from "@/app/office/components/agent-avatar";
import { IconSitemap } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { orchestratorsHref, type Orchestrator } from "@/lib/api/domains/orchestration-api";
import { useSearchParams, useRouter } from "@/lib/routing/client-router";
import { useKanbanOnboardingComplete } from "@/hooks/use-kanban-onboarding-complete";
import {
  useOrchestratorConversation,
  useWorkspaceOrchestrators,
} from "@/hooks/domains/orchestration/use-orchestrator-conversation";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

type NavProps = { workspaceId: string; collapsed: boolean; onNavigate?: () => void };
export function OrchestrationNav({ workspaceId, collapsed, onNavigate }: NavProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const onboarded = useKanbanOnboardingComplete();
  const { data } = useWorkspaceOrchestrators(workspaceId);
  if (!onboarded && !data?.orchestrators.length) return null;
  return (
    <div data-testid="workspace-orchestration-nav">
      <AppSidebarNavItem
        icon={IconSitemap}
        label={t("orchestration:orchestration")}
        href={orchestratorsHref(workspaceId)}
        collapsed={collapsed}
        onClick={
          onNavigate
            ? () => {
                router.push(orchestratorsHref(workspaceId));
                onNavigate();
              }
            : undefined
        }
      />
      {data?.orchestrators.map((item) => (
        <OrchestratorConversationNav
          key={item.id}
          item={item}
          collapsed={collapsed}
          onNavigate={onNavigate}
        />
      ))}
    </div>
  );
}
function OrchestratorConversationNav({
  item,
  collapsed,
  onNavigate,
}: {
  item: Orchestrator;
  collapsed: boolean;
  onNavigate?: () => void;
}) {
  const { open, busy } = useOrchestratorConversation(item.workspace_id, item.id, onNavigate);
  const params = useSearchParams();
  return (
    <AppSidebarNavItem
      icon={({ className }) => (
        <AgentAvatar name={item.name} icon={item.icon} size="sm" className={className} />
      )}
      label={item.name}
      collapsed={collapsed}
      onClick={open}
      disabled={busy}
      isActive={params.get("orchestratorId") === item.id}
      className={collapsed ? undefined : "pl-6"}
      testId={`orchestrator-chat-${item.id}`}
    />
  );
}
