"use client";

import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconLayoutDashboard, IconRefresh } from "@tabler/icons-react";

function resetBrowserStorage() {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.clear();
    window.sessionStorage.clear();
  } finally {
    window.location.reload();
  }
}

export function UIStateCard() {
  const { t } = useTranslation();
  // Shown both inline and in the button's tooltip.
  const help = t("system:uiStateHelp");
  return (
    <Card data-testid="system-ui-state-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconLayoutDashboard className="h-4 w-4" /> {t("system:uiStateTitle")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-start justify-between gap-3 rounded-md border p-3">
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium">{t("system:uiStateResetLabel")}</p>
            <p className="text-xs text-muted-foreground mt-1">{help}</p>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                onClick={resetBrowserStorage}
                className="cursor-pointer shrink-0"
                data-testid="system-ui-state-reset"
              >
                <IconRefresh className="h-3.5 w-3.5 mr-1" />
                {t("system:uiStateReset")}
              </Button>
            </TooltipTrigger>
            <TooltipContent className="max-w-xs">{help}</TooltipContent>
          </Tooltip>
        </div>
      </CardContent>
    </Card>
  );
}
