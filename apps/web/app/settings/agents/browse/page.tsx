"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Separator } from "@kandev/ui/separator";
import { AgentInstallCatalog } from "@/components/settings/agents/agent-install-catalog";

export default function AgentsBrowsePage() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  return (
    <div className="space-y-8">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="flex w-full cursor-pointer items-start justify-between gap-3"
            data-testid="available-to-install-trigger"
          >
            <div className="text-left">
              <h2 className="text-2xl font-bold">{t("agents:browseAvailableAgents")}</h2>
              <p className="text-sm text-muted-foreground mt-1">
                {t("agents:availableToInstallDescription")}
              </p>
            </div>
            <IconChevronDown
              className={`h-5 w-5 shrink-0 text-muted-foreground transition-transform ${
                open ? "rotate-180" : ""
              }`}
            />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="space-y-8 pt-8">
            <Separator />
            <AgentInstallCatalog />
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
