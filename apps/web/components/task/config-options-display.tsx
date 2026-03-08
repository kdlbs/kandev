"use client";

import { memo } from "react";
import { IconSettings } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useAppStore } from "@/components/state-provider";

type ConfigOptionsDisplayProps = {
  sessionId: string | null;
};

export const ConfigOptionsDisplay = memo(function ConfigOptionsDisplay({
  sessionId,
}: ConfigOptionsDisplayProps) {
  const configOptions = useAppStore((state) => {
    if (!sessionId) return undefined;
    return state.sessionModels.bySessionId[sessionId]?.configOptions;
  });

  if (!sessionId || !configOptions || configOptions.length === 0) {
    return null;
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1 px-2 cursor-pointer hover:bg-muted/40"
        >
          <IconSettings className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" side="top" className="w-64 p-3">
        <div className="space-y-2">
          <div className="text-xs font-medium text-muted-foreground">Config Options</div>
          {configOptions.map((option) => (
            <div key={option.id} className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">{option.name}</span>
              <span className="font-mono">{option.currentValue}</span>
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
});
