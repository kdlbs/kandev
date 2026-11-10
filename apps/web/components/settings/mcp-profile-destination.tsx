"use client";

import { useTranslation } from "react-i18next";
import { Card, CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { AgentProfilePage } from "./agent-profile-page";
import { ProfileMCPSelectionCard } from "@/app/settings/agents/[agentId]/profile-mcp-selection-card";
import { useMCPProfileOwner } from "@/hooks/domains/workspace/use-mcp-profile-owner";

export function MCPProfileDestination({
  workspaceId,
  agentKey,
  profileId,
}: {
  workspaceId: string | null;
  agentKey: string;
  profileId: string;
}) {
  const { t } = useTranslation();
  const { owner, loading, canRetry, reload } = useMCPProfileOwner(workspaceId, agentKey, profileId);
  if (!owner)
    return (
      <Card>
        <CardContent className="space-y-3 py-6">
          <p role={loading ? "status" : "alert"}>
            {t(loading ? "settings:mcpLoading" : "settings:mcpOwnerUnavailable")}
          </p>
          {canRetry && (
            <Button variant="outline" onClick={reload}>
              {t("agents:retry")}
            </Button>
          )}
        </CardContent>
      </Card>
    );
  if (!owner.workspace_id) return <AgentProfilePage />;
  if (!owner.supports_mcp || owner.kind !== "concrete")
    return <p role="alert">{t("settings:mcpOwnerUnavailable")}</p>;
  return (
    <div className="min-w-0 space-y-6" data-testid="workspace-profile-mcp-destination">
      <h2 className="break-words text-2xl font-bold">{owner.name}</h2>
      <ProfileMCPSelectionCard
        key={`${workspaceId}:${owner.id}`}
        profileId={owner.id}
        workspaceId={owner.workspace_id}
      />
    </div>
  );
}
