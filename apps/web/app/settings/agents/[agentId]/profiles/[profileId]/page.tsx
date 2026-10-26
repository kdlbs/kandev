"use client";

import { AgentProfilePage } from "@/components/settings/agent-profile-page";
import { MCPProfileDestination } from "@/components/settings/mcp-profile-destination";
import { useParams, useSearchParams } from "@/lib/routing/client-router";
import { readMCPContextHint } from "@/lib/mcp/management-links";

export default function AgentProfileRoute() {
  const params = useParams();
  const hint = readMCPContextHint(useSearchParams(), "workspaceId");
  if (!hint.present) return <AgentProfilePage />;
  return (
    <MCPProfileDestination
      workspaceId={hint.value}
      agentKey={decodeURIComponent(params.agentId ?? "")}
      profileId={params.profileId ?? ""}
    />
  );
}
