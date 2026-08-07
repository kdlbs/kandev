"use client";

import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { IconRefresh, IconArrowRight } from "@tabler/icons-react";
import { useUpdates } from "@/hooks/domains/system/use-updates";

export function VersionSummaryCard() {
  const { t } = useTranslation();
  const { updates } = useUpdates();
  const current = updates?.current ?? "-";
  const latest = updates?.latest ?? "-";
  const updateAvailable = updates?.update_available ?? false;

  return (
    <Card data-testid="system-version-summary-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconRefresh className="h-4 w-4" />
          {t("system:versionTitle")}
          {updateAvailable && (
            <Badge variant="default" className="text-[10px]">
              {t("system:updateAvailable")}
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {/* Both version strings are values. */}
        <div className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <div className="text-xs text-muted-foreground">{t("system:versionCurrent")}</div>
            <div className="font-mono text-sm" data-testid="system-version-current">
              {current}
            </div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("system:versionLatest")}</div>
            <div className="font-mono text-sm" data-testid="system-version-latest">
              {latest}
            </div>
          </div>
        </div>
        <Button asChild variant="outline" size="sm" className="cursor-pointer">
          <Link href="/settings/system/updates" data-testid="system-version-updates-link">
            {t("system:versionViewUpdates")}
            <IconArrowRight className="h-3.5 w-3.5 ml-1" />
          </Link>
        </Button>
      </CardContent>
    </Card>
  );
}
