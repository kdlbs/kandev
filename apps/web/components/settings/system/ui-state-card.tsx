"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconLayoutDashboard, IconRefresh } from "@tabler/icons-react";

const HELP =
  "Clears layout and UI preferences saved in your browser (panel sizes, pinned tasks, expanded groups, etc.). Use this if the app looks broken, panels are stuck, or the layout feels wrong. Your tasks, sessions, and server data are not affected - only the per-browser UI state. The page reloads after the reset.";

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
  return (
    <Card data-testid="system-ui-state-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconLayoutDashboard className="h-4 w-4" /> UI state
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-start justify-between gap-3 rounded-md border p-3">
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium">Reset browser layout</p>
            <p className="text-xs text-muted-foreground mt-1">{HELP}</p>
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
                Reset
              </Button>
            </TooltipTrigger>
            <TooltipContent className="max-w-xs">{HELP}</TooltipContent>
          </Tooltip>
        </div>
      </CardContent>
    </Card>
  );
}
