"use client";

import { useTranslation } from "react-i18next";
import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import Link from "@/components/routing/app-link";
import { AgentInstallCatalog } from "@/components/settings/agents/agent-install-catalog";
import { AGENTS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";

export default function AgentsBrowsePage() {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <div className="space-y-3">
        <Button variant="ghost" size="sm" className="-ml-2 cursor-pointer" asChild>
          <Link href={AGENTS_SETTINGS_HREF}>
            <IconArrowLeft className="h-4 w-4 mr-2" />
            {t("common:agents")}
          </Link>
        </Button>
        <div>
          <h2 className="text-2xl font-bold">{t("agents:browseAvailableAgents")}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            {t("agents:availableToInstallDescription")}
          </p>
        </div>
      </div>
      <Separator />
      <AgentInstallCatalog />
    </div>
  );
}
