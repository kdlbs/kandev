"use client";

import {
  IconPlugConnected,
  IconPlugConnectedX,
  IconClock,
  IconRefresh,
  IconAlertCircle,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { MCPAttachmentHistory } from "@/lib/state/slices/session-runtime/types";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { buildMcpExplorerServers } from "./mcp-explorer-view-model";
import { McpServerExplorer } from "./mcp-server-explorer";

import { useId } from "react";
import type { TaskSessionMCPSelection } from "@/hooks/domains/session/use-task-session-mcp-selection";
import { mcpSelectionMarker, mcpSelectionStateLabelKey } from "./mcp-selection-presentation";

export function McpIndicator({
  mcpServers,
  attachmentHistory,
  selection,
}: {
  mcpServers: string[];
  attachmentHistory?: MCPAttachmentHistory;
  selection?: TaskSessionMCPSelection | null;
}) {
  const { t } = useTranslation();
  const statusId = useId();
  const marker = mcpSelectionMarker(selection?.state);
  const markerIcons = { pending: IconClock, failed: IconAlertCircle, deferred: IconRefresh };
  const MarkerIcon = markerIcons[marker ?? "deferred"];
  const markerColors = {
    failed: "text-destructive",
    pending: "text-amber-600 dark:text-amber-400",
    deferred: "text-blue-600 dark:text-blue-400",
  };
  const servers = buildMcpExplorerServers(mcpServers, attachmentHistory);
  const hasMcp = servers.length > 0;
  const Icon = hasMcp ? IconPlugConnected : IconPlugConnectedX;
  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={t("task:showMcpConnectionStatus")}
      aria-describedby={selection?.state ? statusId : undefined}
      className={cn(
        "relative h-11 w-11 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11 cursor-pointer rounded-md active:scale-95 md:h-7 md:w-7 md:active:scale-100",
        hasMcp ? "text-foreground" : "text-muted-foreground/40",
      )}
      data-testid="mcp-status-trigger"
    >
      <Icon className="h-4 w-4" />
      {marker && (
        <span
          data-testid="mcp-selection-marker"
          data-state={marker}
          className="absolute right-0 top-0 rounded-full bg-background"
          aria-hidden="true"
        >
          <MarkerIcon className={cn("size-3", markerColors[marker])} />
        </span>
      )}
      {selection?.state && (
        <span id={statusId} className="sr-only" role="status">
          {t(mcpSelectionStateLabelKey(selection.state))}
        </span>
      )}
    </Button>
  );

  return <McpServerExplorer trigger={trigger} servers={servers} selection={selection} />;
}
