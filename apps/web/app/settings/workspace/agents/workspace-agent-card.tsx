"use client";

import { AgentExecutionProfile } from "./agent-execution-profile";
import { AgentDelegationContext } from "./agent-delegation-context";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { useAgentRoute } from "@/hooks/domains/office/use-agent-route";
import { useAppStore } from "@/components/state-provider";
import { OpenConversationButton } from "@/app/office/agents/[id]/components/open-conversation-button";
import type { OfficeAgentProfile } from "@/lib/types/agent-profile";
import { toast } from "@/lib/toast/sonner";

export function WorkspaceAgentCard({
  agent,
  chief,
  selectChief,
}: {
  agent: OfficeAgentProfile;
  chief: boolean;
  selectChief: (id: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const route = useAgentRoute(agent.id);
  const executors = useAppStore((s) => s.executors.items);
  const executor = executors
    .flatMap((e) => (e.profiles ?? []).map((p) => ({ ...p, host: e.name })))
    .find((p) => p.id === agent.executorPreference?.executor_profile_id);
  const [saving, setSaving] = useState(false);
  const changeChief = async () => {
    setSaving(true);
    try {
      await selectChief(chief ? "" : agent.id);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };
  const query = `?workspaceId=${encodeURIComponent(agent.workspaceId)}`;
  return (
    <article className="rounded-lg border p-4 space-y-3" data-testid="workspace-agent-card">
      <div className="flex items-center gap-2">
        <h3 className="font-medium">{agent.name}</h3>
        {chief && <Badge>{t("office:primaryChief")}</Badge>}
        <Badge variant="outline">{agent.status}</Badge>
      </div>
      <p className="text-sm text-muted-foreground">
        {executor ? `${executor.host}: ${executor.name}` : t("office:inheritFromProjectWorkspace")}
      </p>
      <p className="text-sm">
        {route.error ?? (route.data?.preview.current_model || t("office:checkAgentConfiguration"))}
      </p>
      <AgentExecutionProfile
        agentId={agent.id}
        workspaceId={agent.workspaceId}
        initialProfileId={agent.executionProfileId}
      />
      <AgentDelegationContext agentId={agent.id} initial={agent.delegationContext ?? ""} />
      <div className="flex flex-wrap gap-2">
        <OpenConversationButton agentId={agent.id} workspaceId={agent.workspaceId} />
        <Button
          variant="outline"
          className="cursor-pointer"
          disabled={saving}
          onClick={changeChief}
        >
          {t(chief ? "office:removePrimaryChief" : "office:makePrimaryChief")}
        </Button>
      </div>
      <div className="flex flex-wrap gap-3 text-sm">
        {(["dashboard", "configuration", "instructions", "permissions", "runs"] as const).map(
          (tab) => (
            <Link
              key={tab}
              className="underline cursor-pointer"
              href={`/office/agents/${agent.id}/${tab}${query}`}
            >
              {t(`office:setupLink_${tab}`)}
            </Link>
          ),
        )}
        <Link className="underline cursor-pointer" href={`/office/routines${query}`}>
          {t("office:routines")}
        </Link>
      </div>
    </article>
  );
}
