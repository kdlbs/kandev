"use client";

import { useTranslation } from "react-i18next";
import { useSearchParams } from "@/lib/routing/client-router";
import { Separator } from "@kandev/ui/separator";
import { AgentInstallCatalog } from "@/components/settings/agents/agent-install-catalog";

export default function AgentsBrowsePage() {
  const { t } = useTranslation();
  const agentHint = useSearchParams().get("agent") ?? undefined;
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{t("agents:browseAvailableAgents")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("agents:availableToInstallDescription")}
        </p>
      </div>
      <Separator />
      <AgentInstallCatalog agentHint={agentHint} />
    </div>
  );
}
