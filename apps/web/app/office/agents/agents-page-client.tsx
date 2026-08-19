"use client";

import { useEffect, useRef, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { useRoutingPreview } from "@/hooks/domains/office/use-routing-preview";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { AgentCard } from "./components/agent-card";
import { CreateAgentDialog } from "./components/create-agent-dialog";
import { EmptyState } from "../components/shared/empty-state";
import { PageHeader } from "../components/shared/page-header";
import { useTranslation } from "react-i18next";

type AgentsPageClientProps = {
  initialAgents: AgentProfile[];
  initialWorkspaceId?: string | null;
};

export function AgentsPageClient({ initialAgents, initialWorkspaceId }: AgentsPageClientProps) {
  const { t } = useTranslation();
  const agents = useAppStore(selectOfficeAgentProfiles);
  const setOfficeAgentProfiles = useAppStore((s) => s.setOfficeAgentProfiles);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [showCreate, setShowCreate] = useState(false);
  // Mounting these hooks fetches workspace routing config + preview once;
  // every agent card reads the resolved preview from the store.
  useWorkspaceRouting(workspaceId);
  useRoutingPreview(workspaceId);

  // Hydrate the SSR payload exactly once: it belongs to the workspace that was
  // active at SSR time, and re-running on a workspace switch would file it
  // under the new workspace.
  const initialHydratedRef = useRef(false);
  useEffect(() => {
    if (
      initialHydratedRef.current ||
      !workspaceId ||
      (initialWorkspaceId !== undefined && initialWorkspaceId !== workspaceId) ||
      initialAgents.length === 0
    ) {
      return;
    }
    initialHydratedRef.current = true;
    setOfficeAgentProfiles(workspaceId, initialAgents);
  }, [initialAgents, initialWorkspaceId, setOfficeAgentProfiles, workspaceId]);

  return (
    <div className="p-6 space-y-4">
      <PageHeader
        title={t("office:agents")}
        action={
          <Button size="sm" className="cursor-pointer" onClick={() => setShowCreate(true)}>
            <IconPlus className="h-4 w-4 mr-1" />
            {t("office:newAgent")}
          </Button>
        }
      />

      {agents.length === 0 ? (
        <EmptyState
          message={t("office:noAgentsYet")}
          description={t("office:createACeoAgentToStart")}
          action={
            <Button
              variant="outline"
              size="sm"
              className="cursor-pointer"
              onClick={() => setShowCreate(true)}
            >
              <IconPlus className="h-4 w-4 mr-1" />
              {t("office:createAgent")}
            </Button>
          }
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {agents.map((agent) => (
            <AgentCard key={agent.id} agent={agent} />
          ))}
        </div>
      )}

      <CreateAgentDialog open={showCreate} onOpenChange={setShowCreate} />
    </div>
  );
}
