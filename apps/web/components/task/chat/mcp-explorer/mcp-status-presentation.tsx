"use client";

import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { mcpStatusLabelKey } from "./mcp-explorer-view-model";

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

export function McpStatusTooltip({ servers }: { servers: MCPAttachmentServer[] }) {
  const { t } = useTranslation();
  return (
    <div data-testid="mcp-status-popover" className="max-w-96 space-y-2 text-[13px] leading-5">
      <div className="font-medium">{t("task:mcpServers")}</div>
      {servers.length === 0 ? (
        <div className="text-muted-foreground">{t("task:mcpNoServers")}</div>
      ) : (
        <div className="space-y-1.5">
          {servers.map((server) => (
            <div
              key={server.name}
              data-testid={`mcp-tooltip-server-${server.name}`}
              className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5"
            >
              <McpStatusDot status={server.status} />
              <span className="max-w-48 truncate font-medium">{server.name}</span>
              <span className="text-muted-foreground">{t(mcpStatusLabelKey(server.status))}</span>
              {server.summary && (
                <span className="basis-full pl-4 text-muted-foreground">{server.summary}</span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
