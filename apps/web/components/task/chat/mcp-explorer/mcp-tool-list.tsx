"use client";

import { IconArrowLeft, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { useTranslation } from "react-i18next";
import { getMcpCatalogState, getMcpToolCounts, isKandevMcpServer } from "./mcp-explorer-view-model";
import { McpServerSummary } from "./mcp-server-detail";

function BackToServers({ onBack }: { onBack: () => void }) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="min-h-11 cursor-pointer gap-2 px-2 text-[13px]"
      aria-label={t("task:mcpBackToServers")}
      onClick={onBack}
    >
      <IconArrowLeft className="h-4 w-4" aria-hidden="true" />
      {t("task:mcpBackToServers")}
    </Button>
  );
}

function CatalogNotice({ server }: { server: MCPAttachmentServer }) {
  const { t } = useTranslation();
  const state = getMcpCatalogState(server);
  if (state === "unavailable") {
    return (
      <p className="text-[13px] text-muted-foreground">
        {isKandevMcpServer(server)
          ? t("task:mcpToolCatalogUnavailable")
          : t("task:mcpThirdPartyCatalogUnavailable")}
      </p>
    );
  }
  if (state === "not_loaded") {
    return <p className="text-[13px] text-muted-foreground">{t("task:mcpToolCatalogNotLoaded")}</p>;
  }
  return null;
}

function ToolRows({
  server,
  onSelect,
}: {
  server: MCPAttachmentServer;
  onSelect: (name: string) => void;
}) {
  const { t } = useTranslation();
  if (server.tools && server.tools.length > 0) {
    return (
      <div className="space-y-1">
        {server.tools.map((tool) => (
          <Button
            key={tool.name}
            type="button"
            variant="ghost"
            data-testid={`mcp-tool-row-${tool.name}`}
            data-tool-name={tool.name}
            className="min-h-11 w-full cursor-pointer justify-start gap-3 px-3 text-left text-[13px]"
            onClick={() => onSelect(tool.name)}
          >
            <span className="min-w-0 flex-1 truncate font-mono text-[13px]">{tool.name}</span>
            {tool.estimated_tokens !== undefined && (
              <span className="shrink-0 text-[13px] text-muted-foreground">
                {t("task:mcpTokenEstimate", { count: tool.estimated_tokens })}
              </span>
            )}
            <IconChevronRight className="h-4 w-4 shrink-0" aria-hidden="true" />
          </Button>
        ))}
      </div>
    );
  }
  if (getMcpCatalogState(server) === "loaded") {
    return <p className="text-[13px] text-muted-foreground">{t("task:mcpNoTools")}</p>;
  }
  return null;
}

export function McpToolList({
  server,
  onSelect,
  onBack,
  scrollRef,
}: {
  server: MCPAttachmentServer;
  onSelect: (name: string) => void;
  onBack?: () => void;
  scrollRef: React.RefObject<HTMLDivElement | null>;
}) {
  const { t } = useTranslation();
  const counts = getMcpToolCounts(server);
  const hasEstimates = server.tools?.some((tool) => tool.estimated_tokens !== undefined);
  return (
    <div data-testid="mcp-tool-list" className="flex h-full min-h-0 flex-col text-[13px] leading-5">
      <div className="shrink-0">
        {onBack && <BackToServers onBack={onBack} />}
        <McpServerSummary server={server} />
        {hasEstimates && (
          <p className="py-2 text-[13px] text-muted-foreground">{t("task:mcpTokenEstimateHelp")}</p>
        )}
        {counts.truncated && (
          <p className="py-2 text-[13px] text-muted-foreground">
            {t("task:mcpStoredToolCount", { count: counts.stored })}
          </p>
        )}
      </div>
      <div
        ref={scrollRef}
        data-testid="mcp-tool-list-scroll"
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain py-2 pr-1"
      >
        <CatalogNotice server={server} />
        <ToolRows server={server} onSelect={onSelect} />
        {counts.truncated && (
          <p className="pt-3 text-[13px] text-muted-foreground">
            {t("task:mcpToolCatalogTruncated", { count: counts.total })}
          </p>
        )}
      </div>
    </div>
  );
}
