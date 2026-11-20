"use client";

import { MCPSettings } from "@/components/settings/workspaces/mcp-settings";

export default function WorkspaceMCPServersPage({ workspaceId }: { workspaceId: string }) {
  return <MCPSettings workspaceId={workspaceId} />;
}
