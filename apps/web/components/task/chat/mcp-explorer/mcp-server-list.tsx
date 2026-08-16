"use client";

import { IconCircle, IconCircleCheck, IconCircleX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { mcpStatusLabelKey } from "./mcp-explorer-view-model";

const STATUS_DOT_CLASS: Record<MCPAttachmentServer["status"], string> = {
  active: "bg-emerald-500",
  connected: "bg-amber-500",
  delivered: "bg-amber-500",
  failed: "bg-destructive",
  filtered: "bg-muted-foreground/50",
  unavailable: "bg-muted-foreground/50",
  unknown: "bg-muted-foreground/50",
};

function mcpStatusIcon(status: MCPAttachmentServer["status"]) {
  switch (status) {
    case "failed":
      return IconCircleX;
    case "active":
      return IconCircleCheck;
    default:
      return IconCircle;
  }
}

function mcpStatusIconClass(status: MCPAttachmentServer["status"]) {
  switch (status) {
    case "failed":
      return "text-destructive";
    case "active":
      return "text-emerald-500";
    default:
      return "text-muted-foreground";
  }
}

export function McpServerList({
  servers,
  selectedName,
  onSelect,
}: {
  servers: MCPAttachmentServer[];
  selectedName: string | null;
  onSelect: (name: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <div data-testid="mcp-server-list" className="h-full min-h-0 overflow-y-auto">
      <div className="mb-2 px-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {t("task:mcpServerList")}
      </div>
      {servers.length === 0 ? (
        <p className="px-1 py-4 text-sm text-muted-foreground">{t("task:mcpNoServers")}</p>
      ) : (
        <div className="space-y-1">
          {servers.map((server) => {
            const selected = server.name === selectedName;
            const StatusIcon = mcpStatusIcon(server.status);
            return (
              <Button
                key={server.name}
                type="button"
                variant="ghost"
                data-testid={`mcp-server-row-${server.name}`}
                aria-current={selected ? "true" : undefined}
                onClick={() => onSelect(server.name)}
                className={cn(
                  "min-h-11 w-full justify-start gap-2 px-2 text-left",
                  selected && "bg-muted/60",
                )}
              >
                <span
                  className={cn("h-2 w-2 shrink-0 rounded-full", STATUS_DOT_CLASS[server.status])}
                  aria-hidden="true"
                />
                <span className="min-w-0 flex-1 truncate">{server.name}</span>
                <StatusIcon
                  className={cn("h-3.5 w-3.5 shrink-0", mcpStatusIconClass(server.status))}
                  aria-hidden="true"
                />
                <span className="sr-only">{t(mcpStatusLabelKey(server.status))}</span>
              </Button>
            );
          })}
        </div>
      )}
    </div>
  );
}
