"use client";

import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";
import type { MCPServerDefinition } from "@/lib/types/http-mcp";
import { mcpDefinitionBadgeKeys } from "./mcp-definition-badges";

export function McpDefinitionDetails({ definition }: { definition: MCPServerDefinition }) {
  const { t } = useTranslation();
  const keys = mcpDefinitionBadgeKeys(definition);
  return (
    <span className="mt-1.5 block space-y-2">
      {definition.description?.trim() && (
        <span
          data-testid="mcp-definition-description"
          className="block wrap-anywhere text-pretty text-xs leading-5 text-muted-foreground"
        >
          {definition.description}
        </span>
      )}
      <span data-testid="mcp-definition-badges" className="flex flex-wrap gap-1.5">
        {[keys.mode, keys.transport, keys.source]
          .filter((key) => key !== undefined)
          .map((key) => (
            <Badge
              key={key}
              variant="outline"
              className="h-auto min-h-6 max-w-full whitespace-normal border-border bg-muted px-2 py-0.5 text-xs font-medium leading-4 text-foreground dark:bg-muted"
            >
              {t(key)}
            </Badge>
          ))}
      </span>
    </span>
  );
}
