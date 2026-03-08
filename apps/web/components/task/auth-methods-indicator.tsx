"use client";

import { memo, useCallback } from "react";
import { IconKey, IconTerminal2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useAppStore } from "@/components/state-provider";
import { authenticateSession } from "@/lib/api/domains/session-api";

type AuthMethodsIndicatorProps = {
  sessionId: string | null;
};

export const AuthMethodsIndicator = memo(function AuthMethodsIndicator({
  sessionId,
}: AuthMethodsIndicatorProps) {
  const authMethods = useAppStore((state) => {
    if (!sessionId) return undefined;
    return state.agentCapabilities.bySessionId[sessionId]?.authMethods;
  });

  const handleAuthenticate = useCallback(
    async (methodId: string) => {
      if (!sessionId) return;
      try {
        await authenticateSession(sessionId, methodId);
      } catch {
        // Auth call failed — agent will report via events if successful
      }
    },
    [sessionId],
  );

  if (!sessionId || !authMethods || authMethods.length === 0) {
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
          <IconKey className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" side="top" className="w-72 p-3">
        <div className="space-y-2">
          <div className="text-xs font-medium text-muted-foreground">Authentication</div>
          {authMethods.map((method) => (
            <div key={method.id} className="space-y-1">
              <div className="text-sm">{method.name}</div>
              {method.description && (
                <div className="text-xs text-muted-foreground">{method.description}</div>
              )}
              {method.terminalAuth ? (
                <div className="flex items-center gap-1.5 text-xs text-muted-foreground bg-muted/50 rounded px-2 py-1">
                  <IconTerminal2 className="h-3 w-3 shrink-0" />
                  <code className="text-[11px]">
                    {method.terminalAuth.command}
                    {method.terminalAuth.args ? ` ${method.terminalAuth.args.join(" ")}` : ""}
                  </code>
                </div>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  className="h-6 text-xs cursor-pointer"
                  onClick={() => handleAuthenticate(method.id)}
                >
                  Login
                </Button>
              )}
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
});
