"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";
import { resolveCursorMcpDiscoveryReason } from "@/lib/prepare/cursor-mcp-discovery-reason";

export function CursorMcpDiscoveryDetails({ step }: { step: PrepareStepInfo }) {
  if (
    step.kind !== "agent_mcp_discovery" ||
    step.mcpProvider !== "cursor" ||
    step.status !== "failed"
  ) {
    return null;
  }
  return <CursorMcpDiscoveryDisclosure failureCode={step.failureCode} />;
}

function CursorMcpDiscoveryDisclosure({ failureCode }: { failureCode?: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { messageKey, marketplaceLookupFailed } = resolveCursorMcpDiscoveryReason(failureCode);

  return (
    <div className="mt-1 min-w-0 max-w-full" data-testid="cursor-mcp-discovery-details">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            className="justify-start px-2 max-md:w-full"
            data-testid="cursor-mcp-discovery-details-toggle"
          >
            {t("task:details")}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="min-w-0 max-w-full">
          <div className="border-muted-foreground/20 text-muted-foreground mt-1 ml-0.5 space-y-1 border-l pl-3 text-xs leading-relaxed break-words">
            <p data-testid="cursor-mcp-discovery-copy">{t(messageKey)}</p>
            {marketplaceLookupFailed && (
              <p>{t("task:agentMcpCursorDiscoveryMarketplaceSkipped")}</p>
            )}
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
