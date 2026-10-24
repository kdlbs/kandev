"use client";

import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import type { MCPSessionSelectionState } from "@/lib/types/http-mcp";
import { mcpSelectionMarker } from "./mcp-selection-presentation";
import { McpSelectionNotice } from "./mcp-selection-notice";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { mcpStatusPresentation } from "./mcp-explorer-view-model";

export const MCP_STATUS_DOT_CLASS: Record<MCPAttachmentServer["status"], string> = {
  active: "bg-emerald-500",
  connected: "bg-amber-500",
  delivered: "bg-amber-500",
  failed: "bg-destructive",
  filtered: "bg-muted-foreground/50",
  unavailable: "bg-muted-foreground/50",
  unknown: "bg-muted-foreground/50",
};

export function McpStatusDot({
  status,
  className,
}: {
  status: MCPAttachmentServer["status"];
  className?: string;
}) {
  return (
    <span
      className={cn("h-2 w-2 shrink-0 rounded-full", MCP_STATUS_DOT_CLASS[status], className)}
      aria-hidden="true"
    />
  );
}

export function McpStatusTooltip({
  servers,
  selectionState,
}: {
  servers: MCPAttachmentServer[];
  selectionState?: MCPSessionSelectionState;
}) {
  const { t } = useTranslation();
  return (
    <div data-testid="mcp-status-popover" className="max-w-96 space-y-2 text-[13px] leading-5">
      {selectionState && mcpSelectionMarker(selectionState) && (
        <div data-testid="mcp-tooltip-selection-state" className="space-y-1 border-b pb-3">
          <div className="font-medium">{t("task:mcpSavedChanges")}</div>
          <McpSelectionNotice state={selectionState} compact />
        </div>
      )}
      <div className="font-medium">{t("task:mcpCurrentAgent")}</div>
      {servers.length === 0 ? (
        <div className="text-muted-foreground">{t("task:mcpNoServers")}</div>
      ) : (
        <div className="space-y-1.5">
          {servers.map((server) => (
            <div
              key={server.name}
              data-testid={`mcp-tooltip-server-${server.name}`}
              className="grid min-w-0 grid-cols-[0.5rem_minmax(0,1fr)] gap-x-2"
            >
              <McpStatusDot status={server.status} className="mt-1.5" />
              <div className="min-w-0 space-y-0.5 break-words">
                <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
                  <span className="min-w-0 font-medium">{server.name}</span>
                  <span className="min-w-0 text-muted-foreground">
                    {t(mcpStatusPresentation(server).labelKey)}
                  </span>
                </div>
                <div className="text-muted-foreground">
                  {t(mcpStatusPresentation(server).explanationKey)}
                </div>
                {server.summary && (!server.reason_code || server.status === "failed") && (
                  <div className="text-muted-foreground">{server.summary}</div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
