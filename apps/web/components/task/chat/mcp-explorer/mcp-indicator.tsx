"use client";

import { IconPlugConnected, IconPlugConnectedX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { MCPAttachmentHistory } from "@/lib/state/slices/session-runtime/types";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { buildMcpExplorerServers } from "./mcp-explorer-view-model";
import { McpServerExplorer } from "./mcp-server-explorer";

export function McpIndicator({
  mcpServers,
  attachmentHistory,
}: {
  mcpServers: string[];
  attachmentHistory?: MCPAttachmentHistory;
}) {
  const { t } = useTranslation();
  const servers = buildMcpExplorerServers(mcpServers, attachmentHistory);
  const hasMcp = servers.length > 0;
  const Icon = hasMcp ? IconPlugConnected : IconPlugConnectedX;
  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={t("task:showMcpConnectionStatus")}
      className={cn(
        "h-11 w-11 cursor-pointer rounded-md active:scale-95 md:h-7 md:w-7 md:active:scale-100",
        hasMcp ? "text-foreground" : "text-muted-foreground/40",
      )}
      data-testid="mcp-status-trigger"
    >
      <Icon className="h-4 w-4" />
    </Button>
  );

  return <McpServerExplorer trigger={trigger} servers={servers} />;
}
