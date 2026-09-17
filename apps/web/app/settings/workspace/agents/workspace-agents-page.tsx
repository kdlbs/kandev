"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { useKanbanOnboardingComplete } from "@/hooks/use-kanban-onboarding-complete";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useWorkspaceAgents } from "@/hooks/domains/office/use-workspace-agents";
import { ConnectAgentDialog } from "./connect-agent-dialog";
import { WorkspaceAgentCard } from "./workspace-agent-card";

export function WorkspaceAgentsPage({ workspaceId }: { workspaceId: string }) {
  const completed = useKanbanOnboardingComplete();
  const enabled = useAppStore((s) => s.features.office);
  const { t } = useTranslation();
  if (!completed)
    return (
      <Link className="underline cursor-pointer" href="/?home=overview">
        {t("office:completeKanbanFirst")}
      </Link>
    );
  if (!enabled) return <p>{t("office:agentsFeatureRequired")}</p>;
  return <AgentConnections key={workspaceId} workspaceId={workspaceId} />;
}

function AgentConnections({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const workspace = useAppStore((s) => s.workspaces.items.find((w) => w.id === workspaceId));
  const data = useWorkspaceAgents(workspaceId);
  const [open, setOpen] = useState(false);
  return (
    <div className="space-y-5" data-testid="workspace-agents-settings">
      <div>
        <h2 className="text-lg font-semibold">{t("office:workspaceAgents")}</h2>
        <p className="text-sm text-muted-foreground">{t("office:workspaceAgentsHint")}</p>
      </div>
      <Button onClick={() => setOpen(true)} disabled={!workspace} className="cursor-pointer">
        {t("office:connectWorkspaceAgent")}
      </Button>
      {data.error && <p role="alert">{data.error}</p>}
      {data.loading && <p>{t("common:loading")}</p>}
      {!data.loading && !data.error && data.agents.length === 0 && (
        <p>{t("office:workspaceAgentsEmpty")}</p>
      )}
      <div className="grid gap-4 lg:grid-cols-2">
        {data.agents.map((agent) => (
          <WorkspaceAgentCard
            key={agent.id}
            agent={agent}
            chief={data.chiefId === agent.id}
            selectChief={data.selectChief}
          />
        ))}
      </div>
      {open && (
        <ConnectAgentDialog workspaceId={workspaceId} open={open} onClose={() => setOpen(false)} />
      )}
    </div>
  );
}
