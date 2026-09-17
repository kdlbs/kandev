import { AgentAvatar } from "@/components/shared/agent-avatar";
import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { OpenOrchestratorConversation } from "./orchestrator-connections";
import { toast } from "@/lib/toast/sonner";
import { useKanbanOnboardingComplete } from "@/hooks/use-kanban-onboarding-complete";
import {
  useOrchestrationData,
  notifyOrchestrationChanged,
} from "@/hooks/domains/orchestration/use-orchestration-data";
import {
  listOrchestrators,
  orchestratorsHref,
  orchestratorHref,
  setOrchestratorStatus,
  type Orchestrator,
} from "@/lib/api/domains/orchestration-api";
import { OrchestrationGate } from "./orchestration-gate";
export function OrchestratorsPage({ workspaceId }: { workspaceId: string }) {
  return (
    <OrchestrationGate>
      <OrchestratorList key={workspaceId} workspaceId={workspaceId} />
    </OrchestrationGate>
  );
}
function OrchestratorList({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const onboarded = useKanbanOnboardingComplete();
  const load = useCallback(() => listOrchestrators(workspaceId), [workspaceId]);
  const { data, error } = useOrchestrationData(load);
  return (
    <section className="space-y-5" data-testid="workspace-orchestrators">
      <h2 className="text-xl font-semibold">{t("orchestration:orchestration")}</h2>
      <p className="text-sm text-muted-foreground">{t("orchestration:orchestratorsHint")}</p>
      <div className="flex flex-wrap gap-4">
        <Link
          className="underline"
          href={onboarded ? `${orchestratorsHref(workspaceId)}/new` : "/?home=overview"}
        >
          {t(onboarded ? "orchestration:addOrchestrator" : "orchestration:completeKanbanFirst")}
        </Link>
        <Link className="underline" href="/settings/orchestration">
          {t("orchestration:manageRoles")}
        </Link>
        <Link className="underline" href={`/?workspaceId=${workspaceId}`}>
          {t("orchestration:workspaceBoard")}
        </Link>
      </div>
      {error && <p role="alert">{error}</p>}
      {!data && !error && <p>{t("common:loading")}</p>}
      {data?.orchestrators.length === 0 && <p>{t("orchestration:noOrchestrators")}</p>}
      <div className="grid gap-4 lg:grid-cols-2">
        {data?.orchestrators.map((o) => (
          <OrchestratorCard key={o.id} item={o} />
        ))}
      </div>
    </section>
  );
}
function OrchestratorCard({ item }: { item: Orchestrator }) {
  const { t } = useTranslation();
  const toggle = async () => {
    try {
      await setOrchestratorStatus(
        item.workspace_id,
        item.id,
        item.status === "paused" ? "idle" : "paused",
      );
      notifyOrchestrationChanged();
    } catch (e) {
      toast.error(String(e));
    }
  };
  return (
    <article className="rounded-lg border p-4 space-y-3" data-testid="orchestrator-card">
      <Link
        className="flex items-center gap-2 text-lg font-semibold underline"
        href={orchestratorHref(item.workspace_id, item.id)}
      >
        <AgentAvatar name={item.name} icon={item.icon} />
        {item.name}
      </Link>
      <p className="text-sm text-muted-foreground">
        {t(`orchestration:agentStatus_${item.status}`, { defaultValue: item.status })}
      </p>
      <p className="text-sm whitespace-pre-wrap">{item.context}</p>
      <div className="flex flex-wrap gap-3">
        <OpenOrchestratorConversation workspaceId={item.workspace_id} id={item.id} />
        <Button variant="outline" onClick={toggle}>
          {t(
            item.status === "paused"
              ? "orchestration:resumeOrchestrator"
              : "orchestration:pauseOrchestrator",
          )}
        </Button>
        <Link className="self-center underline" href={orchestratorHref(item.workspace_id, item.id)}>
          {t("orchestration:configureOrchestrator")}
        </Link>
      </div>
    </article>
  );
}
