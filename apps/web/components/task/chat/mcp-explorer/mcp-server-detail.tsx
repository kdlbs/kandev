"use client";

import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import {
  getMcpCatalogState,
  getMcpToolCounts,
  isKandevMcpServer,
  mcpStatusLabelKey,
} from "./mcp-explorer-view-model";

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

function McpToolCatalog({
  server,
  catalogState,
  counts,
}: {
  server: MCPAttachmentServer;
  catalogState: ReturnType<typeof getMcpCatalogState>;
  counts: ReturnType<typeof getMcpToolCounts>;
}) {
  const { t } = useTranslation();

  return (
    <section data-testid="mcp-tool-catalog" className="space-y-3">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h4 className="font-medium">{t("task:mcpToolCatalog")}</h4>
        {catalogState === "loaded" && (
          <span className="text-xs text-muted-foreground">
            {t("task:mcpToolCount", { count: counts.total })}
          </span>
        )}
      </div>

      {catalogState === "unavailable" && (
        <p className="text-sm text-muted-foreground">
          {isKandevMcpServer(server)
            ? t("task:mcpToolCatalogUnavailable")
            : t("task:mcpThirdPartyCatalogUnavailable")}
        </p>
      )}
      {catalogState === "not_loaded" && (
        <p className="text-sm text-muted-foreground">{t("task:mcpToolCatalogNotLoaded")}</p>
      )}
      {catalogState === "loaded" && (
        <>
          {counts.truncated && (
            <p className="text-xs text-muted-foreground">
              {t("task:mcpStoredToolCount", { count: counts.stored })}
            </p>
          )}
          {server.tools && server.tools.length > 0 ? (
            <div className="space-y-2">
              {server.tools.map((tool) => (
                <article
                  key={tool.name}
                  data-testid={`mcp-tool-row-${tool.name}`}
                  className="rounded-md border border-border/70 px-3 py-2"
                >
                  <div className="break-words font-mono text-sm">{tool.name}</div>
                  {tool.description && (
                    <p className="mt-1 break-words text-sm text-muted-foreground">
                      {tool.description}
                    </p>
                  )}
                </article>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t("task:mcpNoTools")}</p>
          )}
          {counts.truncated && (
            <p className="text-sm text-muted-foreground">
              {t("task:mcpToolCatalogTruncated", { count: counts.total })}
            </p>
          )}
        </>
      )}
    </section>
  );
}

export function McpServerDetail({
  server,
  onBack,
}: {
  server: MCPAttachmentServer | null;
  onBack?: () => void;
}) {
  const { t } = useTranslation();

  if (!server) {
    return (
      <div data-testid="mcp-server-detail" className="flex min-h-full items-center justify-center">
        <p className="text-sm text-muted-foreground">{t("task:mcpNoServers")}</p>
      </div>
    );
  }

  const catalogState = getMcpCatalogState(server);
  const counts = getMcpToolCounts(server);
  const observedAt = formatObservedAt(server.tools_listed_at);

  return (
    <div data-testid="mcp-server-detail" className="min-h-full space-y-5">
      {onBack && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-11 gap-2 px-2"
          aria-label={t("task:mcpBackToServers")}
          onClick={onBack}
        >
          <IconArrowLeft className="h-4 w-4" aria-hidden="true" />
          {t("task:mcpBackToServers")}
        </Button>
      )}

      <div className="space-y-2">
        <div className="flex min-w-0 items-center gap-2">
          <span
            className={cn(
              "h-2.5 w-2.5 shrink-0 rounded-full",
              server.status === "active" ? "bg-emerald-500" : "bg-muted-foreground/50",
            )}
            aria-hidden="true"
          />
          <h3 className="min-w-0 break-words text-lg font-semibold">{server.name}</h3>
        </div>
        <p className="text-sm text-muted-foreground">{t(mcpStatusLabelKey(server.status))}</p>
        {server.summary && <p className="text-sm text-muted-foreground">{server.summary}</p>}
      </div>

      <dl className="space-y-2 text-sm">
        <MetadataRow label={t("task:mcpTransport")} value={server.transport} />
        <MetadataRow label={t("task:mcpTarget")} value={server.target} />
        <MetadataRow label={t("task:mcpConnectionId")} value={server.connection_id} />
        <MetadataRow label={t("task:mcpLastObserved")} value={observedAt ?? undefined} />
      </dl>

      <McpToolCatalog server={server} catalogState={catalogState} counts={counts} />
    </div>
  );
}
