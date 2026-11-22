"use client";

import { useTranslation } from "react-i18next";
import type { MCPInheritedSelection, MCPSelectionOrigin } from "@/lib/types/http-mcp";
import { McpDefinitionDetails } from "./mcp-definition-details";
import type { MCPOriginGroupScope } from "./mcp-origin-groups";
import { formatMCPOriginLabel, type MCPOriginContext } from "./mcp-origin-labels";

export function McpInheritedServerDetails({
  item,
  scope,
  labels,
  context,
}: {
  item: MCPInheritedSelection;
  scope: MCPOriginGroupScope;
  labels: Record<MCPSelectionOrigin["scope"], string>;
  context: MCPOriginContext;
}) {
  const { t } = useTranslation();
  const primary = item.origins.filter((origin) => origin.scope === scope && scope !== "task");
  const secondary = item.origins.filter((origin) => origin.scope !== scope);
  const originText = (origin: MCPSelectionOrigin) => (
    <span
      key={`${origin.workspace_id}:${origin.scope}:${origin.owner_id}`}
      className="block wrap-anywhere text-xs leading-5 text-muted-foreground"
    >
      {formatMCPOriginLabel(origin, labels[origin.scope], context)}
    </span>
  );
  return (
    <div className="space-y-2">
      <McpDefinitionDetails definition={item.definition} />
      {primary.length > 0 && <div>{primary.map(originText)}</div>}
      {secondary.length > 0 && (
        <div className="space-y-0.5 border-t border-border/50 pt-2">
          <span className="block text-[11px] font-medium leading-4 text-foreground/75">
            {t("settings:mcpAlsoSelectedBy")}
          </span>
          {secondary.map(originText)}
        </div>
      )}
    </div>
  );
}
