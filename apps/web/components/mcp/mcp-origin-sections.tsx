"use client";

import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { Badge } from "@kandev/ui/badge";
import type { MCPInheritedSelection, MCPSelectionOrigin } from "@/lib/types/http-mcp";
import { groupMCPOrigins, type MCPOriginGroupScope } from "./mcp-origin-groups";
import type { MCPOriginContext } from "./mcp-origin-labels";
import { McpInheritedServerDetails } from "./mcp-inherited-server-details";

const SECTION_KEYS: Record<MCPOriginGroupScope, { title: string; description: string }> = {
  task: {
    title: "settings:mcpSelectedForTask",
    description: "settings:mcpSelectedForTaskDescription",
  },
  repository: {
    title: "settings:mcpFromRepositories",
    description: "settings:mcpFromRepositoriesDescription",
  },
  profile: {
    title: "settings:mcpFromAgentProfile",
    description: "settings:mcpFromAgentProfileDescription",
  },
  unknown: { title: "settings:mcpFromTaskSetup", description: "settings:mcpOriginUnavailable" },
};

export function McpSectionHeading({
  title,
  description,
  count,
}: {
  title: string;
  description: string;
  count?: number;
}) {
  return (
    <header className="space-y-1.5">
      <h3 className="flex items-center gap-2 text-balance text-[11px] font-semibold uppercase tracking-[0.08em] text-foreground/75">
        {title}
        {count !== undefined && (
          <span
            aria-hidden="true"
            className="rounded bg-muted px-1.5 text-[10px] leading-4 tracking-normal tabular-nums"
          >
            {count}
          </span>
        )}
      </h3>
      <p className="text-pretty text-xs text-muted-foreground">{description}</p>
    </header>
  );
}

export function McpOriginSections({
  inherited,
  context,
  renderServerAnnotation,
}: {
  inherited: MCPInheritedSelection[];
  context: MCPOriginContext;
  renderServerAnnotation?: (id: string) => ReactNode;
}) {
  const { t } = useTranslation();
  const labels: Record<MCPSelectionOrigin["scope"], string> = {
    profile: t("settings:mcpOriginProfile"),
    repository: t("settings:mcpOriginRepository"),
    task: t("settings:mcpOriginTask"),
    task_session: t("settings:mcpOriginSession"),
  };
  return (
    <div data-testid="mcp-inherited-group" className="space-y-6">
      {groupMCPOrigins(inherited).map(({ scope, items }) => (
        <section key={scope} data-testid={`mcp-origin-section-${scope}`} className="space-y-3">
          <McpSectionHeading
            title={t(SECTION_KEYS[scope].title)}
            description={t(SECTION_KEYS[scope].description)}
            count={items.length}
          />
          <div className="divide-y divide-border/60 overflow-hidden rounded-lg border border-border/70 bg-background/30">
            {items.map((item) => (
              <div
                key={item.definition.id}
                data-testid={`mcp-inherited-server-${item.definition.id}`}
                data-mcp-definition-id={item.definition.id}
                tabIndex={-1}
                className="px-3 py-3 data-[mcp-highlighted=true]:ring-2 data-[mcp-highlighted=true]:ring-inset data-[mcp-highlighted=true]:ring-primary/60"
              >
                <div className="flex items-start gap-2">
                  <span className="min-w-0 flex-1 wrap-anywhere text-[13px] font-medium">
                    {item.definition.display_name}
                  </span>
                  {!item.definition.enabled && (
                    <Badge variant="outline">{t("settings:mcpDisabled")}</Badge>
                  )}
                </div>
                <McpInheritedServerDetails
                  item={item}
                  scope={scope}
                  labels={labels}
                  context={context}
                />
                {renderServerAnnotation?.(item.definition.id)}
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
