"use client";

import { useState, useCallback } from "react";
import { IconTerminal2, IconCopy, IconCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { RecoveryAuthMethod } from "@/components/task/chat/types";

function buildFullCommand(termAuth: RecoveryAuthMethod["terminal_auth"]): string | null {
  if (!termAuth) return null;
  if (termAuth.args && termAuth.args.length > 0) {
    return `${termAuth.command} ${termAuth.args.join(" ")}`;
  }
  return termAuth.command;
}

export function AuthMethodsPanel({
  methods,
  onOpenTerminal,
}: {
  methods: RecoveryAuthMethod[];
  onOpenTerminal: (command: string) => void;
}) {
  return (
    <div className="mt-2 rounded border border-amber-500/30 bg-amber-500/5 p-2.5 space-y-2">
      <div className="text-xs font-medium text-amber-600 dark:text-amber-400">
        Authentication required, log in before resuming
      </div>
      {methods.map((method) => (
        <AuthMethodRow key={method.id} method={method} onOpenTerminal={onOpenTerminal} />
      ))}
    </div>
  );
}

export function GenericAuthPanel({ onOpenTerminal }: { onOpenTerminal: () => void }) {
  return (
    <div className="mt-2 rounded border border-amber-500/30 bg-amber-500/5 p-2.5 space-y-2">
      <div className="text-xs font-medium text-amber-600 dark:text-amber-400">
        Authentication required — please log in via the terminal
      </div>
      <Button
        variant="outline"
        size="sm"
        className="h-6 text-[11px] cursor-pointer gap-1 px-2"
        onClick={onOpenTerminal}
      >
        <IconTerminal2 className="h-3 w-3" />
        Open terminal
      </Button>
    </div>
  );
}

function AuthMethodRow({
  method,
  onOpenTerminal,
}: {
  method: RecoveryAuthMethod;
  onOpenTerminal: (command: string) => void;
}) {
  const [copied, setCopied] = useState(false);
  const termAuth = method.terminal_auth;
  const fullCommand = buildFullCommand(termAuth);

  const handleCopy = useCallback(async () => {
    if (!fullCommand) return;
    try {
      await navigator.clipboard.writeText(fullCommand);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard API unavailable
    }
  }, [fullCommand]);

  return (
    <div className="space-y-1">
      <div className="text-xs text-muted-foreground">{method.name}</div>
      {termAuth && fullCommand ? (
        <div className="flex items-center gap-1.5">
          <div className="flex items-center gap-1.5 text-xs bg-muted/50 rounded px-2 py-1 font-mono flex-1 min-w-0">
            <IconTerminal2 className="h-3 w-3 shrink-0 text-muted-foreground" />
            <code className="text-[11px] truncate">{fullCommand}</code>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 cursor-pointer shrink-0"
                onClick={handleCopy}
                aria-label={copied ? "Command copied" : "Copy command"}
              >
                {copied ? (
                  <IconCheck className="h-3 w-3 text-green-500" />
                ) : (
                  <IconCopy className="h-3 w-3 text-muted-foreground" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">Copy command</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                className="h-6 text-[11px] cursor-pointer gap-1 px-2 shrink-0"
                onClick={() => fullCommand && onOpenTerminal(fullCommand)}
              >
                <IconTerminal2 className="h-3 w-3" />
                Run in terminal
              </Button>
            </TooltipTrigger>
            <TooltipContent side="top">
              Open the bottom terminal and paste this command
            </TooltipContent>
          </Tooltip>
        </div>
      ) : (
        <div className="text-xs text-muted-foreground">
          {method.description || "Login required"}
        </div>
      )}
    </div>
  );
}
