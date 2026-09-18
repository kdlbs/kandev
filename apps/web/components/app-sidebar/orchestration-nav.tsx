import { IconSitemap } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { coordinatorHref } from "@/lib/api/domains/orchestration-api";
import { useRouter } from "@/lib/routing/client-router";
import { useKanbanOnboardingComplete } from "@/hooks/use-kanban-onboarding-complete";
import { useWorkspaceOrchestrators } from "@/hooks/domains/orchestration/use-orchestrator-conversation";
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
        label={t("orchestration:coordinator")}
        href={coordinatorHref(workspaceId)}
        collapsed={collapsed}
        onClick={
          onNavigate
            ? () => {
                router.push(coordinatorHref(workspaceId));
                onNavigate();
              }
            : undefined
        }
        testId="workspace-coordinator-link"
      />
    </div>
  );
}
