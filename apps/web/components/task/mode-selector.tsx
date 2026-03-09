"use client";

import { memo, useCallback } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { setSessionMode } from "@/lib/api/domains/session-api";

type ModeSelectorProps = {
  sessionId: string | null;
};

export const ModeSelector = memo(function ModeSelector({ sessionId }: ModeSelectorProps) {
  const modeState = useAppStore((state) =>
    sessionId ? state.sessionMode.bySessionId[sessionId] : undefined,
  );

  const handleModeChange = useCallback(
    async (modeId: string) => {
      if (!sessionId) return;
      try {
        await setSessionMode(sessionId, modeId);
      } catch (err) {
        console.error("[ModeSelector] set-mode API failed:", err);
      }
    },
    [sessionId],
  );

  if (
    !sessionId ||
    !modeState ||
    !modeState.availableModes ||
    modeState.availableModes.length <= 1
  ) {
    return null;
  }

  const currentMode = modeState.availableModes.find((m) => m.id === modeState.currentModeId);
  const displayName = currentMode?.name || modeState.currentModeId || "Mode";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1 px-2 cursor-pointer hover:bg-muted/40 whitespace-nowrap"
        >
          <span className="text-xs">{displayName}</span>
          <IconChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" className="min-w-[280px]">
        {modeState.availableModes.map((mode) => (
          <DropdownMenuItem
            key={mode.id}
            onClick={() => handleModeChange(mode.id)}
            className={mode.id === modeState.currentModeId ? "bg-muted" : ""}
          >
            <div>
              <div>{mode.name}</div>
              {mode.description && (
                <div className="text-xs text-muted-foreground">{mode.description}</div>
              )}
            </div>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
});
