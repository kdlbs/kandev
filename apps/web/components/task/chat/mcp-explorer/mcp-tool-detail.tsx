"use client";

import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { MCPToolSummary } from "@/lib/state/slices/session-runtime/types";
import { useTranslation } from "react-i18next";
import { getMcpToolSchemaState } from "./mcp-explorer-view-model";

function ArgumentRows({ tool }: { tool: MCPToolSummary }) {
  const { t } = useTranslation();
  const state = getMcpToolSchemaState(tool);
  if (state.kind === "none") {
    return <p className="text-[13px] text-muted-foreground">{t("task:mcpNoArguments")}</p>;
  }
  if (state.kind === "too_large") {
    return <p className="text-[13px] text-muted-foreground">{t("task:mcpSchemaTooLarge")}</p>;
  }
  return (
    <div className="space-y-3">
      {state.arguments.map((argument) => (
        <div key={argument.name} className="rounded-md border border-border/70 px-3 py-2">
          <div className="flex min-w-0 flex-wrap items-baseline gap-2">
            <span className="break-all font-mono text-[13px]">{argument.name}</span>
            <span className="text-[13px] text-muted-foreground">{argument.type}</span>
            <span className="text-[13px] text-muted-foreground">
              {argument.required ? t("task:mcpRequired") : t("task:mcpOptional")}
            </span>
          </div>
          {argument.description && (
            <p className="mt-1 break-words text-[13px] text-muted-foreground">
              {argument.description}
            </p>
          )}
        </div>
      ))}
      {state.showJSON && (
        <details className="text-[13px]">
          <summary className="min-h-11 cursor-pointer content-center text-muted-foreground">
            {t("task:mcpJsonSchema")}
          </summary>
          <pre className="max-w-full overflow-x-auto rounded-md bg-muted/50 p-3 text-[13px]">
            {JSON.stringify(tool.input_schema, null, 2)}
          </pre>
        </details>
      )}
    </div>
  );
}

export function McpToolDetail({ tool, onBack }: { tool: MCPToolSummary; onBack: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="mcp-tool-detail"
      className="flex h-full min-h-0 flex-col text-[13px] leading-5"
    >
      <div className="shrink-0 border-b border-border/70 pb-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-11 cursor-pointer gap-2 px-2 text-[13px]"
          aria-label={t("task:mcpBackToTools")}
          onClick={onBack}
        >
          <IconArrowLeft className="h-4 w-4" aria-hidden="true" />
          {t("task:mcpBackToTools")}
        </Button>
        <div className="flex min-w-0 flex-wrap items-baseline gap-3">
          <h3 className="min-w-0 break-all font-mono text-[13px] font-semibold">{tool.name}</h3>
          {tool.estimated_tokens !== undefined && (
            <span className="text-[13px] text-muted-foreground">
              {t("task:mcpTokenEstimate", { count: tool.estimated_tokens })}
            </span>
          )}
        </div>
      </div>
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto overscroll-contain py-3 pr-1">
        <section className="space-y-2">
          <h4 className="font-medium">{t("task:mcpDescription")}</h4>
          <p className="whitespace-pre-wrap break-words text-[13px] text-muted-foreground">
            {tool.description || t("task:mcpNoDescription")}
          </p>
        </section>
        <section className="space-y-2">
          <h4 className="font-medium">{t("task:mcpArguments")}</h4>
          <ArgumentRows tool={tool} />
        </section>
      </div>
    </div>
  );
}
