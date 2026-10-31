"use client";

import { IconCircle, IconCircleCheck, IconCircleX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { mcpStatusPresentation } from "./mcp-explorer-view-model";
import { McpStatusDot } from "./mcp-status-presentation";

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
    <div
      data-testid="mcp-server-list"
      className="h-full min-h-0 overflow-y-auto text-[13px] leading-5"
    >
      <div className="mb-2 px-2 text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
        {t("task:mcpServerList")}
      </div>
      {servers.length === 0 ? (
        <p className="px-1 py-4 text-[13px] text-muted-foreground">{t("task:mcpNoServers")}</p>
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
                  "h-auto min-h-7 w-full items-start justify-start gap-2 whitespace-normal px-2 py-2 text-left text-[13px] max-md:h-auto max-md:min-h-11 [@media(pointer:coarse)]:h-auto [@media(pointer:coarse)]:min-h-11",
                  selected && "bg-muted/60",
                )}
              >
                <McpStatusDot status={server.status} className="mt-1.5" />
                <span className="min-w-0 flex-1 break-words">
                  <span className="block font-medium">{server.name}</span>
                  <span className="block text-xs text-muted-foreground">
                    {t(mcpStatusPresentation(server).labelKey)}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {t(mcpStatusPresentation(server).explanationKey)}
                  </span>
                </span>
                <StatusIcon
                  className={cn("mt-0.5 h-3.5 w-3.5 shrink-0", mcpStatusIconClass(server.status))}
                  aria-hidden="true"
                />
              </Button>
            );
          })}
        </div>
      )}
    </div>
  );
}
