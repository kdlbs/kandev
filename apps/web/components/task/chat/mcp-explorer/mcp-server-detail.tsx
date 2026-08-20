"use client";

import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { useTranslation } from "react-i18next";
import { getMcpToolCounts, mcpStatusLabelKey } from "./mcp-explorer-view-model";
import { McpStatusDot } from "./mcp-status-presentation";

function formatObservedAt(value?: string) {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function MetadataRow({ label, value }: { label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="grid gap-1 sm:grid-cols-[8rem_minmax(0,1fr)] sm:gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{value}</dd>
    </div>
  );
}

export function McpServerSummary({ server }: { server: MCPAttachmentServer }) {
  const { t } = useTranslation();
  const counts = getMcpToolCounts(server);
  const observedAt = formatObservedAt(server.tools_listed_at);
  return (
    <div
      data-testid="mcp-server-detail"
      className="space-y-2 border-b border-border/70 pb-3 text-[13px] leading-5"
    >
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <McpStatusDot status={server.status} className="h-2.5 w-2.5" />
        <h3 className="min-w-0 break-words text-[13px] font-semibold">{server.name}</h3>
        <span className="text-[13px] text-muted-foreground">
          {t(mcpStatusLabelKey(server.status))}
        </span>
        <span className="ml-auto text-[13px] text-muted-foreground">
          {t("task:mcpToolCount", { count: counts.total })}
        </span>
      </div>
      {server.summary && <p className="text-[13px] text-muted-foreground">{server.summary}</p>}
      <details className="text-[13px]">
        <summary className="min-h-11 cursor-pointer content-center text-muted-foreground">
          {t("task:mcpConnectionDetails")}
        </summary>
        <dl className="space-y-2 pb-1 pt-2">
          <MetadataRow label={t("task:mcpTransport")} value={server.transport} />
          <MetadataRow label={t("task:mcpTarget")} value={server.target} />
          <MetadataRow label={t("task:mcpConnectionId")} value={server.connection_id} />
          <MetadataRow label={t("task:mcpLastObserved")} value={observedAt ?? undefined} />
        </dl>
      </details>
    </div>
  );
}
